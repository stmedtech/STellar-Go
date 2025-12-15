package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const proxyProtocolID = protocol.ID("/stellar-proxy/1.0.0")

// Use string version for SetStreamHandler (libp2p accepts both)
const proxyProtocolIDStr = "/stellar-proxy/1.0.0"

// TestLibp2pStreamCreation is a minimal test to verify libp2p stream creation works
// Based on go-libp2p examples/echo pattern
func TestLibp2pStreamCreation(t *testing.T) {
	ctx := context.Background()

	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST (following transport_test.go pattern)
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(ctx, peerInfo)
	require.NoError(t, err)

	// Set up server stream handler AFTER connecting (following transport_test.go pattern)
	// Use string directly like in echo example - libp2p accepts both string and protocol.ID
	var serverStream network.Stream
	handlerCalled := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(handlerCalled)
	})

	// Wait for connection to be established (following transport_test.go pattern)
	// Poll until connected
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()), "Hosts should be connected")

	// Create a stream - this should trigger the handler (following echo example)
	// Use string directly like in echo example
	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err, "Failed to create libp2p stream")
	defer stream.Close()

	// Wait for handler to be called (should happen immediately when NewStream succeeds)
	select {
	case <-handlerCalled:
		require.NotNil(t, serverStream, "Server stream should be set")
		t.Logf("Successfully created libp2p stream. Protocol: %s", proxyProtocolID)
	case <-time.After(2 * time.Second):
		t.Fatal("Stream handler was not called. This indicates libp2p protocol negotiation failed.")
	}

	// Verify we can write and read on the stream
	testData := []byte("hello")
	_, err = stream.Write(testData)
	require.NoError(t, err)

	buffer := make([]byte, len(testData))
	n, err := serverStream.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, len(testData), n)
	require.Equal(t, testData, buffer[:n])
}

// TestProxyHTTPKeepAlive ensures two sequential HTTP/1.1 requests over the same
// proxied TCP connection succeed without empty responses or premature closes.
func TestProxyHTTPKeepAlive(t *testing.T) {
	// Backend HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/one", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("one"))
	})
	mux.HandleFunc("/two", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("two"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host // host:port

	// libp2p hosts
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	// Attach proxy server handler
	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		// Serve until stream/conn closes
		_ = srv.Serve(context.Background())
	})

	// Connect client to server
	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	// Open proxy control stream
	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	// Local pipe simulating browser TCP conn
	localConn, browserConn := net.Pipe()
	defer browserConn.Close()

	// Open proxy and attach local side
	_, err = client.OpenWithLocalConn("p-http", backendAddr, "tcp", localConn)
	require.NoError(t, err)

	// Send two sequential HTTP requests on same conn
	bw := bufio.NewWriter(browserConn)
	br := bufio.NewReader(browserConn)

	sendReq := func(path, connHdr string) {
		fmt.Fprintf(bw, "GET %s HTTP/1.1\r\nHost: test\r\nConnection: %s\r\n\r\n", path, connHdr)
		bw.Flush()
	}

	// First request keep-alive
	sendReq("/one", "keep-alive")
	resp1, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	assert.Equal(t, "one", string(body1))

	// Second request reuses same conn
	sendReq("/two", "close")
	resp2, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	assert.Equal(t, "two", string(body2))
}

// TestProxyHTTPKeepAliveContentLength ensures large bodies over keep-alive are
// delivered fully without truncation (which would surface as
// ERR_CONTENT_LENGTH_MISMATCH in browsers).
func TestProxyHTTPKeepAliveContentLength(t *testing.T) {
	// Large deterministic body
	body := bytes.Repeat([]byte("x"), 128*1024) // 128 KiB

	// Backend HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/small", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host

	// libp2p hosts
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	// Attach proxy server handler
	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		_ = srv.Serve(context.Background())
	})

	// Connect client to server
	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	// Open proxy control stream
	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	// Local pipe simulating browser TCP conn
	localConn, browserConn := net.Pipe()
	defer browserConn.Close()

	_, err = client.OpenWithLocalConn("p-http-big", backendAddr, "tcp", localConn)
	require.NoError(t, err)

	bw := bufio.NewWriter(browserConn)
	br := bufio.NewReader(browserConn)

	sendReq := func(path, connHdr string) {
		fmt.Fprintf(bw, "GET %s HTTP/1.1\r\nHost: test\r\nConnection: %s\r\n\r\n", path, connHdr)
		bw.Flush()
	}

	// First request: big body, keep-alive
	sendReq("/big", "keep-alive")
	resp1, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	b1, err := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, len(body), len(b1))
	assert.True(t, bytes.Equal(body, b1))

	// Second request on same conn to ensure connection still valid
	sendReq("/small", "close")
	resp2, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	b2, err := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "ok", string(b2))
}

