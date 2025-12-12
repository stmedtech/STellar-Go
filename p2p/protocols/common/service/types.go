package service

import (
	"stellar/p2p/protocols/common/protocol"
)

// Matcher is a function that matches a handshake packet
type Matcher func(*protocol.HandshakePacket) (bool, error)

// ControlResponse represents a response to a pending request
type ControlResponse struct {
	Packet *protocol.HandshakePacket
	Err    error
}

// PendingRequest represents a pending request waiting for a response
type PendingRequest struct {
	ID       uint64
	Match    Matcher
	Response chan ControlResponse
}

// Deliver delivers a response to the pending request
func (p *PendingRequest) Deliver(resp ControlResponse) {
	select {
	case p.Response <- resp:
	default:
	}
}
