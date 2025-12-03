package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/proxy/service"

	"github.com/libp2p/go-libp2p/core/network"
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
	proxy2 := NewProxyManager(node2)

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

	// Wait for proxy to be ready
	time.Sleep(500 * time.Millisecond)

	// Test proxy connection
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
	require.NoError(t, err)
	defer conn.Close()

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, serverResponse, string(buffer[:n]))

	// Clean up
	proxy1.Close(proxyPort)
}

// TestProxyServiceHandshake tests the handshake process
func TestProxyServiceHandshake(t *testing.T) {
	node1, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node1.Close()

	node2, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node2.Close()

	// Bind proxy handlers
	NewProxyManager(node1)
	NewProxyManager(node2)

	// Connect nodes
	peerInfo := node2.Host.Peerstore().PeerInfo(node2.ID())
	err = node1.Host.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(500 * time.Millisecond)

	// Create a stream and test handshake
	stream, err := node1.Host.NewStream(context.Background(), node2.ID(), constant.StellarProxyProtocol)
	require.NoError(t, err)
	defer stream.Close()

	// Create server on node2 side
	srv := service.NewServer(stream)
	require.NotNil(t, srv)

	// Create client on node1 side
	clientID := node1.ID().String()
	client := service.NewClient(clientID, stream)
	require.NotNil(t, client)

	// Test handshake
	err = client.Connect()
	assert.NoError(t, err)

	err = srv.Accept()
	assert.NoError(t, err)

	assert.Equal(t, clientID, srv.ClientID())
}

// TestProxyServiceMultipleStreams tests multiple concurrent proxy streams
func TestProxyServiceMultipleStreams(t *testing.T) {
	node1, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node1.Close()

	node2, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer node2.Close()

	proxy1 := NewProxyManager(node1)
	NewProxyManager(node2)

	peerInfo := node2.Host.Peerstore().PeerInfo(node2.ID())
	err = node1.Host.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Start multiple servers
	numServers := 3
	servers := make([]net.Listener, numServers)
	for i := 0; i < numServers; i++ {
		port := 9000 + i
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		servers[i] = listener

		go func(l net.Listener, p int) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				conn.Write([]byte(fmt.Sprintf("Response from server %d", p)))
				conn.Close()
			}
		}(listener, port)
	}

	time.Sleep(200 * time.Millisecond)

	// Create multiple proxies
	proxies := make([]*TcpProxyService, numServers)
	for i := 0; i < numServers; i++ {
		proxyPort := uint64(9100 + i)
		serverAddr := fmt.Sprintf("127.0.0.1:%d", 9000+i)
		proxy, err := proxy1.Proxy(node2.ID(), proxyPort, serverAddr)
		require.NoError(t, err)
		proxies[i] = proxy
	}

	time.Sleep(500 * time.Millisecond)

	// Test all proxies
	for i := 0; i < numServers; i++ {
		proxyPort := uint64(9100 + i)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)
		expected := fmt.Sprintf("Response from server %d", 9000+i)
		assert.Equal(t, expected, string(buffer[:n]))
		conn.Close()
	}

	// Clean up
	for i := 0; i < numServers; i++ {
		servers[i].Close()
		proxy1.Close(uint64(9100 + i))
	}
}

// mockStreamHandler is a helper to simulate stream handling
func mockStreamHandler(handler func(network.Stream)) func(network.Stream) {
	return handler
}

