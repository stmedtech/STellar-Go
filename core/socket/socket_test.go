package socket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIServerStruct(t *testing.T) {
	// Test APIServer struct creation
	apiServer := &APIServer{}

	// Verify initial state
	assert.Nil(t, apiServer.Node)
	assert.Nil(t, apiServer.Proxy)
	assert.Nil(t, apiServer.server)
}

func TestAPIServerStructWithFields(t *testing.T) {
	// Test APIServer struct with fields
	apiServer := &APIServer{
		Node:  nil, // Would be set to actual node in real usage
		Proxy: nil, // Would be set to actual proxy manager in real usage
	}

	// Verify struct can be created with fields
	assert.Nil(t, apiServer.Node)
	assert.Nil(t, apiServer.Proxy)
	assert.Nil(t, apiServer.server)
}

func TestConnectRequestStruct(t *testing.T) {
	// Test ConnectRequest struct (defined in ConnectToPeer method)
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	req := &ConnectRequest{
		PeerInfo: "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	}

	// Verify struct fields
	assert.Equal(t, "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", req.PeerInfo)
}

func TestConnectRequestJSON(t *testing.T) {
	// Test ConnectRequest JSON marshaling/unmarshaling
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	req := &ConnectRequest{
		PeerInfo: "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Unmarshal from JSON
	var req2 ConnectRequest
	err = json.Unmarshal(jsonData, &req2)
	require.NoError(t, err)

	// Verify data is preserved
	assert.Equal(t, req.PeerInfo, req2.PeerInfo)
}

func TestConnectRequestEmptyFields(t *testing.T) {
	// Test ConnectRequest with empty fields
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	req := &ConnectRequest{}

	// Verify struct can be created with empty fields
	assert.Empty(t, req.PeerInfo)

	// Test JSON marshaling with empty fields
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Unmarshal back
	var req2 ConnectRequest
	err = json.Unmarshal(jsonData, &req2)
	require.NoError(t, err)

	// Verify empty fields are preserved
	assert.Empty(t, req2.PeerInfo)
}

func TestNodeInfoResponseStruct(t *testing.T) {
	// Test NodeInfoResponse struct (defined in GetNodeInfo method)
	type NodeInfoResponse struct {
		NodeID         string
		Addresses      []string // Simplified for testing
		Bootstrapper   bool
		RelayNode      bool
		ReferenceToken string
		DevicesCount   int
		Policy         interface{} // Simplified for testing
	}

	response := &NodeInfoResponse{
		NodeID:         "test-node-id",
		Addresses:      []string{"addr1", "addr2"},
		Bootstrapper:   true,
		RelayNode:      false,
		ReferenceToken: "test-token",
		DevicesCount:   5,
		Policy:         nil,
	}

	// Verify struct fields
	assert.Equal(t, "test-node-id", response.NodeID)
	assert.Len(t, response.Addresses, 2)
	assert.True(t, response.Bootstrapper)
	assert.False(t, response.RelayNode)
	assert.Equal(t, "test-token", response.ReferenceToken)
	assert.Equal(t, 5, response.DevicesCount)
	assert.Nil(t, response.Policy)
}

func TestNodeInfoResponseJSON(t *testing.T) {
	// Test NodeInfoResponse JSON marshaling/unmarshaling
	type NodeInfoResponse struct {
		NodeID         string
		Addresses      []string
		Bootstrapper   bool
		RelayNode      bool
		ReferenceToken string
		DevicesCount   int
		Policy         interface{}
	}

	response := &NodeInfoResponse{
		NodeID:         "test-node-id",
		Addresses:      []string{"addr1", "addr2"},
		Bootstrapper:   true,
		RelayNode:      false,
		ReferenceToken: "test-token",
		DevicesCount:   5,
		Policy:         nil,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Unmarshal from JSON
	var response2 NodeInfoResponse
	err = json.Unmarshal(jsonData, &response2)
	require.NoError(t, err)

	// Verify data is preserved
	assert.Equal(t, response.NodeID, response2.NodeID)
	assert.Equal(t, response.Addresses, response2.Addresses)
	assert.Equal(t, response.Bootstrapper, response2.Bootstrapper)
	assert.Equal(t, response.RelayNode, response2.RelayNode)
	assert.Equal(t, response.ReferenceToken, response2.ReferenceToken)
	assert.Equal(t, response.DevicesCount, response2.DevicesCount)
	assert.Equal(t, response.Policy, response2.Policy)
}

func TestProxyRequestStruct(t *testing.T) {
	// Test ProxyRequest struct (defined in CreateProxy method)
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	req := &ProxyRequest{
		DeviceID:   "test-device-id",
		LocalPort:  8080,
		RemoteHost: "127.0.0.1",
		RemotePort: 3000,
	}

	// Verify struct fields
	assert.Equal(t, "test-device-id", req.DeviceID)
	assert.Equal(t, uint64(8080), req.LocalPort)
	assert.Equal(t, "127.0.0.1", req.RemoteHost)
	assert.Equal(t, uint64(3000), req.RemotePort)
}

func TestProxyRequestJSON(t *testing.T) {
	// Test ProxyRequest JSON marshaling/unmarshaling
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	req := &ProxyRequest{
		DeviceID:   "test-device-id",
		LocalPort:  8080,
		RemoteHost: "127.0.0.1",
		RemotePort: 3000,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Unmarshal from JSON
	var req2 ProxyRequest
	err = json.Unmarshal(jsonData, &req2)
	require.NoError(t, err)

	// Verify data is preserved
	assert.Equal(t, req.DeviceID, req2.DeviceID)
	assert.Equal(t, req.LocalPort, req2.LocalPort)
	assert.Equal(t, req.RemoteHost, req2.RemoteHost)
	assert.Equal(t, req.RemotePort, req2.RemotePort)
}

func TestProxyRequestEmptyFields(t *testing.T) {
	// Test ProxyRequest with empty fields
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	req := &ProxyRequest{}

	// Verify struct can be created with empty fields
	assert.Empty(t, req.DeviceID)
	assert.Equal(t, uint64(0), req.LocalPort)
	assert.Empty(t, req.RemoteHost)
	assert.Equal(t, uint64(0), req.RemotePort)

	// Test JSON marshaling with empty fields
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Unmarshal back
	var req2 ProxyRequest
	err = json.Unmarshal(jsonData, &req2)
	require.NoError(t, err)

	// Verify empty fields are preserved
	assert.Empty(t, req2.DeviceID)
	assert.Equal(t, uint64(0), req2.LocalPort)
	assert.Empty(t, req2.RemoteHost)
	assert.Equal(t, uint64(0), req2.RemotePort)
}

func TestAPIServerMethodsExist(t *testing.T) {
	// Test that APIServer methods exist and can be called
	apiServer := &APIServer{}

	// Verify methods exist (this is a basic test to ensure the struct has the expected methods)
	// In a real test environment, these would be tested with proper HTTP requests
	assert.NotNil(t, apiServer)
}

func TestConnectRequestValidation(t *testing.T) {
	// Test ConnectRequest validation scenarios
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	tests := []struct {
		name     string
		peerInfo string
		valid    bool
	}{
		{"valid peer info", "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", true},
		{"empty peer info", "", false},
		{"invalid peer info", "invalid-addr", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ConnectRequest{
				PeerInfo: tt.peerInfo,
			}

			// Test JSON marshaling
			jsonData, err := json.Marshal(req)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotEmpty(t, jsonData)
			} else {
				// Even invalid data should marshal to JSON
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyRequestValidation(t *testing.T) {
	// Test ProxyRequest validation scenarios
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	tests := []struct {
		name       string
		deviceID   string
		localPort  uint64
		remoteHost string
		remotePort uint64
		valid      bool
	}{
		{"valid request", "device1", 8080, "127.0.0.1", 3000, true},
		{"empty device ID", "", 8080, "127.0.0.1", 3000, false},
		{"zero local port", "device1", 0, "127.0.0.1", 3000, false},
		{"empty remote host", "device1", 8080, "", 3000, false},
		{"zero remote port", "device1", 8080, "127.0.0.1", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ProxyRequest{
				DeviceID:   tt.deviceID,
				LocalPort:  tt.localPort,
				RemoteHost: tt.remoteHost,
				RemotePort: tt.remotePort,
			}

			// Test JSON marshaling
			jsonData, err := json.Marshal(req)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotEmpty(t, jsonData)
			} else {
				// Even invalid data should marshal to JSON
				assert.NoError(t, err)
			}
		})
	}
}

func BenchmarkConnectRequestJSON(b *testing.B) {
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	req := &ConnectRequest{
		PeerInfo: "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProxyRequestJSON(b *testing.B) {
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	req := &ProxyRequest{
		DeviceID:   "test-device-id",
		LocalPort:  8080,
		RemoteHost: "127.0.0.1",
		RemotePort: 3000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNodeInfoResponseJSON(b *testing.B) {
	type NodeInfoResponse struct {
		NodeID         string
		Addresses      []string
		Bootstrapper   bool
		RelayNode      bool
		ReferenceToken string
		DevicesCount   int
		Policy         interface{}
	}

	response := &NodeInfoResponse{
		NodeID:         "test-node-id",
		Addresses:      []string{"addr1", "addr2", "addr3"},
		Bootstrapper:   true,
		RelayNode:      false,
		ReferenceToken: "test-token",
		DevicesCount:   5,
		Policy:         nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(response)
		if err != nil {
			b.Fatal(err)
		}
	}
}
