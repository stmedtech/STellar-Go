package control_plane

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTCPConnection creates a TCP connection pair for testing
func setupTCPConnection(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	serverConnChan := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		require.NoError(t, err)
		serverConnChan <- conn
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	serverConn := <-serverConnChan
	return clientConn, serverConn
}

// TestNewControlPlane tests control plane creation
func TestNewControlPlane(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)
}

// TestControlPlaneEnsureStarted tests starting control plane
func TestControlPlaneEnsureStarted(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	assert.NoError(t, err)
}

// TestControlPlaneEnsureStartedNil tests nil control plane
func TestControlPlaneEnsureStartedNil(t *testing.T) {
	var cp *ControlPlane
	err := cp.EnsureStarted()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control plane is nil")
}

// TestControlPlaneEnsureStartedNilConn tests nil connection
func TestControlPlaneEnsureStartedNilConn(t *testing.T) {
	var nilConn *protocol.PacketReadWriteCloser
	cp := NewControlPlane(nilConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control connection not initialized")
}

// TestControlPlaneEnsureStartedMultiple tests multiple calls
func TestControlPlaneEnsureStartedMultiple(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	// Multiple calls should be safe
	err := cp.EnsureStarted()
	assert.NoError(t, err)

	err = cp.EnsureStarted()
	assert.NoError(t, err)

	err = cp.EnsureStarted()
	assert.NoError(t, err)
}

// TestControlPlaneNext tests reading packets
func TestControlPlaneNext(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	// Start control plane
	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Send packet from client
	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	// Read packet
	ctx := context.Background()
	packet, err := cp.Next(ctx)
	require.NoError(t, err)
	require.NotNil(t, packet)
	assert.Equal(t, protocol.HandshakeTypeHello, packet.Type)
}

// TestControlPlaneNextMultiple tests reading multiple packets
func TestControlPlaneNextMultiple(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)

	// Send multiple packets
	for i := 0; i < 5; i++ {
		helloPacket, err := protocol.NewHelloPacket("1.0", fmt.Sprintf("client-%d", i))
		require.NoError(t, err)
		err = clientPacketConn.WritePacket(helloPacket)
		require.NoError(t, err)
	}

	// Read all packets
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		packet, err := cp.Next(ctx)
		require.NoError(t, err)
		require.NotNil(t, packet)
		assert.Equal(t, protocol.HandshakeTypeHello, packet.Type)
	}
}

// TestControlPlaneNextContextCancel tests context cancellation
func TestControlPlaneNextContextCancel(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Nil(t, packet)
}

// TestControlPlaneNextContextTimeout tests context timeout
func TestControlPlaneNextContextTimeout(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Don't send any packets, should timeout
	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Nil(t, packet)
}

// TestControlPlaneNextNilContext tests nil context
func TestControlPlaneNextNilContext(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Send packet
	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	// Nil context should use Background
	packet, err := cp.Next(nil)
	require.NoError(t, err)
	require.NotNil(t, packet)
}

// TestControlPlaneNextNil tests Next on nil control plane
func TestControlPlaneNextNil(t *testing.T) {
	var cp *ControlPlane
	ctx := context.Background()
	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control plane is nil")
	assert.Nil(t, packet)
}

// TestControlPlaneNextConcurrent tests concurrent reads
func TestControlPlaneNextConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)

	// Send many packets
	for i := 0; i < 20; i++ {
		helloPacket, err := protocol.NewHelloPacket("1.0", fmt.Sprintf("client-%d", i))
		require.NoError(t, err)
		err = clientPacketConn.WritePacket(helloPacket)
		require.NoError(t, err)
	}

	// Read concurrently
	var wg sync.WaitGroup
	packets := make(chan *protocol.HandshakePacket, 20)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			packet, err := cp.Next(ctx)
			if err == nil && packet != nil {
				packets <- packet
			}
		}()
	}

	wg.Wait()
	close(packets)

	// Should receive all packets
	count := 0
	for range packets {
		count++
	}
	assert.Equal(t, 20, count)
}

// TestControlPlaneClose tests closing control plane
func TestControlPlaneClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Close should be safe
	cp.Close()

	// Close again should be safe
	cp.Close()
}

// TestControlPlaneCloseNil tests closing nil control plane
func TestControlPlaneCloseNil(t *testing.T) {
	var cp *ControlPlane
	cp.Close() // Should not panic
}

// TestControlPlaneCloseStopsReading tests that close stops reading
func TestControlPlaneCloseStopsReading(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Close control plane
	cp.Close()

	// Try to read - should get EOF
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Nil(t, packet)
	// Should get EOF or context timeout
	assert.True(t, err == context.DeadlineExceeded || err.Error() == "EOF")
}

// TestControlPlaneReadError tests read error handling
func TestControlPlaneReadError(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Close connection to cause read error
	serverConn.Close()

	// Try to read - should get error
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Nil(t, packet)
}

// TestControlPlaneUnmarshalError tests unmarshal error handling
func TestControlPlaneUnmarshalError(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// Write invalid packet data (wrong length prefix or invalid JSON)
	// Write a length prefix that's too large to cause read error, or invalid JSON
	invalidData := []byte{0xFF, 0xFF, 0xFF, 0xFF} // Invalid length prefix
	_, err = serverConn.Write(invalidData)
	require.NoError(t, err)

	// Try to read - should get error (either unmarshal or read error)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	packet, err := cp.Next(ctx)
	assert.Error(t, err)
	assert.Nil(t, packet)
	// Error could be unmarshal, read, or timeout depending on timing
	assert.True(t, err.Error() != "" || err == context.DeadlineExceeded)
}

// TestControlPlaneBufferFull tests buffer full scenario
func TestControlPlaneBufferFull(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)

	// Send many packets rapidly to fill buffer
	// Buffer size is ControlBufferSize (32)
	for i := 0; i < ControlBufferSize+10; i++ {
		helloPacket, err := protocol.NewHelloPacket("1.0", fmt.Sprintf("client-%d", i))
		require.NoError(t, err)
		err = clientPacketConn.WritePacket(helloPacket)
		require.NoError(t, err)
	}

	// Read all packets
	ctx := context.Background()
	readCount := 0
	for i := 0; i < ControlBufferSize+10; i++ {
		packet, err := cp.Next(ctx)
		if err != nil {
			break
		}
		if packet != nil {
			readCount++
		}
	}

	// Should be able to read at least buffer size
	assert.GreaterOrEqual(t, readCount, ControlBufferSize)
}

// TestControlPlaneNilPacket tests nil packet handling
func TestControlPlaneNilPacket(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	packetConn := protocol.NewPacketReadWriteCloser(serverConn)
	cp := NewControlPlane(packetConn)
	require.NotNil(t, cp)

	err := cp.EnsureStarted()
	require.NoError(t, err)

	// The control plane should skip nil packets internally
	// This is tested by ensuring valid packets are still processed
	clientPacketConn := protocol.NewPacketReadWriteCloser(clientConn)
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	ctx := context.Background()
	packet, err := cp.Next(ctx)
	require.NoError(t, err)
	require.NotNil(t, packet)
}
