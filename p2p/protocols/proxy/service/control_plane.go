package service

import (
	"context"
	"fmt"
	"io"
	"sync"

	"stellar/p2p/protocols/common/protocol"
)

const controlBufferSize = 32

type controlEvent struct {
	packet *protocol.HandshakePacket
	err    error
}

type controlPlane struct {
	conn     *protocol.PacketReadWriteCloser
	once     sync.Once
	cancel   context.CancelFunc
	events   chan controlEvent
	startErr error
}

func newControlPlane(conn *protocol.PacketReadWriteCloser) *controlPlane {
	return &controlPlane{
		conn: conn,
	}
}

func (c *controlPlane) ensureStarted() error {
	c.once.Do(func() {
		if c.conn == nil {
			c.startErr = fmt.Errorf("control connection not initialized")
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.events = make(chan controlEvent, controlBufferSize)

		go c.run(ctx)
	})

	return c.startErr
}

func (c *controlPlane) Next(ctx context.Context) (*protocol.HandshakePacket, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := c.ensureStarted(); err != nil {
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
			if evt.err != nil {
				return nil, evt.err
			}
			if evt.packet == nil {
				continue
			}
			return evt.packet, nil
		}
	}
}

func (c *controlPlane) Close() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *controlPlane) run(ctx context.Context) {
	defer close(c.events)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		packet, err := c.conn.ReadPacket()
		if err != nil {
			c.events <- controlEvent{err: fmt.Errorf("read message: %w", err)}
			return
		}

		handshake, err := protocol.UnmarshalHandshakePacket(packet)
		if err != nil {
			c.events <- controlEvent{err: fmt.Errorf("unmarshal: %w", err)}
			return
		}

		select {
		case c.events <- controlEvent{packet: handshake}:
		case <-ctx.Done():
			return
		}
	}
}
