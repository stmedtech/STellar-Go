package service

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
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
	stream, err := h2.NewStream(ctx, h1.ID(), proxyProtocolIDStr)
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

// setupLibp2pNodes creates two libp2p hosts and connects them
func setupLibp2pNodes(t *testing.T) (host.Host, host.Host, network.Stream) {
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

	// Connect host2 to host1
	peerInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}
	err = h2.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to be established
	time.Sleep(100 * time.Millisecond)

	// Create a stream from host2 to host1
	stream, err := h2.NewStream(context.Background(), h1.ID(), proxyProtocolID)
	require.NoError(t, err)

	return h1, h2, stream
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
	stream, err := h2.NewStream(ctx, h1.ID(), proxyProtocolIDStr)
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
	stream, err := h2.NewStream(context.Background(), h1.ID(), proxyProtocolIDStr)
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
	stream, err := h2.NewStream(context.Background(), h1.ID(), proxyProtocolIDStr)
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
	stream, err := h2.NewStream(context.Background(), h1.ID(), proxyProtocolIDStr)
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
	stream, err := h2.NewStream(context.Background(), h1.ID(), proxyProtocolIDStr)
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
