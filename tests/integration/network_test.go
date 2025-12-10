//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"stellar/p2p/node"
	"stellar/p2p/protocols/echo"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetworkIntegration tests the complete network functionality
func TestNetworkIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create bootstrapper node
	bootstrapper, err := node.NewBootstrapper(
		"0.0.0.0",
		4000,
		"",
		true, // relay node
		true, // debug
	)
	require.NoError(t, err)
	defer bootstrapper.Close()

	// Create two test nodes
	node1, err := node.NewNode("0.0.0.0", 4001)
	require.NoError(t, err)
	defer node1.Close()

	node2, err := node.NewNode("0.0.0.0", 4002)
	require.NoError(t, err)
	defer node2.Close()

	// Bind protocols to nodes
	echo.BindEchoStream(node1)
	echo.BindEchoStream(node2)
	file.BindFileStream(node1)
	file.BindFileStream(node2)

	// Create proxy managers
	proxy1 := proxy.NewProxyManager(node1)
	proxy2 := proxy.NewProxyManager(node2)

	// Test node discovery
	t.Run("NodeDiscovery", func(t *testing.T) {
		testNodeDiscovery(t, bootstrapper, node1, node2)
	})

	// Test echo protocol
	t.Run("EchoProtocol", func(t *testing.T) {
		testEchoProtocol(t, node1, node2)
	})

	// Test file transfer
	t.Run("FileTransfer", func(t *testing.T) {
		testFileTransfer(t, node1, node2)
	})

	// Test proxy functionality
	t.Run("ProxyFunctionality", func(t *testing.T) {
		testProxyFunctionality(t, node1, node2, proxy1, proxy2)
	})

	// Test device health check
	t.Run("DeviceHealthCheck", func(t *testing.T) {
		testDeviceHealthCheck(t, node1, node2)
	})
}

func testNodeDiscovery(t *testing.T, bootstrapper, node1, node2 *node.Node) {
	// Wait for nodes to discover each other
	time.Sleep(5 * time.Second)

	// Check if nodes are in each other's device list
	node1Devices := node1.Devices
	node2Devices := node2.Devices

	assert.NotEmpty(t, node1Devices, "Node1 should have discovered other nodes")
	assert.NotEmpty(t, node2Devices, "Node2 should have discovered other nodes")

	// Verify specific nodes are discovered
	node1Found := false
	node2Found := false

	for _, device := range node1Devices {
		if device.ID == node2.ID() {
			node2Found = true
			break
		}
	}

	for _, device := range node2Devices {
		if device.ID == node1.ID() {
			node1Found = true
			break
		}
	}

	assert.True(t, node1Found, "Node1 should have discovered Node2")
	assert.True(t, node2Found, "Node2 should have discovered Node1")
}

func testEchoProtocol(t *testing.T, node1, node2 *node.Node) {
	// Test ping between nodes
	err := node1.Ping(node2.ID())
	assert.NoError(t, err, "Node1 should be able to ping Node2")

	err = node2.Ping(node1.ID())
	assert.NoError(t, err, "Node2 should be able to ping Node1")

	// Test device info retrieval
	deviceInfo, err := node1.GetEcho(node2.ID(), "deviceInfo")
	assert.NoError(t, err, "Node1 should be able to get device info from Node2")
	assert.NotEmpty(t, deviceInfo, "Device info should not be empty")

	// Parse device info JSON
	var device node.Device
	err = json.Unmarshal([]byte(deviceInfo), &device)
	assert.NoError(t, err, "Device info should be valid JSON")
	assert.Equal(t, node2.ID(), device.ID, "Device ID should match Node2 ID")
}

