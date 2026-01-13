package service

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"stellar/p2p/protocols/common/control_plane"
	"stellar/p2p/protocols/common/multiplexer"
	"stellar/p2p/protocols/common/protocol"
)

// ClientHelloHandler defines the interface for protocol-specific hello packet handling
type ClientHelloHandler interface {
	// CreateHelloPacket creates the hello packet for the protocol
	CreateHelloPacket(version, clientID string) (*protocol.Packet, error)

	// ExpectedAckType returns the expected acknowledgment handshake type
	ExpectedAckType() protocol.HandshakeType

	// ErrorHandshakeType returns the error handshake type for this protocol
	ErrorHandshakeType() protocol.HandshakeType
}

// EventsHandler defines an optional interface for handling unmatched control packets as events
type EventsHandler interface {
	// HandleEvent is called for packets that don't match any pending request
	HandleEvent(packet *protocol.HandshakePacket)
}

// BaseClient provides common client functionality for all protocols
type BaseClient struct {
	// Common fields
	clientID         string
	controlConn      *protocol.PacketReadWriteCloser
	multiplexer      *multiplexer.Multiplexer
	control          *control_plane.ControlPlane
	handshakeDone    bool
	HandshakeTimeout time.Duration
	mu               sync.RWMutex
	connectMu        sync.Mutex // Protects Connect() from concurrent calls
	writeMu          sync.Mutex // Serializes control-plane writes
	pendingMu        sync.Mutex
	pending          map[uint64]*PendingRequest
	nextRequestIDVal uint64

	// Protocol-specific handlers
	helloHandler  ClientHelloHandler
	eventsHandler EventsHandler // Optional: for protocols that need events

	// Dispatcher state
	dispatchOnce   sync.Once
	dispatchCtx    context.Context
	dispatchCancel context.CancelFunc
}

// NewBaseClient creates a new base client with automatic multiplexer setup
func NewBaseClient(clientID string, conn io.ReadWriteCloser, helloHandler ClientHelloHandler, eventsHandler EventsHandler) *BaseClient {
	if conn == nil || clientID == "" || helloHandler == nil {
		return nil
	}

	mux := multiplexer.NewMultiplexer(conn)
	controlStream, err := mux.ControlStream()
	if err != nil {
		return nil
	}

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return &BaseClient{
		clientID:         clientID,
		controlConn:      packetConn,
		multiplexer:      mux,
		HandshakeTimeout: 30 * time.Second,
		control:          control_plane.NewControlPlane(packetConn),
		pending:          make(map[uint64]*PendingRequest),
		helloHandler:     helloHandler,
		eventsHandler:    eventsHandler,
	}
}

// Connect performs handshake and establishes connection
// This method is protected by connectMu to prevent concurrent Connect() calls
func (c *BaseClient) Connect() error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	if c.helloHandler == nil {
		return fmt.Errorf("hello handler not set")
	}

	// Use exclusive lock to prevent concurrent Connect() calls
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	// Check if already connected
	c.mu.RLock()
	alreadyConnected := c.handshakeDone
	c.mu.RUnlock()

	if alreadyConnected {
		return fmt.Errorf("already connected")
	}

	// Create and send hello using handler
	helloPacket, err := c.helloHandler.CreateHelloPacket("1.0", c.clientID)
	if err != nil {
		return fmt.Errorf("create hello: %w", err)
	}

	if err := c.controlConn.WritePacket(helloPacket); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	// Read response with timeout
	response, err := c.readHandshakeWithTimeout(c.HandshakeTimeout)
	if err != nil {
		return err
	}

	// Check for error response
	errorType := c.helloHandler.ErrorHandshakeType()
	if response.Type == errorType {
		var errorPayload protocol.ErrorPayload
		if err := response.UnmarshalPayload(&errorPayload); err == nil {
			return fmt.Errorf("%s: %s", errorPayload.Code, errorPayload.Message)
		}
		return fmt.Errorf("handshake error")
	}

	// Verify ack
	expectedAckType := c.helloHandler.ExpectedAckType()
	if response.Type != expectedAckType {
		return fmt.Errorf("unexpected type: %s, expected %s", response.Type, expectedAckType)
	}

	if err := c.ensureDispatcher(); err != nil {
		return err
	}

	// Mark as connected
	c.mu.Lock()
	c.handshakeDone = true
	c.mu.Unlock()
	return nil
}

// SendRequest sends a request and waits for a matching response
func (c *BaseClient) SendRequest(ctx context.Context, requestPacket *protocol.Packet, match Matcher) (*protocol.HandshakePacket, error) {
	if err := c.ensureDispatcher(); err != nil {
		return nil, err
	}

	req := c.registerPending(match)

	if err := c.writeControlPacket(requestPacket); err != nil {
		c.completePending(req.ID, ControlResponse{Err: fmt.Errorf("send request: %w", err)})
		return nil, err
	}

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}
		return resp.Packet, nil
	case <-ctx.Done():
		c.completePending(req.ID, ControlResponse{Err: ctx.Err()})
		return nil, ctx.Err()
	}
}

