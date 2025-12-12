package service

import (
	"testing"

	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
)

// TestPendingRequestDeliver tests PendingRequest.Deliver
func TestPendingRequestDeliver(t *testing.T) {
	req := &PendingRequest{
		ID:       1,
		Match:    func(*protocol.HandshakePacket) (bool, error) { return true, nil },
		Response: make(chan ControlResponse, 1),
	}

	// Deliver response
	resp := ControlResponse{
		Packet: &protocol.HandshakePacket{
			Type: protocol.HandshakeTypeHello,
		},
		Err: nil,
	}

	req.Deliver(resp)

	// Should receive response
	received := <-req.Response
	assert.NotNil(t, received.Packet)
	assert.Equal(t, protocol.HandshakeTypeHello, received.Packet.Type)
	assert.NoError(t, received.Err)
}

// TestPendingRequestDeliverError tests delivering error response
func TestPendingRequestDeliverError(t *testing.T) {
	req := &PendingRequest{
		ID:       1,
		Match:    func(*protocol.HandshakePacket) (bool, error) { return true, nil },
		Response: make(chan ControlResponse, 1),
	}

	// Deliver error
	resp := ControlResponse{
		Packet: nil,
		Err:    assert.AnError,
	}

	req.Deliver(resp)

	// Should receive error
	received := <-req.Response
	assert.Nil(t, received.Packet)
	assert.Error(t, received.Err)
}

// TestPendingRequestDeliverNonBlocking tests non-blocking deliver
func TestPendingRequestDeliverNonBlocking(t *testing.T) {
	req := &PendingRequest{
		ID:       1,
		Match:    func(*protocol.HandshakePacket) (bool, error) { return true, nil },
		Response: make(chan ControlResponse, 1),
	}

	// Fill channel
	resp := ControlResponse{
		Packet: &protocol.HandshakePacket{Type: protocol.HandshakeTypeHello},
	}
	req.Response <- resp

	// Deliver should not block even if channel is full
	req.Deliver(resp) // Should not block or panic
}

// TestControlResponse tests ControlResponse structure
func TestControlResponse(t *testing.T) {
	packet := &protocol.HandshakePacket{
		Type: protocol.HandshakeTypeHelloAck,
	}

	resp := ControlResponse{
		Packet: packet,
		Err:    nil,
	}

	assert.NotNil(t, resp.Packet)
	assert.Equal(t, protocol.HandshakeTypeHelloAck, resp.Packet.Type)
	assert.NoError(t, resp.Err)
}

// TestMatcher tests Matcher function type
func TestMatcher(t *testing.T) {
	// Test matching matcher
	match := func(h *protocol.HandshakePacket) (bool, error) {
		return h.Type == protocol.HandshakeTypeHello, nil
	}

	packet := &protocol.HandshakePacket{
		Type: protocol.HandshakeTypeHello,
	}

	matched, err := match(packet)
	assert.NoError(t, err)
	assert.True(t, matched)

	// Test non-matching
	packet2 := &protocol.HandshakePacket{
		Type: protocol.HandshakeTypeHelloAck,
	}

	matched, err = match(packet2)
	assert.NoError(t, err)
	assert.False(t, matched)
}

// TestMatcherError tests matcher returning error
func TestMatcherError(t *testing.T) {
	match := func(h *protocol.HandshakePacket) (bool, error) {
		return false, assert.AnError
	}

	packet := &protocol.HandshakePacket{
		Type: protocol.HandshakeTypeHello,
	}

	matched, err := match(packet)
	assert.Error(t, err)
	assert.False(t, matched)
}
