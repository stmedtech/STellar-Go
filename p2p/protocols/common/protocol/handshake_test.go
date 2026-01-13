package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for simplified test setup
func mustNewHandshakePacket(t HandshakeType, payload interface{}) *Packet {
	packet, err := NewHandshakePacket(t, payload)
	if err != nil {
		panic(err)
	}
	return packet
}

// ============================================================================
// Basic Functionality Tests
// ============================================================================

func TestHandshakePacketCreation(t *testing.T) {
	hello := HelloPayload{
		Version:  "1.0",
		ClientID: "test-client",
	}

	packet, err := NewHandshakePacket(HandshakeTypeHello, hello)
	require.NoError(t, err)
	assert.Equal(t, PacketTypeTunneling, packet.Type)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeHello, handshake.Type)
}

func TestHandshakePacketUnmarshal(t *testing.T) {
	hello := HelloPayload{
		Version:  "1.0",
		ClientID: "test-client",
	}

	packet, err := NewHandshakePacket(HandshakeTypeHello, hello)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeHello, handshake.Type)

	var payload HelloPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, "1.0", payload.Version)
	assert.Equal(t, "test-client", payload.ClientID)
}

func TestHelloPayload(t *testing.T) {
	hello := HelloPayload{
		Version:  "1.0",
		ClientID: "my-client",
	}

	packet, err := NewHandshakePacket(HandshakeTypeHello, hello)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload HelloPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, hello.Version, payload.Version)
	assert.Equal(t, hello.ClientID, payload.ClientID)
}

func TestProxyOpenRequest(t *testing.T) {
	request := ProxyOpenRequest{
		ProxyID:    "proxy-1",
		RemoteAddr: "target:9090",
		Protocol:   "tcp",
	}

	packet, err := NewHandshakePacket(HandshakeTypeProxyOpen, request)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeProxyOpen, handshake.Type)

	var payload ProxyOpenRequest
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, request.ProxyID, payload.ProxyID)
	assert.Equal(t, request.RemoteAddr, payload.RemoteAddr)
	assert.Equal(t, request.Protocol, payload.Protocol)
}

func TestProxyOpenResponse(t *testing.T) {
	response := ProxyOpenResponse{
		ProxyID:  "proxy-1",
		Success:  true,
		StreamID: 1,
	}

	packet, err := NewHandshakePacket(HandshakeTypeProxyOpened, response)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeProxyOpened, handshake.Type)

	var payload ProxyOpenResponse
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, response.ProxyID, payload.ProxyID)
	assert.Equal(t, response.Success, payload.Success)
	assert.Equal(t, response.StreamID, payload.StreamID)
}

func TestErrorPayload(t *testing.T) {
	errorPayload := ErrorPayload{
		Code:    "connection_failed",
		Message: "Failed to connect to remote",
	}

	packet, err := NewHandshakePacket(HandshakeTypeError, errorPayload)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeError, handshake.Type)

	var payload ErrorPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, errorPayload.Code, payload.Code)
	assert.Equal(t, errorPayload.Message, payload.Message)
}

func TestHandshakePacketWithoutPayload(t *testing.T) {
	packet, err := NewHandshakePacket(HandshakeTypeHelloAck, nil)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeHelloAck, handshake.Type)
	assert.Empty(t, handshake.Payload)
}

func TestProxyCloseRequest(t *testing.T) {
	request := ProxyCloseRequest{
		ProxyID: "proxy-1",
	}

	packet, err := NewHandshakePacket(HandshakeTypeProxyClose, request)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload ProxyCloseRequest
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, request.ProxyID, payload.ProxyID)
}

func TestProxyListResponse(t *testing.T) {
	response := ProxyListResponse{
		Proxies: []ProxyInfo{
			{
				ProxyID:    "proxy-1",
				RemoteAddr: "target:9090",
				Protocol:   "tcp",
				Status:     "active",
			},
			{
				ProxyID:    "proxy-2",
				RemoteAddr: "target:9091",
				Protocol:   "tcp",
				Status:     "active",
			},
		},
	}

	packet, err := NewHandshakePacket(HandshakeTypeProxyListResp, response)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload ProxyListResponse
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Len(t, payload.Proxies, 2)
	assert.Equal(t, "proxy-1", payload.Proxies[0].ProxyID)
	assert.Equal(t, "proxy-2", payload.Proxies[1].ProxyID)
}

