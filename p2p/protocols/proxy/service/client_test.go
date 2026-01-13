package service

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)
	assert.Equal(t, "test-client", client.ClientID())
	assert.NotNil(t, client.manager)
	assert.NotNil(t, client.Multiplexer())
	assert.NotNil(t, client.ControlConn())
}

// TestClientConnect tests client connection/handshake
func TestClientConnect(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient("test-client", clientConn)
	server := NewServer(serverConn)

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

	// Wait a bit for handshake to complete
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, "test-client", server.ClientID())
}

// TestClientOpenClose tests proxy open and close
func TestClientOpenClose(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient("test-client", clientConn)
	server := NewServer(serverConn)

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

	// Start a test server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Open proxy
	proxy, err := client.Open("test-proxy", testAddr, "tcp")
	require.NoError(t, err)
	require.NotNil(t, proxy)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Close proxy
	err = client.Close("test-proxy")
	require.NoError(t, err)
}

// TestClientList tests proxy listing
func TestClientList(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient("test-client", clientConn)
	server := NewServer(serverConn)

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

	// Create test server
	testServer, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer testServer.Close()

	testAddr := testServer.Addr().String()

	// Open multiple proxies
	_, err = client.Open("proxy1", testAddr, "tcp")
	require.NoError(t, err)

	_, err = client.Open("proxy2", testAddr, "tcp")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// List proxies
	proxies, err := client.List()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(proxies), 2)
}

// TestClientCloseAll tests closing all proxies
func TestClientCloseAll(t *testing.T) {
	clientConn, serverConn := SetupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	err := client.CloseAll()
	assert.NoError(t, err)
}