// TestProxyHTTPConcurrentConnections exercises multiple simultaneous proxied TCP
// connections (browser-style parallel fetches) to ensure responses are isolated
// and bodies are delivered fully without mixups or truncation.
func TestProxyHTTPConcurrentConnections(t *testing.T) {
	bodyA := bytes.Repeat([]byte("A"), 64*1024)
	bodyB := bytes.Repeat([]byte("B"), 96*1024)

	// Backend HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyA)))
		_, _ = w.Write(bodyA)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyB)))
		_, _ = w.Write(bodyB)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		_ = srv.Serve(context.Background())
	})

	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	type reqSpec struct {
		path     string
		expected []byte
	}

	specs := []reqSpec{
		{path: "/a", expected: bodyA},
		{path: "/b", expected: bodyB},
	}

	var wg sync.WaitGroup
	wg.Add(len(specs))

	for i, spec := range specs {
		spec := spec
		proxyID := fmt.Sprintf("p-http-concurrent-%d", i)

		go func() {
			defer wg.Done()

			// Local pipe simulating browser TCP conn
			localConn, browserConn := net.Pipe()
			defer browserConn.Close()
			defer localConn.Close()

			_, err := client.OpenWithLocalConn(proxyID, backendAddr, "tcp", localConn)
			require.NoError(t, err)

			bw := bufio.NewWriter(browserConn)
			br := bufio.NewReader(browserConn)

			fmt.Fprintf(bw, "GET %s HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n", spec.path)
			bw.Flush()

			resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, len(spec.expected), len(data))
			assert.True(t, bytes.Equal(spec.expected, data))
		}()
	}

	wg.Wait()
}

// TestProxyHTTPClientHalfClose ensures that when the client half-closes (FIN on
// upload side) the downstream response is still delivered fully and not
// truncated/closed prematurely.
func TestProxyHTTPClientHalfClose(t *testing.T) {
	body := bytes.Repeat([]byte("Z"), 64*1024)

	// Backend HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/z", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write(body)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		_ = srv.Serve(context.Background())
	})

	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	// Create a real TCP pair so we can CloseWrite on browser side
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	serverConnCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		serverConnCh <- c
	}()

	localConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer localConn.Close()

	browserConn := <-serverConnCh
	defer browserConn.Close()

	_, err = client.OpenWithLocalConn("p-http-halfclose", backendAddr, "tcp", localConn)
	require.NoError(t, err)

	bw := bufio.NewWriter(browserConn)
	br := bufio.NewReader(browserConn)

	fmt.Fprintf(bw, "GET /z HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n")
	bw.Flush()

	// Half-close upload side (FIN) while keeping read open
	if tcpConn, ok := browserConn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}

	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, len(body), len(data))
	assert.True(t, bytes.Equal(body, data))
}

// TestProxyHTTPChunked ensures chunked responses arrive intact over the proxy.
func TestProxyHTTPChunked(t *testing.T) {
	chunks := []string{"hello ", "chunked ", "world"}

	mux := http.NewServeMux()
	mux.HandleFunc("/chunked", func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/plain")
		for _, c := range chunks {
			_, _ = w.Write([]byte(c))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		_ = srv.Serve(context.Background())
	})

	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	localConn, browserConn := net.Pipe()
	defer browserConn.Close()

	_, err = client.OpenWithLocalConn("p-http-chunked", backendAddr, "tcp", localConn)
	require.NoError(t, err)

	bw := bufio.NewWriter(browserConn)
	br := bufio.NewReader(browserConn)

	fmt.Fprintf(bw, "GET /chunked HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n")
	bw.Flush()

	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	assert.Equal(t, strings.Join(chunks, ""), string(data))
}