// ============================================================================
// Edge Cases: All Handshake Types
// ============================================================================

func TestAllHandshakeTypes(t *testing.T) {
	types := []HandshakeType{
		HandshakeTypeHello,
		HandshakeTypeHelloAck,
		HandshakeTypeProxyOpen,
		HandshakeTypeProxyOpened,
		HandshakeTypeProxyClose,
		HandshakeTypeProxyClosed,
		HandshakeTypeProxyList,
		HandshakeTypeProxyListResp,
		HandshakeTypeError,
	}

	for _, handshakeType := range types {
		t.Run(string(handshakeType), func(t *testing.T) {
			packet, err := NewHandshakePacket(handshakeType, nil)
			require.NoError(t, err, "Should create packet for type %s", handshakeType)

			handshake, err := UnmarshalHandshakePacket(packet)
			require.NoError(t, err, "Should unmarshal type %s", handshakeType)
			assert.Equal(t, handshakeType, handshake.Type)
		})
	}
}

// ============================================================================
// Edge Cases: Invalid Data
// ============================================================================

func TestUnmarshalHandshakePacketWrongPacketType(t *testing.T) {
	// Create a non-tunneling packet
	packet := &Packet{
		Type:    PacketTypeHandshake,
		Content: []byte(`{"type":"hello"}`),
	}

	_, err := UnmarshalHandshakePacket(packet)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a tunneling packet")
}

