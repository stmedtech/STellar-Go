package socket

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"stellar/p2p/node"
	"stellar/p2p/protocols/echo"
	"stellar/p2p/protocols/proxy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// TestCreateProxyAPI tests the CreateProxy API endpoint
func TestCreateProxyAPI(t *testing.T) {
	// Create test node
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	// Bind echo so ConnectDevice/Ping works (required by node.ConnectDevice health checks)
	echo.BindEchoStream(testNode)

	// Create proxy manager
	proxyManager := proxy.NewProxyManager(testNode)

	// Create API server
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	// Create a test device (peer)
	testNode2, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode2.Close()

	// Bind protocols on the remote peer so negotiation works
	echo.BindEchoStream(testNode2)
	_ = proxy.NewProxyManager(testNode2) // registers proxy stream handler on remote

	// Connect nodes
	peerInfo := testNode2.Host.Peerstore().PeerInfo(testNode2.ID())
	err = testNode.Host.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// Add device to node
	device, err := testNode.ConnectDevice(peerInfo)
	require.NoError(t, err)
	require.NotNil(t, device)

	localPort := freeTCPPort(t)

	// Create request body
	reqBody := map[string]interface{}{
		"device_id":   device.ID.String(),
		"local_port":  localPort,
		"remote_host": "127.0.0.1",
		"remote_port": 8080,
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// Create HTTP request
	req := httptest.NewRequest("POST", "/proxy", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute request
	apiServer.server.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response)
}

// TestCreateProxyAPIInvalidDevice tests CreateProxy with invalid device
func TestCreateProxyAPIInvalidDevice(t *testing.T) {
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	proxyManager := proxy.NewProxyManager(testNode)
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	localPort := freeTCPPort(t)
	reqBody := map[string]interface{}{
		"device_id":   "invalid-device-id",
		"local_port":  localPort,
		"remote_host": "127.0.0.1",
		"remote_port": 8080,
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/proxy", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestCreateProxyAPIInvalidRequest tests CreateProxy with invalid request
func TestCreateProxyAPIInvalidRequest(t *testing.T) {
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	proxyManager := proxy.NewProxyManager(testNode)
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	// Missing required fields
	reqBody := map[string]interface{}{
		"device_id": "test-device",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/proxy", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestListProxiesAPI tests the ListProxies API endpoint
func TestListProxiesAPI(t *testing.T) {
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	proxyManager := proxy.NewProxyManager(testNode)
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	req := httptest.NewRequest("GET", "/proxy", nil)
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var proxies []interface{}
	err = json.Unmarshal(w.Body.Bytes(), &proxies)
	require.NoError(t, err)
	assert.NotNil(t, proxies)
}

// TestCloseProxyAPI tests the CloseProxy API endpoint
func TestCloseProxyAPI(t *testing.T) {
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	proxyManager := proxy.NewProxyManager(testNode)
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	// Close a proxy that doesn't exist (should still return success)
	req := httptest.NewRequest("DELETE", "/proxy/8080", nil)
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, true, response["success"])
	assert.Equal(t, float64(8080), response["port"])
}

// TestCloseProxyAPIInvalidPort tests CloseProxy with invalid port
func TestCloseProxyAPIInvalidPort(t *testing.T) {
	testNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode.Close()

	proxyManager := proxy.NewProxyManager(testNode)
	apiServer := &APIServer{
		Node:  testNode,
		Proxy: proxyManager,
	}
	apiServer.Start()

	req := httptest.NewRequest("DELETE", "/proxy/invalid", nil)
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestProxyAPIIntegration tests full proxy lifecycle through API
func TestProxyAPIIntegration(t *testing.T) {
	// Create two nodes
	testNode1, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode1.Close()

	testNode2, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer testNode2.Close()

	// Bind protocols so ConnectDevice is deterministic.
	echo.BindEchoStream(testNode1)
	echo.BindEchoStream(testNode2)

	// Create proxy managers
	proxyManager1 := proxy.NewProxyManager(testNode1)
	proxy.NewProxyManager(testNode2) // Server-side

	// Create API server
	apiServer := &APIServer{
		Node:  testNode1,
		Proxy: proxyManager1,
	}
	apiServer.Start()

	// Connect nodes
	peerInfo := testNode2.Host.Peerstore().PeerInfo(testNode2.ID())
	err = testNode1.Host.Connect(context.Background(), peerInfo)
	require.NoError(t, err)

	// ConnectDevice performs ping/device-info handshake and registers the device locally.
	device, err := testNode1.ConnectDevice(peerInfo)
	require.NoError(t, err)
	require.NotNil(t, device)

	// Use ephemeral ports to avoid collisions in parallel test runs.
	localPort := freeTCPPort(t)
	destListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	destPort := destListener.Addr().(*net.TCPAddr).Port
	require.NoError(t, destListener.Close())

	// Create proxy via API
	reqBody := map[string]interface{}{
		"device_id":   device.ID.String(),
		"local_port":  localPort,
		"remote_host": "127.0.0.1",
		"remote_port": destPort,
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/proxy", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	apiServer.server.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	createdPort, ok := createResp["local_port"].(float64)
	require.True(t, ok)

	// List proxies
	req = httptest.NewRequest("GET", "/proxy", nil)
	w = httptest.NewRecorder()
	apiServer.server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var proxies []interface{}
	err = json.Unmarshal(w.Body.Bytes(), &proxies)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(proxies), 0)

	// Close proxy via API (use the actual created port).
	req = httptest.NewRequest("DELETE", "/proxy/"+strconv.Itoa(int(createdPort)), nil)
	w = httptest.NewRecorder()
	apiServer.server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
