package service

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"stellar/p2p/protocols/common/multiplexer"
	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClientHelloHandler implements ClientHelloHandler for testing
type testClientHelloHandler struct {
	expectedAckType protocol.HandshakeType
	errorType       protocol.HandshakeType
}

func (h *testClientHelloHandler) CreateHelloPacket(version, clientID string) (*protocol.Packet, error) {
	return protocol.NewHelloPacket(version, clientID)
}

func (h *testClientHelloHandler) ExpectedAckType() protocol.HandshakeType {
	return h.expectedAckType
}

func (h *testClientHelloHandler) ErrorHandshakeType() protocol.HandshakeType {
	return h.errorType
}

// testEventsHandler implements EventsHandler for testing
type testEventsHandler struct {
	mu      sync.Mutex
	events  []*protocol.HandshakePacket
	handler func(*protocol.HandshakePacket)
}

// setupServerControl returns a packet connection on the server control stream and a cleanup func.
func setupServerControl(t *testing.T, conn net.Conn) (*protocol.PacketReadWriteCloser, func()) {
	serverMux := multiplexer.NewMultiplexer(conn)
	cleanup := func() { _ = serverMux.Close() }

	controlStream, err := serverMux.ControlStream()
	require.NoError(t, err)

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return packetConn, cleanup
}

func (h *testEventsHandler) HandleEvent(packet *protocol.HandshakePacket) {
	h.mu.Lock()
	h.events = append(h.events, packet)
	h.mu.Unlock()

	if h.handler != nil {
		h.handler(packet)
	}
}

func (h *testEventsHandler) GetEvents() []*protocol.HandshakePacket {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]*protocol.HandshakePacket, len(h.events))
	copy(result, h.events)
	return result
}

func (h *testEventsHandler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = nil
}

// TestNewBaseClient tests client creation
func TestNewBaseClient(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	assert.Equal(t, "test-client", client.ClientID())
	assert.NotNil(t, client.ControlConn())
	assert.NotNil(t, client.Multiplexer())
	assert.Equal(t, 30*time.Second, client.HandshakeTimeout)
}

// TestNewBaseClientNilConnection tests nil connection handling
func TestNewBaseClientNilConnection(t *testing.T) {
	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", nil, handler, nil)
	assert.Nil(t, client)
}

// TestNewBaseClientEmptyID tests empty client ID handling
func TestNewBaseClientEmptyID(t *testing.T) {
	clientConn, _ := setupTCPConnection(t)
	defer clientConn.Close()

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("", clientConn, handler, nil)
	assert.Nil(t, client)
}

// TestNewBaseClientNilHandler tests nil handler handling
func TestNewBaseClientNilHandler(t *testing.T) {
	clientConn, _ := setupTCPConnection(t)
	defer clientConn.Close()

	client := NewBaseClient("test-client", clientConn, nil, nil)
	assert.Nil(t, client)
}

// TestBaseClientConnect tests client connection
func TestBaseClientConnect(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	// Set up server to respond with ack using multiplexer to match client
	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	serverReady := make(chan struct{})
	go func() {
		defer close(serverReady)
		// Read hello
		_, err := serverPacketConn.ReadPacket()
		if err != nil {
			return
		}

		// Send ack
		ackPacket, err := protocol.NewHelloAckPacket()
		if err != nil {
			return
		}
		_ = serverPacketConn.WritePacket(ackPacket)
	}()

	// Wait a bit for server to be ready
	time.Sleep(50 * time.Millisecond)

	err := client.Connect()
	require.NoError(t, err)
	assert.True(t, client.IsHandshakeDone())

	// Wait for server goroutine
	select {
	case <-serverReady:
	case <-time.After(1 * time.Second):
		t.Fatal("Server goroutine did not complete")
	}

	// Close client to stop background goroutines before test ends
	client.Close()
}