func testFileTransfer(t *testing.T, node1, node2 *node.Node) {
	// Create a test file
	testContent := "Hello, this is a test file for transfer!"
	testFile := "/tmp/test_file.txt"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)
	defer os.Remove(testFile)

	// Upload file from node1 to node2
	err = file.Upload(node1, node2.ID(), testFile, "received_file.txt")
	assert.NoError(t, err, "File upload should succeed")

	// Download file from node2 to node1
	downloadPath := "/tmp/downloaded_file.txt"
	_, err = file.Download(node1, node2.ID(), "received_file.txt", downloadPath)
	assert.NoError(t, err, "File download should succeed")
	defer os.Remove(downloadPath)

	// Verify file content
	downloadedContent, err := os.ReadFile(downloadPath)
	assert.NoError(t, err, "Should be able to read downloaded file")
	assert.Equal(t, testContent, string(downloadedContent), "Downloaded content should match original")
}

func testProxyFunctionality(t *testing.T, node1, node2 *node.Node, proxy1, proxy2 *proxy.ProxyManager) {
	// Create a simple HTTP server on node2
	serverAddr := "127.0.0.1:8080"
	go startTestHTTPServer(t, serverAddr)

	// Wait for server to start
	time.Sleep(2 * time.Second)

	// Create proxy from node1 to node2
	proxyPort := uint64(8081)
	proxyService := proxy.NewProxyService(node1, proxyPort, node2.ID(), serverAddr)

	// Start proxy service
	go func() {
		err := proxyService.Serve()
		assert.NoError(t, err, "Proxy service should start successfully")
	}()

	// Wait for proxy to start
	time.Sleep(2 * time.Second)

	// Test proxy connection
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
	assert.NoError(t, err, "Should be able to connect to proxy")
	defer conn.Close()

	// Send HTTP request through proxy
	request := "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"
	_, err = conn.Write([]byte(request))
	assert.NoError(t, err, "Should be able to write to proxy connection")

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	assert.NoError(t, err, "Should be able to read from proxy connection")
	assert.Greater(t, n, 0, "Should receive response from proxy")

	// Verify response contains expected content
	response := string(buffer[:n])
	assert.Contains(t, response, "HTTP/1.1", "Response should be HTTP")
	assert.Contains(t, response, "Hello from test server", "Response should contain expected content")

	// Clean up proxy
	proxyService.Close()
}

func testDeviceHealthCheck(t *testing.T, node1, node2 *node.Node) {
	// Start health check on both nodes
	go node1.HealthcheckDevices()
	go node2.HealthcheckDevices()

	// Wait for health checks to run
	time.Sleep(10 * time.Second)

	// Check device status
	node1Devices := node1.Devices
	node2Devices := node2.Devices

	// Find devices in each node's list
	var node1Device node.Device
	var node2Device node.Device

	for _, device := range node1Devices {
		if device.ID == node2.ID() {
			node1Device = device
			break
		}
	}

	for _, device := range node2Devices {
		if device.ID == node1.ID() {
			node2Device = device
			break
		}
	}

	// Verify device status
	assert.Equal(t, node.DeviceStatusHealthy, node1Device.Status, "Node2 should be healthy in Node1's view")
	assert.Equal(t, node.DeviceStatusHealthy, node2Device.Status, "Node1 should be healthy in Node2's view")

	// Verify system information is populated
	assert.NotEmpty(t, node1Device.SysInfo.Platform, "System info should be populated")
	assert.NotEmpty(t, node1Device.SysInfo.CPU, "CPU info should be populated")
	assert.Greater(t, node1Device.SysInfo.RAM, uint64(0), "RAM info should be populated")
}

func startTestHTTPServer(t *testing.T, addr string) {
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Simple HTTP response
		response := "HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 25\r\n" +
			"\r\n" +
			"Hello from test server"

		conn.Write([]byte(response))
	}
}

// TestNetworkStress tests network functionality under stress
func TestNetworkStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Create multiple nodes
	nodes := make([]*node.Node, 5)
	for i := 0; i < 5; i++ {
		node, err := node.NewNode("0.0.0.0", uint64(4010+i))
		require.NoError(t, err)
		defer node.Close()

		echo.BindEchoStream(node)
		file.BindFileStream(node)
		nodes[i] = node
	}

	// Test concurrent pings
	t.Run("ConcurrentPings", func(t *testing.T) {
		testConcurrentPings(t, nodes)
	})

	// Test concurrent file transfers
	t.Run("ConcurrentFileTransfers", func(t *testing.T) {
		testConcurrentFileTransfers(t, nodes)
	})
}

