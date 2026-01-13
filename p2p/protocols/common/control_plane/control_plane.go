package control_plane

import (
	"context"
	"fmt"
	"io"
	"sync"

	"stellar/p2p/protocols/common/protocol"
)

const ControlBufferSize = 32

type ControlEvent struct {
	Packet *protocol.HandshakePacket
	Err    error
}

// ControlPlane handles control-plane packet reading and event dispatching
type ControlPlane struct {
	conn     *protocol.PacketReadWriteCloser
	once     sync.Once
	cancel   context.CancelFunc
	events   chan ControlEvent
	startErr error
}

// NewControlPlane creates a new control plane
func NewControlPlane(conn *protocol.PacketReadWriteCloser) *ControlPlane {
	return &ControlPlane{
		conn: conn,
	}
}

// EnsureStarted starts the control plane if not already started
func (c *ControlPlane) EnsureStarted() error {
	if c == nil {
		return fmt.Errorf("control plane is nil")
	}

	c.once.Do(func() {
		if c.conn == nil {
			c.startErr = fmt.Errorf("control connection not initialized")
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.events = make(chan ControlEvent, ControlBufferSize)

		go c.run(ctx)
	})

	return c.startErr
}

// Next returns the next control packet or error
func (c *ControlPlane) Next(ctx context.Context) (*protocol.HandshakePacket, error) {
	if c == nil {
		return nil, fmt.Errorf("control plane is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := c.EnsureStarted(); err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case evt, ok := <-c.events:
			if !ok {
				return nil, io.EOF
			}
			if evt.Err != nil {
				return nil, evt.Err
			}
			if evt.Packet == nil {
				continue
			}
			return evt.Packet, nil
		}
	}
}

// Close closes the control plane
func (c *ControlPlane) Close() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *ControlPlane) run(ctx context.Context) {
	defer close(c.events)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		packet, err := c.conn.ReadPacket()
		if err != nil {
			c.events <- ControlEvent{Err: fmt.Errorf("read message: %w", err)}
			return
		}

		handshake, err := protocol.UnmarshalHandshakePacket(packet)
		if err != nil {
			c.events <- ControlEvent{Err: fmt.Errorf("unmarshal: %w", err)}
			return
		}

		select {
		case c.events <- ControlEvent{Packet: handshake}:
		case <-ctx.Done():
			return
		}
	}
}
