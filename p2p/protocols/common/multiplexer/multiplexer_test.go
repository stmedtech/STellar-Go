package multiplexer

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTCPConnection(t *testing.T) (clientConn, serverConn net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	serverAddr := listener.Addr().String()

	var server net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		server, err = listener.Accept()
		require.NoError(t, err)
	}()

	client, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	<-done
	require.NotNil(t, server)

	return client, server
}

func TestMultiplexerBasic(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)

	// Wait for readLoops to start
	time.Sleep(50 * time.Millisecond)

	// Get control streams
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)

	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)

	// Write from client
	testData := []byte("hello from client")
	_, err = clientControl.Write(testData)
	require.NoError(t, err)

	// Read from server
	buffer := make([]byte, 100)
	n, err := serverControl.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from client", string(buffer[:n]))

	// Write from server
	responseData := []byte("hello from server")
	_, err = serverControl.Write(responseData)
	require.NoError(t, err)

	// Read from client
	responseBuffer := make([]byte, 100)
	n, err = clientControl.Read(responseBuffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from server", string(responseBuffer[:n]))
}