// GetOrCreateStream gets or creates a data stream with the given ID
func (c *BaseClient) GetOrCreateStream(streamID uint32) (io.ReadWriteCloser, error) {
	if streamID == 0 {
		return nil, fmt.Errorf("invalid stream ID 0 - reserved for control stream")
	}

	if c.multiplexer == nil {
		return nil, fmt.Errorf("multiplexer is nil - cannot create stream with ID %d", streamID)
	}

	// Try to get existing stream
	if stream, err := c.multiplexer.GetStream(streamID); err == nil {
		// Verify the stream is not the control stream
		if stream.ID == 0 {
			return nil, fmt.Errorf("CRITICAL BUG: Retrieved stream has ID 0 (control stream)")
		}
		// Check if stream is closed - if so, remove it and create a new one
		if stream.IsClosed() {
			// Stream exists but is closed - remove it and create a new one
			_ = c.multiplexer.CloseStream(streamID)
			// Fall through to create new stream
		} else {
			return stream, nil
		}
	}

	// Create new stream
	stream, err := c.multiplexer.OpenStreamWithID(streamID)
	if err != nil {
		return nil, err
	}

	// Verify the stream is not the control stream
	if stream.ID == 0 {
		return nil, fmt.Errorf("CRITICAL BUG: Created stream has ID 0 (control stream)")
	}

	return stream, nil
}

// ControlConn returns the control connection
func (c *BaseClient) ControlConn() *protocol.PacketReadWriteCloser {
	return c.controlConn
}

// Multiplexer returns the multiplexer
func (c *BaseClient) Multiplexer() *multiplexer.Multiplexer {
	return c.multiplexer
}

// ClientID returns the client ID
func (c *BaseClient) ClientID() string {
	if c == nil {
		return ""
	}
	return c.clientID
}

// IsHandshakeDone returns whether the handshake is complete
func (c *BaseClient) IsHandshakeDone() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.handshakeDone
}

// Close closes the base client (protocols can override for additional cleanup)
func (c *BaseClient) Close() error {
	if c == nil {
		return nil
	}
	if c.dispatchCancel != nil {
		c.dispatchCancel()
	}
	if c.control != nil {
		c.control.Close()
	}
	if c.multiplexer != nil {
		return c.multiplexer.Close()
	}
	return nil
}

// Internal helper methods

// EnsureConnected checks if the client is connected
func (c *BaseClient) EnsureConnected() error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	c.mu.RLock()
	done := c.handshakeDone
	c.mu.RUnlock()
	if !done {
		return fmt.Errorf("not connected - call Connect() first")
	}
	return nil
}

func (c *BaseClient) ensureDispatcher() error {
	var startErr error
	c.dispatchOnce.Do(func() {
		if err := c.control.EnsureStarted(); err != nil {
			startErr = err
			return
		}
		c.dispatchCtx, c.dispatchCancel = context.WithCancel(context.Background())
		go c.runDispatcher()
	})
	return startErr
}

func (c *BaseClient) writeControlPacket(packet *protocol.Packet) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.controlConn.WritePacket(packet)
}

func (c *BaseClient) runDispatcher() {
	for {
		packet, err := c.control.Next(c.dispatchCtx)
		if err != nil {
			c.failAllPending(err)
			return
		}

		if c.dispatchPending(packet) {
			continue
		}

		// If packet didn't match any pending request and we have an events handler, forward it
		if c.eventsHandler != nil {
			c.eventsHandler.HandleEvent(packet)
		}
	}
}

func (c *BaseClient) dispatchPending(packet *protocol.HandshakePacket) bool {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, req := range c.pending {
		matched, err := req.Match(packet)
		if err != nil {
			delete(c.pending, id)
			req.Deliver(ControlResponse{Err: err})
			return true
		}
		if matched {
			delete(c.pending, id)
			req.Deliver(ControlResponse{Packet: packet})
			return true
		}
	}

	return false
}

func (c *BaseClient) registerPending(match Matcher) *PendingRequest {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	c.nextRequestIDVal++
	req := &PendingRequest{
		ID:       c.nextRequestIDVal,
		Match:    match,
		Response: make(chan ControlResponse, 1),
	}
	c.pending[req.ID] = req
	return req
}

func (c *BaseClient) completePending(id uint64, resp ControlResponse) {
	c.pendingMu.Lock()
	req, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	if ok {
		req.Deliver(resp)
	}
}

func (c *BaseClient) failAllPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, req := range c.pending {
		req.Deliver(ControlResponse{Err: err})
		delete(c.pending, id)
	}
}

func (c *BaseClient) readHandshakeWithTimeout(timeout time.Duration) (*protocol.HandshakePacket, error) {
	type result struct {
		handshake *protocol.HandshakePacket
		err       error
	}

	done := make(chan result, 1)

	go func() {
		packet, err := c.controlConn.ReadPacket()
		if err != nil {
			done <- result{nil, fmt.Errorf("read: %w", err)}
			return
		}

		h, err := protocol.UnmarshalHandshakePacket(packet)
		if err != nil {
			done <- result{nil, fmt.Errorf("unmarshal: %w", err)}
			return
		}

		done <- result{h, nil}
	}()

	select {
	case res := <-done:
		return res.handshake, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}
