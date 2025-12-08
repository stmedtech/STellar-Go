package protocol

import (
	"net"
	"testing"
	"time"

	"stellar/p2p/protocols/common/multiplexer"

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

func TestPacketThroughMultiplexer(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := multiplexer.NewMultiplexer(clientConn)
	serverMux := multiplexer.NewMultiplexer(serverConn)

	// Wait for readLoops
	time.Sleep(50 * time.Millisecond)

	// Get control streams
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)

	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)

	// Create packet RWC
	clientPacketRWC := NewPacketReadWriteCloser(clientControl)
	serverPacketRWC := NewPacketReadWriteCloser(serverControl)

	// Create and send hello packet
	helloPacket, err := NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)

	err = clientPacketRWC.WritePacket(helloPacket)
	require.NoError(t, err)

	// Read packet on server side
	receivedPacket, err := serverPacketRWC.ReadPacket()
	require.NoError(t, err)
	require.NotNil(t, receivedPacket)

	// Unmarshal handshake
	handshake, err := UnmarshalHandshakePacket(receivedPacket)
	require.NoError(t, err)
	require.NotNil(t, handshake)

	assert.Equal(t, HandshakeTypeHello, handshake.Type)

	var helloPayload HelloPayload
	err = handshake.UnmarshalPayload(&helloPayload)
	require.NoError(t, err)
	assert.Equal(t, "1.0", helloPayload.Version)
	assert.Equal(t, "test-client", helloPayload.ClientID)
}
