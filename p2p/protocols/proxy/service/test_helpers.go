package service

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupTCPConnection creates a TCP connection pair for testing
func setupTCPConnection(t testing.TB) (clientConn, serverConn net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	serverAddr := listener.Addr().String()

	var server net.Conn
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		server, err = listener.Accept()
		require.NoError(t, err)
	}()

	client, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	wg.Wait()
	require.NotNil(t, server)

	return client, server
}
