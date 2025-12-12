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

// HandshakeHandler defines the interface for protocol-specific handshake handling
type HandshakeHandler interface {
	// ExpectedHandshakeType returns the handshake type this handler expects
	ExpectedHandshakeType() protocol.HandshakeType

	// UnmarshalHelloPayload unmarshals the hello payload into the handler's type
	UnmarshalHelloPayload(payload []byte) (interface{}, error)

	// CreateAckPacket creates the acknowledgment packet
	CreateAckPacket() (*protocol.Packet, error)

	// ExtractClientID extracts client ID from the unmarshaled hello payload
	ExtractClientID(helloPayload interface{}) string
}

// PacketDispatcher defines the interface for protocol-specific packet dispatching
type PacketDispatcher interface {
	// Dispatch handles a control packet
	Dispatch(ctx context.Context, packet *protocol.HandshakePacket) error
}

// BaseServer provides common server functionality for all protocols
type BaseServer struct {
	// Common fields
	controlConn      *protocol.PacketReadWriteCloser
	multiplexer      *multiplexer.Multiplexer
	control          *control_plane.ControlPlane
	clientID         string
	handshakeDone    bool
	streamIDCounter  uint32
	HandshakeTimeout time.Duration
	mu               sync.RWMutex
	acceptMu         sync.Mutex // Protects Accept() from concurrent calls

	// Protocol-specific handlers
	handshakeHandler HandshakeHandler
	packetDispatcher PacketDispatcher
}

// NewBaseServer creates a new base server with automatic multiplexer setup
func NewBaseServer(conn io.ReadWriteCloser, handshakeHandler HandshakeHandler, packetDispatcher PacketDispatcher) *BaseServer {
	if conn == nil {
		return nil
	}

	mux := multiplexer.NewMultiplexer(conn)
	controlStream, err := mux.ControlStream()
	if err != nil {
		return nil
	}

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return &BaseServer{
		controlConn:      packetConn,
		multiplexer:      mux,
		HandshakeTimeout: 30 * time.Second,
		control:          control_plane.NewControlPlane(packetConn),
		handshakeHandler: handshakeHandler,
		packetDispatcher: packetDispatcher,
	}
}

// Accept performs handshake and accepts a client connection
// This method is protected by acceptMu to prevent concurrent Accept() calls
func (s *BaseServer) Accept() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	if s.handshakeHandler == nil {
		return fmt.Errorf("handshake handler not set")
	}

	// Use exclusive lock to prevent concurrent Accept() calls
	s.acceptMu.Lock()
	defer s.acceptMu.Unlock()

	// Check if already accepted
	s.mu.RLock()
	alreadyAccepted := s.handshakeDone
	s.mu.RUnlock()

	if alreadyAccepted {
		return fmt.Errorf("already accepted")
	}

	// Read hello with timeout
	handshake, err := s.readHandshakeWithTimeout(s.HandshakeTimeout)
	if err != nil {
		return err
	}

	// Verify handshake type
	expectedType := s.handshakeHandler.ExpectedHandshakeType()
	if handshake.Type != expectedType {
		return fmt.Errorf("unexpected type: %s, expected %s", handshake.Type, expectedType)
	}

	// Unmarshal payload using handler
	helloPayload, err := s.handshakeHandler.UnmarshalHelloPayload(handshake.Payload)
	if err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	// Create and send ack using handler
	ackPacket, err := s.handshakeHandler.CreateAckPacket()
	if err != nil {
		return fmt.Errorf("create ack: %w", err)
	}

	if err := s.controlConn.WritePacket(ackPacket); err != nil {
		return fmt.Errorf("send ack: %w", err)
	}

	// Extract and store client ID
	clientID := s.handshakeHandler.ExtractClientID(helloPayload)

	// Mark as accepted
	s.mu.Lock()
	s.clientID = clientID
	s.handshakeDone = true
	s.mu.Unlock()
	return nil
}

// Serve runs the control-plane event loop until the context is canceled or an error occurs.
func (s *BaseServer) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureAccepted(); err != nil {
		return err
	}

	if s.packetDispatcher == nil {
		return fmt.Errorf("packet dispatcher not set")
	}

	for {
		packet, err := s.control.Next(ctx)
		if err != nil {
			return err
		}
		if err := s.packetDispatcher.Dispatch(ctx, packet); err != nil {
			return err
		}
	}
}

// NextStreamID returns the next available stream ID
func (s *BaseServer) NextStreamID() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamIDCounter == 0 {
		s.streamIDCounter = 1 // Stream 0 is control
	}
	id := s.streamIDCounter
	s.streamIDCounter++
	return id
}

// ControlConn returns the control connection
func (s *BaseServer) ControlConn() *protocol.PacketReadWriteCloser {
	return s.controlConn
}

// Multiplexer returns the multiplexer
func (s *BaseServer) Multiplexer() *multiplexer.Multiplexer {
	return s.multiplexer
}

// ClientID returns the client ID
func (s *BaseServer) ClientID() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientID
}

// IsHandshakeDone returns whether the handshake is complete
func (s *BaseServer) IsHandshakeDone() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handshakeDone
}

// Close closes the base server (protocols can override for additional cleanup)
func (s *BaseServer) Close() error {
	if s == nil {
		return nil
	}
	if s.control != nil {
		s.control.Close()
	}
	if s.multiplexer != nil {
		return s.multiplexer.Close()
	}
	return nil
}

// Internal helper methods

func (s *BaseServer) readHandshakeWithTimeout(timeout time.Duration) (*protocol.HandshakePacket, error) {
	type result struct {
		handshake *protocol.HandshakePacket
		err       error
	}

	done := make(chan result, 1)

	go func() {
		packet, err := s.controlConn.ReadPacket()
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

func (s *BaseServer) ensureAccepted() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}
	s.mu.RLock()
	done := s.handshakeDone
	s.mu.RUnlock()
	if !done {
		return fmt.Errorf("not accepted - call Accept() first")
	}
	return nil
}
