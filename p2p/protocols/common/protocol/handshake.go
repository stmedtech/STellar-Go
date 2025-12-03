package protocol

import (
	"encoding/json"
	"fmt"
)

// HandshakeType represents the type of handshake message
type HandshakeType string

const (
	HandshakeTypeHello         HandshakeType = "hello"
	HandshakeTypeHelloAck      HandshakeType = "hello_ack"
	HandshakeTypeProxyOpen     HandshakeType = "proxy_open"
	HandshakeTypeProxyOpened   HandshakeType = "proxy_opened"
	HandshakeTypeProxyClose    HandshakeType = "proxy_close"
	HandshakeTypeProxyClosed   HandshakeType = "proxy_closed"
	HandshakeTypeProxyList     HandshakeType = "proxy_list"
	HandshakeTypeProxyListResp HandshakeType = "proxy_list_response"
	HandshakeTypeError         HandshakeType = "error"
)

// HandshakePacket represents a handshake message encapsulated within a Packet
type HandshakePacket struct {
	Type    HandshakeType   `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HelloPayload is the payload for the initial hello handshake
type HelloPayload struct {
	Version  string `json:"version"`
	ClientID string `json:"client_id"`
}

// ProxyOpenRequest is the payload for requesting a new proxy connection
type ProxyOpenRequest struct {
	ProxyID    string `json:"proxy_id"`
	RemoteAddr string `json:"remote_addr"`
	Protocol   string `json:"protocol"` // "tcp" or "udp"
}

// ProxyCloseRequest is the payload for closing an existing proxy connection
type ProxyCloseRequest struct {
	ProxyID string `json:"proxy_id"`
}

// ProxyClosedResponse is the payload returned after attempting to close a proxy
type ProxyClosedResponse struct {
	ProxyID string `json:"proxy_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ProxyListRequest is the payload for requesting a list of active proxies
type ProxyListRequest struct{}

// ProxyOpenResponse is the response to proxy open request
type ProxyOpenResponse struct {
	ProxyID  string `json:"proxy_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	StreamID uint32 `json:"stream_id,omitempty"` // For nested packet stream
}

// ProxyListResponse contains list of active proxies
type ProxyListResponse struct {
	Proxies []ProxyInfo `json:"proxies"`
}

// ProxyInfo contains information about an active proxy
type ProxyInfo struct {
	ProxyID    string `json:"proxy_id"`
	RemoteAddr string `json:"remote_addr"`
	Protocol   string `json:"protocol"`
	Status     string `json:"status"` // "active", "closing", "closed"
}

// ErrorPayload contains error information
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewHandshakePacket creates a new handshake packet with the given type and payload
func NewHandshakePacket(handshakeType HandshakeType, payload interface{}) (*Packet, error) {
	var payloadJSON json.RawMessage
	if payload != nil {
		var err error
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal handshake payload: %w", err)
		}
	}

	handshake := HandshakePacket{
		Type:    handshakeType,
		Payload: payloadJSON,
	}

	handshakeJSON, err := json.Marshal(handshake)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handshake: %w", err)
	}

	return NewTunnelingPacket(handshakeJSON)
}

// UnmarshalHandshakePacket unmarshals a packet into a handshake packet
func UnmarshalHandshakePacket(packet *Packet) (*HandshakePacket, error) {
	if packet == nil {
		return nil, fmt.Errorf("packet is nil")
	}

	if packet.Type != PacketTypeTunneling {
		return nil, fmt.Errorf("packet is not a tunneling packet, got type: %s", packet.Type)
	}

	// First, unmarshal the tunneling content to get the actual data
	tunnelingContent, err := packet.UnmarshalTunnelingContent()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tunneling content: %w", err)
	}

	// The tunneling content's Data field contains the handshake JSON
	var handshake HandshakePacket
	if err := json.Unmarshal(tunnelingContent.Data, &handshake); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake packet content: %w", err)
	}

	return &handshake, nil
}

// UnmarshalPayload extracts the payload from a handshake packet
func (h *HandshakePacket) UnmarshalPayload(v interface{}) error {
	if h == nil {
		return fmt.Errorf("handshake packet is nil")
	}
	if len(h.Payload) == 0 {
		return nil // Empty payload is valid
	}
	if v == nil {
		return fmt.Errorf("target is nil")
	}
	if err := json.Unmarshal(h.Payload, v); err != nil {
		return fmt.Errorf("failed to unmarshal handshake payload: %w", err)
	}
	return nil
}

// MarshalPayload marshals the payload of a handshake packet
func (h *HandshakePacket) MarshalPayload(v interface{}) error {
	if h == nil {
		return fmt.Errorf("handshake packet is nil")
	}
	var err error
	h.Payload, err = json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake payload: %w", err)
	}
	return nil
}

// Convenience functions for creating common handshake packets

// NewHelloPacket creates a hello packet
func NewHelloPacket(version, clientID string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeHello, HelloPayload{
		Version:  version,
		ClientID: clientID,
	})
}

// NewHelloAckPacket creates a hello_ack packet
func NewHelloAckPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeHelloAck, nil)
}

// NewProxyOpenPacket creates a proxy_open packet
func NewProxyOpenPacket(proxyID, remoteAddr, protocol string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyOpen, ProxyOpenRequest{
		ProxyID:    proxyID,
		RemoteAddr: remoteAddr,
		Protocol:   protocol,
	})
}

// NewProxyOpenedPacket creates a proxy_opened packet
func NewProxyOpenedPacket(proxyID string, success bool, streamID uint32, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyOpened, ProxyOpenResponse{
		ProxyID:  proxyID,
		Success:  success,
		StreamID: streamID,
		Error:    errMsg,
	})
}

// NewProxyClosePacket creates a proxy_close packet
func NewProxyClosePacket(proxyID string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyClose, ProxyCloseRequest{
		ProxyID: proxyID,
	})
}

// NewProxyClosedPacket creates a proxy_closed packet
func NewProxyClosedPacket(proxyID string, success bool, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyClosed, ProxyClosedResponse{
		ProxyID: proxyID,
		Success: success,
		Error:   errMsg,
	})
}

// NewProxyListPacket creates a proxy_list packet
func NewProxyListPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyList, ProxyListRequest{})
}

// NewProxyListResponsePacket creates a proxy_list_response packet
func NewProxyListResponsePacket(proxies []ProxyInfo) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyListResp, ProxyListResponse{
		Proxies: proxies,
	})
}

// NewErrorPacket creates an error packet
func NewErrorPacket(code, message string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeError, ErrorPayload{
		Code:    code,
		Message: message,
	})
}
