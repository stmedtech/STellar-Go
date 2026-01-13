package service

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServer tests server creation
func TestNewServer(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server := NewServer(serverConn)
	require.NotNil(t, server)
	assert.NotNil(t, server.manager)
	assert.NotNil(t, server.Multiplexer())
	assert.NotNil(t, server.ControlConn())
}

// TestServerAccept tests the handshake acceptance
func TestServerAccept(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server := NewServer(serverConn)
	require.NotNil(t, server)

	// Create client on other side
	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	// Wait for multiplexer readLoops to start
	time.Sleep(100 * time.Millisecond)

	// Server accepts in background (reads hello, sends ack)
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	// Small delay to ensure server is ready to read
	time.Sleep(50 * time.Millisecond)

	// Client connects (sends hello, waits for ack)
	err := client.Connect()
	require.NoError(t, err)

	// Wait for server to finish accepting
	err = <-acceptErr
	require.NoError(t, err)

	assert.Equal(t, "test-client", server.ClientID())
}

// TestServerHandleProxyOpen tests proxy open handling
func TestServerHandleProxyOpen(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server := NewServer(serverConn)
	client := NewClient("test-client", clientConn)

	// Wait for multiplexer readLoops
	time.Sleep(100 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

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

	// Open proxy
	proxy, err := client.Open("test-proxy", testAddr, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy)

	assert.Equal(t, "test-proxy", proxy.ID)
	assert.Equal(t, testAddr, proxy.RemoteAddr)
}

// TestServerHandleProxyList tests proxy list handling
func TestServerHandleProxyList(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server := NewServer(serverConn)
	client := NewClient("test-client", clientConn)

	// Wait for multiplexer readLoops
	time.Sleep(100 * time.Millisecond)

	// Establish connection
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create a test server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Open a proxy
	_, err = client.Open("proxy1", testAddr, "tcp")
	require.NoError(t, err)

	// Wait a bit for server to process
	time.Sleep(200 * time.Millisecond)

	// List proxies
	proxies, err := client.List()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(proxies), 1)
}

// TestServerClose tests server cleanup
func TestServerClose(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server := NewServer(serverConn)
	require.NotNil(t, server)

	err := server.Close()
	assert.NoError(t, err)
}
