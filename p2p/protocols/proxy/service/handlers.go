package service

import (
	"context"
	"encoding/json"
	"fmt"

	"stellar/p2p/protocols/common/protocol"
)

// proxyHandshakeHandler implements HandshakeHandler for proxy protocol
type proxyHandshakeHandler struct{}

func (h *proxyHandshakeHandler) ExpectedHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeHello
}

func (h *proxyHandshakeHandler) UnmarshalHelloPayload(payload []byte) (interface{}, error) {
	var helloPayload protocol.HelloPayload
	if err := json.Unmarshal(payload, &helloPayload); err != nil {
		return nil, fmt.Errorf("unmarshal hello payload: %w", err)
	}
	return helloPayload, nil
}

func (h *proxyHandshakeHandler) CreateAckPacket() (*protocol.Packet, error) {
	return protocol.NewHelloAckPacket()
}

func (h *proxyHandshakeHandler) ExtractClientID(helloPayload interface{}) string {
	payload, ok := helloPayload.(protocol.HelloPayload)
	if !ok {
		return ""
	}
	return payload.ClientID
}

// proxyPacketDispatcher implements PacketDispatcher for proxy protocol
type proxyPacketDispatcher struct {
	server *Server
}

func (d *proxyPacketDispatcher) Dispatch(ctx context.Context, packet *protocol.HandshakePacket) error {
	if packet == nil {
		return fmt.Errorf("nil handshake packet")
	}

	switch packet.Type {
	case protocol.HandshakeTypeProxyOpen:
		return d.server.handleOpen(packet)
	case protocol.HandshakeTypeProxyClose:
		return d.server.handleClose(packet)
	case protocol.HandshakeTypeProxyList:
		return d.server.handleList(packet)
	default:
		return fmt.Errorf("unknown type: %s", packet.Type)
	}
}

// proxyHelloHandler implements ClientHelloHandler for proxy protocol
type proxyHelloHandler struct{}

func (h *proxyHelloHandler) CreateHelloPacket(version, clientID string) (*protocol.Packet, error) {
	return protocol.NewHelloPacket(version, clientID)
}

func (h *proxyHelloHandler) ExpectedAckType() protocol.HandshakeType {
	return protocol.HandshakeTypeHelloAck
}

func (h *proxyHelloHandler) ErrorHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeError
}