// TestProxyHTTPManyParallel stresses many concurrent short requests to catch
// racey teardown issues that could surface as ERR_EMPTY_RESPONSE.
func TestProxyHTTPManyParallel(t *testing.T) {
	var bodies [][]byte
	for i := 0; i < 10; i++ {
		size := 16*1024 + i*4096
		bodies = append(bodies, bytes.Repeat([]byte{byte('a' + i)}, size))
	}

	mux := http.NewServeMux()
	for i, b := range bodies {
		path := fmt.Sprintf("/r%d", i)
		body := b
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			_, _ = w.Write(body)
		})
	}
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	backendAddr := tsURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hServer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer hClient.Close()

	hServer.SetStreamHandler(proxyProtocolID, func(s network.Stream) {
		srv := NewServer(s)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		_ = srv.Serve(context.Background())
	})

	err = hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()})
	require.NoError(t, err)

	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	ctrl, err := hClient.NewStream(allowCtx, hServer.ID(), proxyProtocolID)
	require.NoError(t, err)
	defer ctrl.Close()

	client := NewClient(hClient.ID().String(), ctrl)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	defer client.CloseAll()

	var wg sync.WaitGroup
	wg.Add(len(bodies))

	for i, expected := range bodies {
		i, expected := i, expected
		go func() {
			defer wg.Done()
			localConn, browserConn := net.Pipe()
			defer browserConn.Close()
			defer localConn.Close()

			_, err := client.OpenWithLocalConn(fmt.Sprintf("p-http-par-%d", i), backendAddr, "tcp", localConn)
			require.NoError(t, err)

			bw := bufio.NewWriter(browserConn)
			br := bufio.NewReader(browserConn)

			fmt.Fprintf(bw, "GET /r%d HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n", i)
			bw.Flush()

			resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, len(expected), len(data))
			assert.True(t, bytes.Equal(expected, data))
		}()
	}

	wg.Wait()
}

// TestLibp2pServerAccept tests server handshake with libp2p stream
func TestLibp2pServerAccept(t *testing.T) {
	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST (following transport_test.go pattern)
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err, "Failed to connect host2 to host1")

	// Wait for connection to be established
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()), "Hosts should be connected")

	// Set up server stream handler AFTER connecting (following transport_test.go pattern)
	var serverStream network.Stream
	ready := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(ready)
	})

	// Create a stream from host2 to host1 - this should trigger the server's stream handler
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	allowCtx := network.WithAllowLimitedConn(ctx, proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err, "Failed to create libp2p stream")
	defer stream.Close()

	// Wait for server stream handler to be called
	select {
	case <-ready:
		require.NotNil(t, serverStream, "Server stream should not be nil")
	case <-time.After(5 * time.Second):
		t.Fatal("Server stream handler was not called within 5 seconds")
	}

	// Ensure stream is fully established by checking its state
	// Libp2p streams may need a moment to be fully ready
	time.Sleep(100 * time.Millisecond)

	server := NewServer(serverStream)
	require.NotNil(t, server)

	// Create client from the outgoing stream
	client := NewClient("test-client", stream)
	require.NotNil(t, client)

	// Wait for multiplexer readLoops to start (critical for packet reading)
	// Both server and client need their readLoops running
	// Libp2p streams may need more time than TCP connections
	time.Sleep(500 * time.Millisecond)

	// Start server Accept() FIRST - it will block waiting for hello
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	// Give server time to start Accept() and be ready to read
	time.Sleep(200 * time.Millisecond)

	// Now client connects (sends hello, waits for ack)
	// The hello packet will be written to the control stream, which should
	// trigger the server's readLoop to receive it
	connectErr := make(chan error, 1)
	go func() {
		connectErr <- client.Connect()
	}()

	// Wait for both to complete with timeouts
	// If this hangs, the issue is likely that:
	// 1. The multiplexer readLoop isn't receiving data from libp2p streams
	// 2. The libp2p stream isn't fully established
	// 3. There's a buffering issue with libp2p streams
	select {
	case err = <-acceptErr:
		require.NoError(t, err, "Server Accept() should succeed")
	case <-time.After(10 * time.Second):
		t.Fatal("Server Accept() timed out - ReadPacket() is blocking. " +
			"This indicates the multiplexer readLoop isn't receiving data from the libp2p stream. " +
			"Libp2p streams may need special handling or the stream may not be fully established.")
	}

	select {
	case err = <-connectErr:
		require.NoError(t, err, "Client Connect() should succeed")
	case <-time.After(10 * time.Second):
		t.Fatal("Client Connect() timed out - ReadPacket() is blocking. " +
			"This indicates the multiplexer readLoop isn't receiving data from the libp2p stream.")
	}

	assert.Equal(t, "test-client", server.ClientID())
}

