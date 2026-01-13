package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"stellar/p2p/protocols/common/multiplexer"
	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTCPConnection creates a TCP connection pair for testing
// Uses t.Cleanup() for proper resource management
func setupTCPConnection(t *testing.T) (net.Conn, net.Conn) {
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

// setupClientControl returns packet connection on client control stream with cleanup.
func setupClientControl(t *testing.T, conn net.Conn) (*protocol.PacketReadWriteCloser, func()) {
	clientMux := multiplexer.NewMultiplexer(conn)
	cleanup := func() { _ = clientMux.Close() }

	controlStream, err := clientMux.ControlStream()
	require.NoError(t, err)

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return packetConn, cleanup
}

// testHandshakeHandler implements HandshakeHandler for testing
type testHandshakeHandler struct {
	expectedType protocol.HandshakeType
	ackType      protocol.HandshakeType
}

func (h *testHandshakeHandler) ExpectedHandshakeType() protocol.HandshakeType {
	return h.expectedType
}

func (h *testHandshakeHandler) UnmarshalHelloPayload(payload []byte) (interface{}, error) {
	var helloPayload protocol.HelloPayload
	if err := json.Unmarshal(payload, &helloPayload); err != nil {
		return nil, fmt.Errorf("unmarshal hello payload: %w", err)
	}
	return helloPayload, nil
}

func (h *testHandshakeHandler) CreateAckPacket() (*protocol.Packet, error) {
	return protocol.NewHelloAckPacket()
}

func (h *testHandshakeHandler) ExtractClientID(helloPayload interface{}) string {
	payload, ok := helloPayload.(protocol.HelloPayload)
	if !ok {
		return ""
	}
	return payload.ClientID
}

// testPacketDispatcher implements PacketDispatcher for testing
type testPacketDispatcher struct {
	mu       sync.Mutex
	packets  []*protocol.HandshakePacket
	dispatch func(ctx context.Context, packet *protocol.HandshakePacket) error
}

func (d *testPacketDispatcher) Dispatch(ctx context.Context, packet *protocol.HandshakePacket) error {
	d.mu.Lock()
	d.packets = append(d.packets, packet)
	d.mu.Unlock()

	if d.dispatch != nil {
		return d.dispatch(ctx, packet)
	}
	return nil
}

func (d *testPacketDispatcher) GetPackets() []*protocol.HandshakePacket {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]*protocol.HandshakePacket, len(d.packets))
	copy(result, d.packets)
	return result
}

func (d *testPacketDispatcher) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.packets = nil
}

// TestNewBaseServer tests server creation
func TestNewBaseServer(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
		ackType:      protocol.HandshakeTypeHelloAck,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	assert.NotNil(t, server.ControlConn())
	assert.NotNil(t, server.Multiplexer())
	assert.Equal(t, 30*time.Second, server.HandshakeTimeout)
}

// TestNewBaseServerNilConnection tests nil connection handling
func TestNewBaseServerNilConnection(t *testing.T) {
	handler := &testHandshakeHandler{}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(nil, handler, dispatcher)
	assert.Nil(t, server)
	// No cleanup needed for nil server
}

// TestNewBaseServerNilHandler tests nil handler handling
func TestNewBaseServerNilHandler(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	dispatcher := &testPacketDispatcher{}

	// This should still work, but Accept will fail
	server := NewBaseServer(serverConn, nil, dispatcher)
	// Note: NewBaseServer doesn't check for nil handler, so server is created
	// but Accept will fail
	if server != nil {
		t.Cleanup(func() { server.Close() })
		err := server.Accept()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handshake handler not set")
	}
}

// TestBaseServerAccept tests handshake acceptance
func TestBaseServerAccept(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	// Wait for multiplexer to be ready
	time.Sleep(100 * time.Millisecond)

	// Send hello from client side
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)

	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)

	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	// Server accepts
	err = server.Accept()
	require.NoError(t, err)

	assert.Equal(t, "test-client", server.ClientID())
	assert.True(t, server.IsHandshakeDone())
}

// TestBaseServerAcceptAlreadyAccepted tests double accept prevention
func TestBaseServerAcceptAlreadyAccepted(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	// Wait for multiplexer
	time.Sleep(100 * time.Millisecond)

	// First accept
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	// Second accept should fail
	err = server.Accept()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already accepted")
}

