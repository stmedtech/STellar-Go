package service

import (
	"context"
	"encoding/json"
	"fmt"

	"stellar/p2p/protocols/common/protocol"
)

// fileHandshakeHandler implements server-side handshake handling for file protocol
type fileHandshakeHandler struct{}

func (h *fileHandshakeHandler) ExpectedHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeFileHello
}

func (h *fileHandshakeHandler) UnmarshalHelloPayload(payload []byte) (interface{}, error) {
	var hello protocol.FileHelloPayload
	if err := json.Unmarshal(payload, &hello); err != nil {
		return nil, fmt.Errorf("unmarshal hello payload: %w", err)
	}
	return hello, nil
}

func (h *fileHandshakeHandler) CreateAckPacket() (*protocol.Packet, error) {
	return protocol.NewFileHelloAckPacket()
}

func (h *fileHandshakeHandler) ExtractClientID(helloPayload interface{}) string {
	payload, ok := helloPayload.(protocol.FileHelloPayload)
	if !ok {
		return ""
	}
	return payload.ClientID
}

// filePacketDispatcher routes control-plane packets to the file server handlers
type filePacketDispatcher struct {
	server *Server
}

func (d *filePacketDispatcher) Dispatch(_ context.Context, packet *protocol.HandshakePacket) error {
	if d == nil || d.server == nil {
		return fmt.Errorf("dispatcher not initialized")
	}
	switch packet.Type {
	case protocol.HandshakeTypeFileList:
		return d.server.handleList(packet)
	case protocol.HandshakeTypeFileGet:
		return d.server.handleGet(packet)
	case protocol.HandshakeTypeFileSend:
		return d.server.handleSend(packet)
	default:
		return fmt.Errorf("unknown type: %s", packet.Type)
	}
}

// fileHelloHandler implements client-side hello for file protocol
type fileHelloHandler struct{}

func (h *fileHelloHandler) CreateHelloPacket(version, clientID string) (*protocol.Packet, error) {
	features := []string{"list", "get", "send", "checksum", "recursive"}
	return protocol.NewFileHelloPacket(version, clientID, features)
}

func (h *fileHelloHandler) ExpectedAckType() protocol.HandshakeType {
	return protocol.HandshakeTypeFileHelloAck
}

func (h *fileHelloHandler) ErrorHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeFileError
}