// TestLibp2pProxyOpenClose tests proxy open and close with libp2p streams
func TestLibp2pProxyOpenClose(t *testing.T) {
	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to be established
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()))

	// Set up server stream handler AFTER connecting
	var serverStream network.Stream
	ready := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(ready)
	})

	// Create a stream from host2 to host1
	allowCtx := network.WithAllowLimitedConn(context.Background(), proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err)
	defer stream.Close()

	// Wait for server stream to be created
	<-ready
	require.NotNil(t, serverStream)

	server := NewServer(serverStream)
	client := NewClient("test-client", stream)

	// Wait for multiplexer readLoops to start
	time.Sleep(200 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, client.Connect())

	select {
	case err = <-acceptErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server Accept() timed out")
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Start a test TCP server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Open proxy
	proxy, err := client.Open("test-proxy", testAddr, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy)

	assert.Equal(t, "test-proxy", proxy.ID)
	assert.Equal(t, testAddr, proxy.RemoteAddr)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Close proxy
	err = client.Close("test-proxy")
	require.NoError(t, err)
}

// TestLibp2pBidirectionalCommunication tests bidirectional data flow through proxy
func TestLibp2pBidirectionalCommunication(t *testing.T) {
	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to be established
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()))

	// Set up server stream handler AFTER connecting
	var serverStream network.Stream
	ready := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(ready)
	})

	// Create a stream from host2 to host1
	allowCtx := network.WithAllowLimitedConn(context.Background(), proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err)
	defer stream.Close()

	// Wait for server stream to be created
	<-ready
	require.NotNil(t, serverStream)

	server := NewServer(serverStream)
	client := NewClient("test-client", stream)

	// Wait for multiplexer readLoops to start
	time.Sleep(200 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, client.Connect())

	select {
	case err = <-acceptErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server Accept() timed out")
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Start a test echo server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Start echo server
	echoReady := make(chan struct{})
	go func() {
		close(echoReady)
		for {
			conn, err := testServer.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo
			}(conn)
		}
	}()
	<-echoReady
	time.Sleep(50 * time.Millisecond)

	// Open proxy
	proxy, err := client.Open("test-proxy", testAddr, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy)

	// Wait for proxy to be ready
	time.Sleep(200 * time.Millisecond)

	// Connect to the proxy (simulating a client connecting to the proxy)
	// In a real scenario, this would be done via the local port
	// For this test, we'll use the proxy's stream directly
	proxyStream := proxy.Stream
	require.NotNil(t, proxyStream)

	// Write data to proxy
	testData := []byte("hello from client")
	_, err = proxyStream.Write(testData)
	require.NoError(t, err)

	// Read echoed data back
	buffer := make([]byte, 100)
	n, err := proxyStream.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from client", string(buffer[:n]))

	// Test reverse direction
	reverseData := []byte("hello from server")
	_, err = proxyStream.Write(reverseData)
	require.NoError(t, err)

	reverseBuffer := make([]byte, 100)
	n, err = proxyStream.Read(reverseBuffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from server", string(reverseBuffer[:n]))
}