func TestUnmarshalHandshakePacketInvalidJSON(t *testing.T) {
	// Create tunneling packet with invalid JSON
	packet, err := NewTunnelingPacket([]byte(`{invalid json}`))
	require.NoError(t, err)

	_, err = UnmarshalHandshakePacket(packet)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestUnmarshalHandshakePacketEmptyData(t *testing.T) {
	// Create tunneling packet with empty data
	packet, err := NewTunnelingPacket([]byte{})
	require.NoError(t, err)

	_, err = UnmarshalHandshakePacket(packet)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Payload Handling
// ============================================================================

func TestHandshakeUnmarshalPayloadEmpty(t *testing.T) {
	packet, err := NewHandshakePacket(HandshakeTypeHelloAck, nil)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload HelloPayload
	err = handshake.UnmarshalPayload(&payload)
	// Empty payload should not error, just leave payload as zero value
	assert.NoError(t, err)
}

func TestHandshakeUnmarshalPayloadWrongType(t *testing.T) {
	hello := HelloPayload{
		Version:  "1.0",
		ClientID: "test",
	}

	packet, err := NewHandshakePacket(HandshakeTypeHello, hello)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	// Try to unmarshal into wrong type
	var wrongPayload ProxyOpenRequest
	err = handshake.UnmarshalPayload(&wrongPayload)
	// JSON unmarshaling may succeed but the data will be invalid (empty/wrong fields)
	// The payload should be empty or have wrong values
	if err == nil {
		// If no error, verify the data is invalid
		assert.Empty(t, wrongPayload.ProxyID)
		assert.Empty(t, wrongPayload.RemoteAddr)
	} else {
		// Error is also acceptable
		assert.Error(t, err)
	}
}

func TestHandshakeUnmarshalPayloadInvalidJSON(t *testing.T) {
	// Create handshake with invalid JSON payload
	handshake := &HandshakePacket{
		Type:    HandshakeTypeHello,
		Payload: []byte(`{invalid json}`),
	}

	var payload HelloPayload
	err := handshake.UnmarshalPayload(&payload)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Large Payloads
// ============================================================================

func TestHandshakeLargePayload(t *testing.T) {
	// Create a large proxy list response
	proxies := make([]ProxyInfo, 1000)
	for i := range proxies {
		proxies[i] = ProxyInfo{
			ProxyID:    "proxy-" + string(rune(i)),
			RemoteAddr: "target:9090",
			Protocol:   "tcp",
			Status:     "active",
		}
	}

	response := ProxyListResponse{Proxies: proxies}
	packet, err := NewHandshakePacket(HandshakeTypeProxyListResp, response)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload ProxyListResponse
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Len(t, payload.Proxies, 1000)
}

// ============================================================================
// Edge Cases: Special Characters in Payloads
// ============================================================================

func TestHandshakeSpecialCharacters(t *testing.T) {
	testCases := []struct {
		name    string
		payload interface{}
	}{
		{"unicode client ID", HelloPayload{Version: "1.0", ClientID: "客户端-测试"}},
		{"special chars in address", ProxyOpenRequest{ProxyID: "proxy-1", RemoteAddr: "target:9090?param=value&other=test", Protocol: "tcp"}},
		{"newlines in error", ErrorPayload{Code: "error", Message: "Error\nwith\nnewlines"}},
		{"quotes in message", ErrorPayload{Code: "error", Message: `Error with "quotes"`}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet, err := NewHandshakePacket(HandshakeTypeHello, tc.payload)
			require.NoError(t, err)

			handshake, err := UnmarshalHandshakePacket(packet)
			require.NoError(t, err)
			assert.Equal(t, HandshakeTypeHello, handshake.Type)
		})
	}
}

// ============================================================================
// Edge Cases: Round-trip Consistency
// ============================================================================

func TestHandshakeRoundTripConsistency(t *testing.T) {
	testCases := []struct {
		name    string
		payload interface{}
	}{
		{"hello", HelloPayload{Version: "1.0", ClientID: "test"}},
		{"proxy open", ProxyOpenRequest{ProxyID: "p1", RemoteAddr: "addr:123", Protocol: "tcp"}},
		{"proxy open response success", ProxyOpenResponse{ProxyID: "p1", Success: true, StreamID: 1}},
		{"proxy open response error", ProxyOpenResponse{ProxyID: "p1", Success: false, Error: "failed"}},
		{"error payload", ErrorPayload{Code: "code", Message: "message"}},
		{"proxy list", ProxyListResponse{Proxies: []ProxyInfo{{ProxyID: "p1", RemoteAddr: "addr", Protocol: "tcp", Status: "active"}}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet, err := NewHandshakePacket(HandshakeTypeHello, tc.payload)
			require.NoError(t, err)

			handshake, err := UnmarshalHandshakePacket(packet)
			require.NoError(t, err)

			// Re-marshal and verify consistency
			packet2, err := NewHandshakePacket(handshake.Type, tc.payload)
			require.NoError(t, err)

			handshake2, err := UnmarshalHandshakePacket(packet2)
			require.NoError(t, err)
			assert.Equal(t, handshake.Type, handshake2.Type)
		})
	}
}

// ============================================================================
// Edge Cases: Nil and Zero Values
// ============================================================================

func TestHandshakeNilPayload(t *testing.T) {
	packet, err := NewHandshakePacket(HandshakeTypeHelloAck, nil)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Empty(t, handshake.Payload)
}

func TestHandshakeZeroValuePayload(t *testing.T) {
	zeroHello := HelloPayload{}
	packet, err := NewHandshakePacket(HandshakeTypeHello, zeroHello)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload HelloPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Empty(t, payload.Version)
	assert.Empty(t, payload.ClientID)
}

// ============================================================================
// Edge Cases: Concurrent Access
// ============================================================================

func TestHandshakeConcurrentCreation(t *testing.T) {
	hello := HelloPayload{Version: "1.0", ClientID: "test"}

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			packet, err := NewHandshakePacket(HandshakeTypeHello, hello)
			assert.NoError(t, err)
			assert.NotNil(t, packet)
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
// Edge Cases: MarshalPayload
// ============================================================================

func TestHandshakeMarshalPayload(t *testing.T) {
	handshake := &HandshakePacket{
		Type: HandshakeTypeHello,
	}

	hello := HelloPayload{
		Version:  "1.0",
		ClientID: "test",
	}

	err := handshake.MarshalPayload(hello)
	require.NoError(t, err)
	assert.NotEmpty(t, handshake.Payload)

	var payload HelloPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, hello.Version, payload.Version)
	assert.Equal(t, hello.ClientID, payload.ClientID)
}

func TestHandshakeMarshalPayloadInvalid(t *testing.T) {
	handshake := &HandshakePacket{
		Type: HandshakeTypeHello,
	}

	// Try to marshal something that can't be marshaled
	invalidPayload := make(chan int)
	err := handshake.MarshalPayload(invalidPayload)
	assert.Error(t, err)
}