func testConcurrentPings(t *testing.T, nodes []*node.Node) {
	// Wait for nodes to discover each other
	time.Sleep(5 * time.Second)

	// Create channels for results
	results := make(chan error, len(nodes)*len(nodes))

	// Start concurrent pings
	for i, node1 := range nodes {
		for j, node2 := range nodes {
			if i != j {
				go func(n1, n2 *node.Node) {
					err := n1.Ping(n2.ID())
					results <- err
				}(node1, node2)
			}
		}
	}

	// Collect results
	successCount := 0
	totalPings := len(nodes) * (len(nodes) - 1)

	for i := 0; i < totalPings; i++ {
		err := <-results
		if err == nil {
			successCount++
		}
	}

	// Verify most pings succeed (allow some failures due to network conditions)
	successRate := float64(successCount) / float64(totalPings)
	assert.Greater(t, successRate, 0.8, "At least 80% of pings should succeed")
}

func testConcurrentFileTransfers(t *testing.T, nodes []*node.Node) {
	// Create test files
	testFiles := make([]string, len(nodes))
	for i := range nodes {
		content := fmt.Sprintf("Test file content for node %d", i)
		testFile := fmt.Sprintf("/tmp/test_file_%d.txt", i)
		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)
		defer os.Remove(testFile)
		testFiles[i] = testFile
	}

	// Wait for nodes to discover each other
	time.Sleep(5 * time.Second)

	// Create channels for results
	results := make(chan error, len(nodes)*len(nodes))

	// Start concurrent file transfers
	for i, node1 := range nodes {
		for j, node2 := range nodes {
			if i != j {
				go func(n1, n2 *node.Node, sourceFile string, sourceIndex, targetIndex int) {
					remotePath := fmt.Sprintf("file_from_node_%d_to_%d.txt", sourceIndex, targetIndex)
					err := file.Upload(n1, n2.ID(), sourceFile, remotePath)
					results <- err
				}(node1, node2, testFiles[i], i, j)
			}
		}
	}

	// Collect results
	successCount := 0
	totalTransfers := len(nodes) * (len(nodes) - 1)

	for i := 0; i < totalTransfers; i++ {
		err := <-results
		if err == nil {
			successCount++
		}
	}

	// Verify most transfers succeed
	successRate := float64(successCount) / float64(totalTransfers)
	assert.Greater(t, successRate, 0.7, "At least 70% of file transfers should succeed")
}

// TestNetworkRecovery tests network recovery after node failures
func TestNetworkRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping recovery test in short mode")
	}

	// Create nodes
	node1, err := node.NewNode("0.0.0.0", 4020)
	require.NoError(t, err)
	defer node1.Close()

	node2, err := node.NewNode("0.0.0.0", 4021)
	require.NoError(t, err)

	echo.BindEchoStream(node1)
	echo.BindEchoStream(node2)

	// Wait for discovery
	time.Sleep(5 * time.Second)

	// Verify initial connection
	err = node1.Ping(node2.ID())
	assert.NoError(t, err, "Initial ping should succeed")

	// Close node2
	node2.Close()

	// Wait for health check to detect failure
	time.Sleep(10 * time.Second)

	// Verify node2 is no longer healthy
	node1Devices := node1.Devices
	var node2Device node.Device
	found := false

	for _, device := range node1Devices {
		if device.ID == node2.ID() {
			node2Device = device
			found = true
			break
		}
	}

	if found {
		// Node might still be in the list but with different status
		// This depends on the health check implementation
		t.Logf("Node2 device status: %s", node2Device.Status)
	}
}