// TestLibp2pMultipleProxies tests multiple proxies over a single libp2p stream
func TestLibp2pMultipleProxies(t *testing.T) {
	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to be established
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()))

	// Set up server stream handler AFTER connecting
	var serverStream network.Stream
	ready := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(ready)
	})

	// Create a stream from host2 to host1
	allowCtx := network.WithAllowLimitedConn(context.Background(), proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err)
	defer stream.Close()

	// Wait for server stream to be created
	<-ready
	require.NotNil(t, serverStream)

	server := NewServer(serverStream)
	client := NewClient("test-client", stream)

	// Wait for multiplexer readLoops to start
	time.Sleep(200 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, client.Connect())

	select {
	case err = <-acceptErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server Accept() timed out")
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Start multiple test servers
	testServer1, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer1.Close()

	testServer2, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer2.Close()

	testAddr1 := testServer1.Addr().String()
	testAddr2 := testServer2.Addr().String()

	// Open multiple proxies
	proxy1, err := client.Open("proxy-1", testAddr1, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy1)

	proxy2, err := client.Open("proxy-2", testAddr2, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy2)

	// Wait for proxies to be ready
	time.Sleep(200 * time.Millisecond)

	// Verify both proxies exist
	assert.Equal(t, "proxy-1", proxy1.ID)
	assert.Equal(t, "proxy-2", proxy2.ID)

	// List proxies
	proxies, err := client.List()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(proxies), 2)

	// Close one proxy
	err = client.Close("proxy-1")
	require.NoError(t, err)

	// Verify proxy is closed
	time.Sleep(100 * time.Millisecond)
	proxies, err = client.List()
	require.NoError(t, err)
	// Should have at least one proxy left (proxy-2)
	assert.GreaterOrEqual(t, len(proxies), 1)
}

// TestLibp2pConcurrentProxies tests concurrent proxy operations
func TestLibp2pConcurrentProxies(t *testing.T) {
	// Create host1 (server)
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h1.Close() })

	// Create host2 (client)
	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { h2.Close() })

	// Connect host2 to host1 FIRST
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to be established
	for i := 0; i < 50; i++ {
		if h2.Network().Connectedness(h1.ID()) == network.Connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, network.Connected, h2.Network().Connectedness(h1.ID()))

	// Set up server stream handler AFTER connecting
	var serverStream network.Stream
	ready := make(chan struct{})
	h1.SetStreamHandler(proxyProtocolIDStr, func(s network.Stream) {
		serverStream = s
		close(ready)
	})

	// Create a stream from host2 to host1
	allowCtx := network.WithAllowLimitedConn(context.Background(), proxyProtocolIDStr)
	stream, err := h2.NewStream(allowCtx, h1.ID(), proxyProtocolIDStr)
	require.NoError(t, err)
	defer stream.Close()

	// Wait for server stream to be created
	<-ready
	require.NotNil(t, serverStream)

	server := NewServer(serverStream)
	client := NewClient("test-client", stream)

	// Wait for multiplexer readLoops to start
	time.Sleep(200 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, client.Connect())

	select {
	case err = <-acceptErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server Accept() timed out")
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Start a test server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Accept connections in background
	go func() {
		for {
			conn, err := testServer.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Concurrent proxy opens
	numProxies := 5
	done := make(chan bool, numProxies)

	for i := 0; i < numProxies; i++ {
		go func(id int) {
			defer func() { done <- true }()
			proxyID := "proxy-" + string(rune(id))
			_, err := client.Open(proxyID, testAddr, "tcp")
			if err != nil {
				t.Logf("Failed to open proxy %s: %v", proxyID, err)
			}
		}(i)
	}

	// Wait for all operations
	timeout := time.After(5 * time.Second)
	for i := 0; i < numProxies; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Concurrent proxy operations timed out")
		}
	}

	// Wait a bit for all proxies to be registered
	time.Sleep(200 * time.Millisecond)

	// Verify proxies were created
	proxies, err := client.List()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(proxies), 1) // At least some proxies should be created
}
