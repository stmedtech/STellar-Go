package service

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"stellar/p2p/constant"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	tpttcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowConn introduces small delays to mimic relayed/slow links.
type slowConn struct {
	net.Conn
	readDelay  time.Duration
	writeDelay time.Duration
}

func (c *slowConn) Read(b []byte) (int, error) {
	if c.readDelay > 0 {
		time.Sleep(c.readDelay)
	}
	return c.Conn.Read(b)
}

func (c *slowConn) Write(b []byte) (int, error) {
	if c.writeDelay > 0 {
		time.Sleep(c.writeDelay)
	}
	return c.Conn.Write(b)
}

type filePair struct {
	server *Server
	client *Client
	cancel context.CancelFunc
	done   chan error
	dir    string
}

func startFilePair(t *testing.T, clientConn, serverConn net.Conn) *filePair {
	t.Helper()

	dir := t.TempDir()
	server := NewServer(serverConn, dir)
	require.NotNil(t, server)

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	acceptErr := make(chan error, 1)
	go func() { acceptErr <- server.Accept() }()

	// Allow server to enter Accept
	time.Sleep(20 * time.Millisecond)

	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Log("server did not stop before timeout")
		}
	})

	return &filePair{
		server: server,
		client: client,
		cancel: cancel,
		done:   done,
		dir:    dir,
	}
}

func writeServerFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// Test direct in-memory pipe (byte-level ReadWriteCloser).
func TestFileProtocolDirectPipe(t *testing.T) {
	c1, c2 := net.Pipe()
	pair := startFilePair(t, c1, c2)

	writeServerFile(t, pair.dir, "hello.txt", "world")

	entries, err := pair.client.List("/", false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
}

// Test over a slow link (simulated relay).
func TestFileProtocolSlowLink(t *testing.T) {
	c1, c2 := net.Pipe()
	slow1 := &slowConn{Conn: c1, readDelay: 2 * time.Millisecond, writeDelay: 2 * time.Millisecond}
	slow2 := &slowConn{Conn: c2, readDelay: 2 * time.Millisecond, writeDelay: 2 * time.Millisecond}

	pair := startFilePair(t, slow1, slow2)

	writeServerFile(t, pair.dir, "hello.txt", "world")

	_, err := pair.client.List("/", false)
	require.NoError(t, err)
}

// Test concurrent requests to ensure dispatcher and multiplexer are healthy.
func TestFileProtocolConcurrentRequests(t *testing.T) {
	c1, c2 := net.Pipe()
	pair := startFilePair(t, c1, c2)

	// Seed two files
	writeServerFile(t, pair.dir, "a.txt", "A")
	writeServerFile(t, pair.dir, "b.txt", "B")

	var wg sync.WaitGroup
	wg.Add(2)

	errCh := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := pair.client.List("/", false)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := pair.client.List("/", true)
		errCh <- err
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

// -------- libp2p-level tests --------

// spin up a file server handler on a libp2p host
func attachFileHandler(t *testing.T, h host.Host, dataDir string) {
	h.SetStreamHandler(protocol.ID(constant.StellarFileProtocol), func(s network.Stream) {
		defer s.Close()
		srv := NewServer(s, dataDir)
		if srv == nil {
			return
		}
		if err := srv.Accept(); err != nil {
			return
		}
		// serve until stream closes
		_ = srv.Serve(context.Background())
	})
}

func TestFileProtocolLibp2pDirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hServer, err := libp2p.New()
	require.NoError(t, err)
	defer hServer.Close()

	hClient, err := libp2p.New()
	require.NoError(t, err)
	defer hClient.Close()

	// Setup server handler
	serverDir := t.TempDir()
	attachFileHandler(t, hServer, serverDir)
	writeServerFile(t, serverDir, "hello.txt", "world")

	// Connect hosts
	require.NoError(t, hClient.Connect(ctx, peer.AddrInfo{ID: hServer.ID(), Addrs: hServer.Addrs()}))

	// Open stream and use service client
	allowCtx := network.WithAllowLimitedConn(ctx, string(constant.StellarFileProtocol))
	stream, err := hClient.NewStream(allowCtx, hServer.ID(), protocol.ID(constant.StellarFileProtocol))
	require.NoError(t, err)
	defer stream.Close()

	client := NewClient(hClient.ID().String(), stream)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())

	entries, err := client.List("/", false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
}

func TestFileProtocolLibp2pRelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Relay host
	hRelay, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(tpttcp.NewTCPTransport),
	)
	require.NoError(t, err)
	defer hRelay.Close()
	_, err = relayv2.New(hRelay)
	require.NoError(t, err)

	// Server behind relay
	hServer, err := libp2p.New(
		libp2p.EnableRelay(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(tpttcp.NewTCPTransport),
	)
	require.NoError(t, err)
	defer hServer.Close()

	// Client behind relay
	hClient, err := libp2p.New(
		libp2p.EnableRelay(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(tpttcp.NewTCPTransport),
		libp2p.EnableAutoRelay(),
		libp2p.EnableAutoRelayWithStaticRelays(nil, autorelay.WithMinCandidates(0)),
	)
	require.NoError(t, err)
	defer hClient.Close()

	// Setup server handler
	serverDir := t.TempDir()
	attachFileHandler(t, hServer, serverDir)
	writeServerFile(t, serverDir, "hello.txt", "world")

	// Connect server to relay
	require.NoError(t, hServer.Connect(ctx, peer.AddrInfo{ID: hRelay.ID(), Addrs: hRelay.Addrs()}))
	_, err = relayclient.Reserve(ctx, hServer, peer.AddrInfo{ID: hRelay.ID(), Addrs: hRelay.Addrs()})
	require.NoError(t, err)
	// Connect client to relay
	require.NoError(t, hClient.Connect(ctx, peer.AddrInfo{ID: hRelay.ID(), Addrs: hRelay.Addrs()}))
	_, err = relayclient.Reserve(ctx, hClient, peer.AddrInfo{ID: hRelay.ID(), Addrs: hRelay.Addrs()})
	require.NoError(t, err)

	// Build relay multiaddr to server
	base := hRelay.Addrs()[0]
	relayPart, _ := ma.NewMultiaddr("/p2p/" + hRelay.ID().String())
	circuit, _ := ma.NewMultiaddr("/p2p-circuit")
	serverPart, _ := ma.NewMultiaddr("/p2p/" + hServer.ID().String())
	relayToServer := base.Encapsulate(relayPart).Encapsulate(circuit).Encapsulate(serverPart)
	serverInfo, err := peer.AddrInfoFromP2pAddr(relayToServer)
	require.NoError(t, err)

	// Connect client to server via relay
	if sw, ok := hClient.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(hServer.ID())
	}
	require.NoError(t, hClient.Connect(ctx, *serverInfo))

	// Open stream via relay
	allowCtx := network.WithAllowLimitedConn(ctx, string(constant.StellarFileProtocol))
	stream, err := hClient.NewStream(allowCtx, hServer.ID(), protocol.ID(constant.StellarFileProtocol))
	require.NoError(t, err)
	defer stream.Close()

	client := NewClient(hClient.ID().String(), stream)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())

	entries, err := client.List("/", false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
}
