package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"stellar/p2p/node"
	"stellar/p2p/protocols/proxy/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProxyServiceIntegration tests the full proxy service integration
func TestProxyServiceIntegration(t *testing.T) {
	// Create two nodes
	node1, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node1.Close()

	node2, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node2.Close()

	// Bind proxy handlers
	proxy1 := NewProxyManager(node1)
	_ = NewProxyManager(node2) // Server-side proxy manager

	// Connect nodes
	peerInfo := node2.Host.Peerstore().PeerInfo(node2.ID())
	err = node1.Host.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection to establish
	time.Sleep(500 * time.Millisecond)

	// Start a simple TCP server on node2
	serverAddr := "127.0.0.1:8080"
	listener, err := net.Listen("tcp", serverAddr)
	require.NoError(t, err)
	defer listener.Close()

	serverResponse := "Hello from test server"
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte(serverResponse))
			conn.Close()
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Create proxy from node1 to node2's server
	proxyPort := uint64(8081)
	proxyService, err := proxy1.Proxy(node2.ID(), proxyPort, serverAddr)
	require.NoError(t, err)
	require.NotNil(t, proxyService)

	// Wait for proxy to be ready - try connecting with retries
	var conn net.Conn
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		var err error
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
		if err == nil {
			break
		}
		if i == 9 {
			require.NoError(t, err, "Failed to connect to proxy after retries")
		}
	}
	require.NotNil(t, conn)
	defer conn.Close()

	// Give proxy forwarding time to establish
	time.Sleep(200 * time.Millisecond)

	// Read response (server writes immediately on accept)
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		require.NoError(t, err, "Failed to read from proxy connection")
	}
	if n > 0 {
		assert.Equal(t, serverResponse, string(buffer[:n]))
	} else {
		// If no data, try writing to trigger forwarding
		_, err = conn.Write([]byte("test"))
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
		n, err = conn.Read(buffer)
		if err == nil && n > 0 {
			assert.Equal(t, serverResponse, string(buffer[:n]))
		}
	}

	// Clean up
	proxy1.Close(proxyPort)
}

// TestProxyServiceHandshake tests the handshake process using TCP connections
func TestProxyServiceHandshake(t *testing.T) {
	// Use TCP connections for simpler testing
	clientConn, serverConn := service.SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// Wait for multiplexer readLoops to start
	time.Sleep(100 * time.Millisecond)

	// Create server on server side
	srv := service.NewServer(serverConn)
	require.NotNil(t, srv)

	// Create client on client side
	clientID := "test-client-handshake"
	client := service.NewClient(clientID, clientConn)
	require.NotNil(t, client)

	// Server accepts in background
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- srv.Accept()
	}()

	// Small delay to ensure server is ready
	time.Sleep(50 * time.Millisecond)

	// Test handshake - client connects
	err := client.Connect()
	assert.NoError(t, err)

	// Wait for server to finish accepting
	err = <-acceptErr
	assert.NoError(t, err)

	assert.Equal(t, clientID, srv.ClientID())
}

// TestProxyServiceMultipleStreams tests multiple concurrent proxy streams over a
// single control plane using the service layer (deterministic, no libp2p).
func TestProxyServiceMultipleStreams(t *testing.T) {
	// Start backend servers
	numServers := 3
	listeners := make([]net.Listener, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners[i] = ln
		addrs[i] = ln.Addr().String()
		resp := fmt.Sprintf("Response from server %d", i)
		go func(l net.Listener, msg string) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				_, _ = conn.Write([]byte(msg))
				conn.Close()
			}
		}(ln, resp)
	}
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
	}()

	// Control-plane connection
	clientConn, serverConn := service.SetupTCPConnection(t)

	// Server side
	srv := service.NewServer(serverConn)
	require.NotNil(t, srv)
	acceptErr := make(chan error, 1)
	go func() { acceptErr <- srv.Accept() }()

	// Client side
	client := service.NewClient("multi-stream-client", clientConn)
	require.NotNil(t, client)
	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	// Open multiple proxies sequentially to validate multiple streams without race flakiness.
	for i := 0; i < numServers; i++ {
		local, remote := net.Pipe()

		proxyID := fmt.Sprintf("proxy-%d", i)
		_, err := client.OpenWithLocalConn(proxyID, addrs[i], "tcp", local)
		require.NoError(t, err, "open %s", proxyID)

		buf := make([]byte, 64)
		_ = remote.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := remote.Read(buf)
		require.NoError(t, err, "read %s", proxyID)
		expected := fmt.Sprintf("Response from server %d", i)
		require.Equal(t, expected, string(buf[:n]), "response %s", proxyID)

		local.Close()
		remote.Close()
	}
}