// TestBaseClientConnectAlreadyConnected tests double connect prevention
func TestBaseClientConnectAlreadyConnected(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// First connect
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)
		ackPacket, err := protocol.NewHelloAckPacket()
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(ackPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	require.NoError(t, err)

	// Second connect should fail
	err = client.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already connected")

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientConnectConcurrent tests concurrent connect prevention
func TestBaseClientConnectConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Set up server to respond multiple times
	go func() {
		for i := 0; i < 2; i++ {
			_, err := serverPacketConn.ReadPacket()
			if err != nil {
				return
			}
			ackPacket, err := protocol.NewHelloAckPacket()
			if err != nil {
				return
			}
			serverPacketConn.WritePacket(ackPacket)
		}
	}()

	// Try concurrent connects
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()
		errors <- client.Connect()
	}()

	go func() {
		defer wg.Done()
		errors <- client.Connect()
	}()

	wg.Wait()
	close(errors)

	// One should succeed, one should fail
	successCount := 0
	alreadyConnectedCount := 0
	for err := range errors {
		if err == nil {
			successCount++
		} else if err != nil && err.Error() == "already connected" {
			alreadyConnectedCount++
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, alreadyConnectedCount)

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientConnectTimeout tests connection timeout
func TestBaseClientConnectTimeout(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	client.HandshakeTimeout = 100 * time.Millisecond

	time.Sleep(100 * time.Millisecond)

	// Don't send ack, should timeout
	err := client.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientConnectWrongAckType tests wrong ack type
func TestBaseClientConnectWrongAckType(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Send wrong packet type
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)

		// Send wrong type
		wrongPacket, err := protocol.NewHelloPacket("1.0", "server")
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(wrongPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected type")

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientConnectErrorResponse tests error response handling
func TestBaseClientConnectErrorResponse(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Send error response
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)

		// Send error
		errorPacket, err := protocol.NewErrorPacket("TEST_ERROR", "test error message")
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(errorPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TEST_ERROR")
	assert.Contains(t, err.Error(), "test error message")

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientSendRequest tests sending requests
func TestBaseClientSendRequest(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	// Server control plane
	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Handle server side: hello ack, then request/response
	requestReceived := make(chan struct{})
	go func() {
		// Read hello
		if _, err := serverPacketConn.ReadPacket(); err != nil {
			return
		}
		ackPacket, err := protocol.NewHelloAckPacket()
		if err != nil {
			return
		}
		_ = serverPacketConn.WritePacket(ackPacket)

		// Read request
		if _, err := serverPacketConn.ReadPacket(); err != nil {
			return
		}
		close(requestReceived)

		// Send response
		responsePacket, err := protocol.NewProxyListResponsePacket([]protocol.ProxyInfo{})
		if err != nil {
			return
		}
		_ = serverPacketConn.WritePacket(responsePacket)
	}()

	err := client.Connect()
	require.NoError(t, err)

	// Send request
	requestPacket, err := protocol.NewProxyListPacket()
	require.NoError(t, err)

	match := func(h *protocol.HandshakePacket) (bool, error) {
		return h.Type == protocol.HandshakeTypeProxyListResp, nil
	}

	ctx := context.Background()
	response, err := client.SendRequest(ctx, requestPacket, match)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, protocol.HandshakeTypeProxyListResp, response.Type)

	<-requestReceived

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientSendRequestContextCancel tests context cancellation
func TestBaseClientSendRequestContextCancel(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Connect
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)
		ackPacket, err := protocol.NewHelloAckPacket()
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(ackPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	require.NoError(t, err)

	// Send request with canceling context
	requestPacket, err := protocol.NewProxyListPacket()
	require.NoError(t, err)

	match := func(h *protocol.HandshakePacket) (bool, error) {
		return h.Type == protocol.HandshakeTypeProxyListResp, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	response, err := client.SendRequest(ctx, requestPacket, match)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Nil(t, response)

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientSendRequestNotConnected tests sending request without connection
func TestBaseClientSendRequestNotConnected(t *testing.T) {
	clientConn, _ := setupTCPConnection(t)
	defer clientConn.Close()

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	requestPacket, err := protocol.NewProxyListPacket()
	require.NoError(t, err)

	match := func(h *protocol.HandshakePacket) (bool, error) {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	response, err := client.SendRequest(ctx, requestPacket, match)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, response)
}

// TestBaseClientGetOrCreateStream tests stream creation
func TestBaseClientGetOrCreateStream(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)

	// Stream ID 0 should fail
	stream, err := client.GetOrCreateStream(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid stream ID 0")
	assert.Nil(t, stream)

	// Valid stream ID should work
	stream, err = client.GetOrCreateStream(1)
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Type assert to check ID
	streamTyped, ok := stream.(*multiplexer.Stream)
	require.True(t, ok)
	assert.Equal(t, uint32(1), streamTyped.ID)

	// Getting same stream should return existing
	stream2, err := client.GetOrCreateStream(1)
	require.NoError(t, err)
	require.NotNil(t, stream2)

	stream2Typed, ok := stream2.(*multiplexer.Stream)
	require.True(t, ok)
	assert.Equal(t, streamTyped.ID, stream2Typed.ID)

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientGetOrCreateStreamConcurrent tests concurrent stream creation
func TestBaseClientGetOrCreateStreamConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)

	var wg sync.WaitGroup
	streams := make(chan io.ReadWriteCloser, 10)

	// Create streams concurrently
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			stream, err := client.GetOrCreateStream(id)
			if err == nil {
				streams <- stream
			}
		}(uint32(i))
	}

	wg.Wait()
	close(streams)

	// All streams should be created
	count := 0
	for range streams {
		count++
	}
	assert.Equal(t, 10, count)

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientEventsHandler tests events handling
func TestBaseClientEventsHandler(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	eventsHandler := &testEventsHandler{}
	client := NewBaseClient("test-client", clientConn, handler, eventsHandler)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Connect
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)
		ackPacket, err := protocol.NewHelloAckPacket()
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(ackPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	require.NoError(t, err)

	// Send unmatched packet (should go to events)
	go func() {
		// Send unmatched packet
		eventPacket, err := protocol.NewProxyListResponsePacket([]protocol.ProxyInfo{})
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(eventPacket)
		require.NoError(t, err)
	}()

	// Wait for event
	time.Sleep(200 * time.Millisecond)

	events := eventsHandler.GetEvents()
	assert.GreaterOrEqual(t, len(events), 1)

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientClose tests client cleanup
func TestBaseClientClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)

	// Close again should be safe
	err = client.Close()
	assert.NoError(t, err)

	// No additional cleanup needed - already closed
}

// TestBaseClientCloseNil tests closing nil client
func TestBaseClientCloseNil(t *testing.T) {
	var client *BaseClient
	err := client.Close()
	assert.NoError(t, err)
}

// TestBaseClientClientID tests client ID retrieval
func TestBaseClientClientID(t *testing.T) {
	clientConn, _ := setupTCPConnection(t)
	t.Cleanup(func() { clientConn.Close() })

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client-456", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	assert.Equal(t, "test-client-456", client.ClientID())
}

// TestBaseClientClientIDNil tests client ID on nil client
func TestBaseClientClientIDNil(t *testing.T) {
	var client *BaseClient
	assert.Empty(t, client.ClientID())
}

// TestBaseClientIsHandshakeDone tests handshake status
func TestBaseClientIsHandshakeDone(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)

	// Before connect
	assert.False(t, client.IsHandshakeDone())

	time.Sleep(100 * time.Millisecond)

	serverPacketConn, cleanup := setupServerControl(t, serverConn)
	t.Cleanup(cleanup)

	// Connect
	go func() {
		_, err := serverPacketConn.ReadPacket()
		require.NoError(t, err)
		ackPacket, err := protocol.NewHelloAckPacket()
		require.NoError(t, err)
		err = serverPacketConn.WritePacket(ackPacket)
		require.NoError(t, err)
	}()

	err := client.Connect()
	require.NoError(t, err)

	// After connect
	assert.True(t, client.IsHandshakeDone())

	t.Cleanup(func() { client.Close() })
}

// TestBaseClientIsHandshakeDoneNil tests handshake status on nil client
func TestBaseClientIsHandshakeDoneNil(t *testing.T) {
	var client *BaseClient
	assert.False(t, client.IsHandshakeDone())
}

// TestBaseClientEnsureConnected tests connection check
func TestBaseClientEnsureConnected(t *testing.T) {
	clientConn, _ := setupTCPConnection(t)
	t.Cleanup(func() { clientConn.Close() })

	handler := &testClientHelloHandler{
		expectedAckType: protocol.HandshakeTypeHelloAck,
		errorType:       protocol.HandshakeTypeError,
	}

	client := NewBaseClient("test-client", clientConn, handler, nil)
	require.NotNil(t, client)
	t.Cleanup(func() { client.Close() })

	// Not connected
	err := client.EnsureConnected()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	// Connect
	clientConn2, serverConn2 := setupTCPConnection(t)
	t.Cleanup(func() {
		clientConn2.Close()
		serverConn2.Close()
	})

	client2 := NewBaseClient("test-client", clientConn2, handler, nil)
	require.NotNil(t, client2)
	t.Cleanup(func() { client2.Close() })

	time.Sleep(100 * time.Millisecond)

	serverPacketConn2, cleanup2 := setupServerControl(t, serverConn2)
	t.Cleanup(cleanup2)

	go func() {
		_, err := serverPacketConn2.ReadPacket()
		require.NoError(t, err)
		ackPacket, err := protocol.NewHelloAckPacket()
		require.NoError(t, err)
		err = serverPacketConn2.WritePacket(ackPacket)
		require.NoError(t, err)
	}()

	err = client2.Connect()
	require.NoError(t, err)

	// Connected
	err = client2.EnsureConnected()
	assert.NoError(t, err)
}

// TestBaseClientEnsureConnectedNil tests connection check on nil client
func TestBaseClientEnsureConnectedNil(t *testing.T) {
	var client *BaseClient
	err := client.EnsureConnected()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is nil")
}