// TestBaseServerAcceptConcurrent tests concurrent accept prevention
func TestBaseServerAcceptConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	time.Sleep(100 * time.Millisecond)

	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	// Try concurrent accepts
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()
		errors <- server.Accept()
	}()

	go func() {
		defer wg.Done()
		errors <- server.Accept()
	}()

	wg.Wait()
	close(errors)

	// One should succeed, one should fail
	successCount := 0
	alreadyAcceptedCount := 0
	for err := range errors {
		if err == nil {
			successCount++
		} else if err != nil && err.Error() == "already accepted" {
			alreadyAcceptedCount++
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, alreadyAcceptedCount)

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerAcceptTimeout tests handshake timeout
func TestBaseServerAcceptTimeout(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	server.HandshakeTimeout = 100 * time.Millisecond

	time.Sleep(100 * time.Millisecond)

	// Don't send hello, should timeout
	err := server.Accept()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerAcceptWrongType tests wrong handshake type
func TestBaseServerAcceptWrongType(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	time.Sleep(100 * time.Millisecond)

	// Send wrong packet type
	wrongPacket, err := protocol.NewHelloAckPacket()
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(wrongPacket)
	require.NoError(t, err)

	err = server.Accept()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected type")

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerNextStreamID tests stream ID generation
func TestBaseServerNextStreamID(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	// First stream ID should be 1 (0 is reserved for control)
	id1 := server.NextStreamID()
	assert.Equal(t, uint32(1), id1)

	// Subsequent IDs should increment
	id2 := server.NextStreamID()
	assert.Equal(t, uint32(2), id2)

	id3 := server.NextStreamID()
	assert.Equal(t, uint32(3), id3)
}

// TestBaseServerNextStreamIDConcurrent tests concurrent stream ID generation
func TestBaseServerNextStreamIDConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	var wg sync.WaitGroup
	ids := make(chan uint32, 100)

	// Generate many IDs concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- server.NextStreamID()
		}()
	}

	wg.Wait()
	close(ids)

	// Collect all IDs
	idSet := make(map[uint32]bool)
	for id := range ids {
		assert.False(t, idSet[id], "duplicate stream ID: %d", id)
		assert.NotEqual(t, uint32(0), id, "stream ID 0 should not be generated")
		idSet[id] = true
	}

	// Should have 100 unique IDs
	assert.Equal(t, 100, len(idSet))

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerServe tests serving control packets
func TestBaseServerServe(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	time.Sleep(100 * time.Millisecond)

	// Accept first
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	// Start serving
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx)
	}()

	// Send a control packet
	testPacket, err := protocol.NewProxyListPacket()
	require.NoError(t, err)
	err = clientPacketConn.WritePacket(testPacket)
	require.NoError(t, err)

	// Wait for dispatch
	time.Sleep(200 * time.Millisecond)

	// Check dispatcher received packet
	packets := dispatcher.GetPackets()
	assert.GreaterOrEqual(t, len(packets), 1)

	// Cancel context
	cancel()

	// Wait for serve to finish
	select {
	case err := <-serveErr:
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerServeWithoutAccept tests serving without accept
func TestBaseServerServeWithoutAccept(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Serve(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not accepted")
}

// TestBaseServerServeNilDispatcher tests serving with nil dispatcher
func TestBaseServerServeNilDispatcher(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}

	server := NewBaseServer(serverConn, handler, nil)
	require.NotNil(t, server)

	time.Sleep(100 * time.Millisecond)

	// Accept first
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = server.Serve(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "packet dispatcher not set")

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerClose tests server cleanup
func TestBaseServerClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	err := server.Close()
	assert.NoError(t, err)

	// Close again should be safe
	err = server.Close()
	assert.NoError(t, err)
}

// TestBaseServerCloseNil tests closing nil server
func TestBaseServerCloseNil(t *testing.T) {
	var server *BaseServer
	err := server.Close()
	assert.NoError(t, err)
}

// TestBaseServerClientID tests client ID retrieval
func TestBaseServerClientID(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	// Before accept, should be empty
	assert.Empty(t, server.ClientID())

	time.Sleep(100 * time.Millisecond)

	// Accept
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client-123")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	// After accept, should have client ID
	assert.Equal(t, "test-client-123", server.ClientID())

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerClientIDNil tests client ID on nil server
func TestBaseServerClientIDNil(t *testing.T) {
	var server *BaseServer
	assert.Empty(t, server.ClientID())
}

// TestBaseServerIsHandshakeDone tests handshake status
func TestBaseServerIsHandshakeDone(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)

	// Before accept
	assert.False(t, server.IsHandshakeDone())

	time.Sleep(100 * time.Millisecond)

	// Accept
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	// After accept
	assert.True(t, server.IsHandshakeDone())

	t.Cleanup(func() { server.Close() })
}

// TestBaseServerIsHandshakeDoneNil tests handshake status on nil server
func TestBaseServerIsHandshakeDoneNil(t *testing.T) {
	var server *BaseServer
	assert.False(t, server.IsHandshakeDone())
}

// TestBaseServerDispatcherError tests dispatcher error handling
func TestBaseServerDispatcherError(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testHandshakeHandler{
		expectedType: protocol.HandshakeTypeHello,
	}
	dispatcher := &testPacketDispatcher{
		dispatch: func(ctx context.Context, packet *protocol.HandshakePacket) error {
			return fmt.Errorf("dispatcher error")
		},
	}

	server := NewBaseServer(serverConn, handler, dispatcher)
	require.NotNil(t, server)
	t.Cleanup(func() { server.Close() })

	time.Sleep(100 * time.Millisecond)

	// Accept
	helloPacket, err := protocol.NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)
	clientPacketConn, cleanup := setupClientControl(t, clientConn)
	t.Cleanup(cleanup)
	err = clientPacketConn.WritePacket(helloPacket)
	require.NoError(t, err)

	err = server.Accept()
	require.NoError(t, err)

	// Start serving
	ctx := context.Background()
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx)
	}()

	// Send a packet that will cause dispatcher error
	testPacket, err := protocol.NewProxyListPacket()
	require.NoError(t, err)
	err = clientPacketConn.WritePacket(testPacket)
	require.NoError(t, err)

	// Wait for error
	select {
	case err := <-serveErr:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dispatcher error")
	case <-time.After(1 * time.Second):
		t.Fatal("Serve did not return error")
	}

	t.Cleanup(func() { server.Close() })
}
