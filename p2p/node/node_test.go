package node

import (
	"testing"
	"time"

	"stellar/p2p/util"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestDeviceStatusConstants(t *testing.T) {
	// Test DeviceStatus constants
	assert.Equal(t, DeviceStatus("discovered"), DeviceStatusDiscovered)
	assert.Equal(t, DeviceStatus("healthy"), DeviceStatusHealthy)
}

func TestDeviceStruct(t *testing.T) {
	// Test Device struct creation
	device := &Device{
		ID:             "test-peer-id",
		ReferenceToken: "test-token",
		Status:         DeviceStatusDiscovered,
		SysInfo:        util.SystemInformation{},
		Timestamp:      time.Now(),
	}

	// Verify struct fields
	assert.Equal(t, peer.ID("test-peer-id"), device.ID)
	assert.Equal(t, "test-token", device.ReferenceToken)
	assert.Equal(t, DeviceStatusDiscovered, device.Status)
	assert.NotNil(t, device.SysInfo)
	assert.False(t, device.Timestamp.IsZero())
}

func TestDeviceStructWithSystemInfo(t *testing.T) {
	// Test Device struct with system information
	sysInfo := util.SystemInformation{
		Platform: "test-platform",
		CPU:      "test-cpu",
		GPU:      []string{"test-gpu"},
		RAM:      8192,
	}

	device := &Device{
		ID:             "test-peer-id",
		ReferenceToken: "test-token",
		Status:         DeviceStatusHealthy,
		SysInfo:        sysInfo,
		Timestamp:      time.Now(),
	}

	// Verify struct fields
	assert.Equal(t, peer.ID("test-peer-id"), device.ID)
	assert.Equal(t, "test-token", device.ReferenceToken)
	assert.Equal(t, DeviceStatusHealthy, device.Status)
	assert.Equal(t, "test-platform", device.SysInfo.Platform)
	assert.Equal(t, "test-cpu", device.SysInfo.CPU)
	assert.Equal(t, []string{"test-gpu"}, device.SysInfo.GPU)
	assert.Equal(t, uint64(8192), device.SysInfo.RAM)
}

func TestNodeStruct(t *testing.T) {
	// Test Node struct creation
	node := &Node{
		Bootstrapper:   true,
		RelayNode:      false,
		ReferenceToken: "test-token",
		Policy:         nil,
		Host:           nil,
		DHT:            nil,
		Devices:        make(map[string]Device),
		CTX:            nil,
		Cancel:         nil,
	}

	// Verify struct fields
	assert.True(t, node.Bootstrapper)
	assert.False(t, node.RelayNode)
	assert.Equal(t, "test-token", node.ReferenceToken)
	assert.Nil(t, node.Policy)
	assert.Nil(t, node.Host)
	assert.Nil(t, node.DHT)
	assert.NotNil(t, node.Devices)
	assert.Empty(t, node.Devices)
}

func TestNodeStructWithDevices(t *testing.T) {
	// Test Node struct with devices
	devices := map[string]Device{
		"device1": {
			ID:             "peer1",
			ReferenceToken: "token1",
			Status:         DeviceStatusDiscovered,
			SysInfo:        util.SystemInformation{},
			Timestamp:      time.Now(),
		},
		"device2": {
			ID:             "peer2",
			ReferenceToken: "token2",
			Status:         DeviceStatusHealthy,
			SysInfo:        util.SystemInformation{},
			Timestamp:      time.Now(),
		},
	}

	node := &Node{
		Bootstrapper:   false,
		RelayNode:      true,
		ReferenceToken: "node-token",
		Policy:         nil,
		Host:           nil,
		DHT:            nil,
		Devices:        devices,
		CTX:            nil,
		Cancel:         nil,
	}

	// Verify struct fields
	assert.False(t, node.Bootstrapper)
	assert.True(t, node.RelayNode)
	assert.Equal(t, "node-token", node.ReferenceToken)
	assert.Len(t, node.Devices, 2)
	assert.Contains(t, node.Devices, "device1")
	assert.Contains(t, node.Devices, "device2")
	assert.Equal(t, "token1", node.Devices["device1"].ReferenceToken)
	assert.Equal(t, "token2", node.Devices["device2"].ReferenceToken)
}

func TestBuildListenAddrOptions(t *testing.T) {
	// Test buildListenAddrOptions function
	host := "127.0.0.1"
	port := uint64(4001)

	options := buildListenAddrOptions(host, port)

	// Verify options are returned
	assert.NotNil(t, options)
	assert.Len(t, options, 1) // Should contain one libp2p.Option

	// Note: We can't easily test the internal structure of libp2p.Option
	// but we can verify the function doesn't panic and returns options
}

func TestBuildListenAddrOptionsWithDifferentHosts(t *testing.T) {
	// Test buildListenAddrOptions with different hosts
	testCases := []struct {
		host string
		port uint64
	}{
		{"127.0.0.1", 4001},
		{"0.0.0.0", 4002},
		{"localhost", 4003},
	}

	for _, tc := range testCases {
		t.Run(tc.host, func(t *testing.T) {
			options := buildListenAddrOptions(tc.host, tc.port)
			assert.NotNil(t, options)
			assert.Len(t, options, 1)
		})
	}
}

func TestBuildListenAddrOptionsWithDifferentPorts(t *testing.T) {
	// Test buildListenAddrOptions with different ports
	host := "127.0.0.1"
	ports := []uint64{4001, 4002, 4003, 4004, 4005}

	for _, port := range ports {
		t.Run("port", func(t *testing.T) {
			options := buildListenAddrOptions(host, port)
			assert.NotNil(t, options)
			assert.Len(t, options, 1)
		})
	}
}

func TestDeviceStatusStringConversion(t *testing.T) {
	// Test DeviceStatus string conversion
	tests := []struct {
		status   DeviceStatus
		expected string
	}{
		{DeviceStatusDiscovered, "discovered"},
		{DeviceStatusHealthy, "healthy"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestDeviceStatusComparison(t *testing.T) {
	// Test DeviceStatus comparison
	assert.Equal(t, DeviceStatusDiscovered, DeviceStatus("discovered"))
	assert.Equal(t, DeviceStatusHealthy, DeviceStatus("healthy"))
	assert.NotEqual(t, DeviceStatusDiscovered, DeviceStatusHealthy)
}

func TestDeviceTimestamp(t *testing.T) {
	// Test Device timestamp functionality
	now := time.Now()
	device := &Device{
		ID:        "test-peer",
		Timestamp: now,
	}

	// Verify timestamp is set correctly
	assert.Equal(t, now, device.Timestamp)
	assert.False(t, device.Timestamp.IsZero())
}

func TestNodeDevicesMap(t *testing.T) {
	// Test Node devices map functionality
	node := &Node{
		Devices: make(map[string]Device),
	}

	// Test adding devices
	device1 := Device{ID: "peer1", Status: DeviceStatusDiscovered}
	device2 := Device{ID: "peer2", Status: DeviceStatusHealthy}

	node.Devices["device1"] = device1
	node.Devices["device2"] = device2

	// Verify devices are added
	assert.Len(t, node.Devices, 2)
	assert.Equal(t, device1, node.Devices["device1"])
	assert.Equal(t, device2, node.Devices["device2"])

	// Test removing a device
	delete(node.Devices, "device1")
	assert.Len(t, node.Devices, 1)
	assert.Contains(t, node.Devices, "device2")
	assert.NotContains(t, node.Devices, "device1")
}

func TestDeviceStatusTransitions(t *testing.T) {
	// Test Device status transitions
	device := &Device{
		ID:     "test-peer",
		Status: DeviceStatusDiscovered,
	}

	// Verify initial status
	assert.Equal(t, DeviceStatusDiscovered, device.Status)

	// Test status transition
	device.Status = DeviceStatusHealthy
	assert.Equal(t, DeviceStatusHealthy, device.Status)

	// Test custom status
	device.Status = "custom-status"
	assert.Equal(t, DeviceStatus("custom-status"), device.Status)
}

func BenchmarkBuildListenAddrOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buildListenAddrOptions("127.0.0.1", uint64(4001+i%1000))
	}
}

func BenchmarkDeviceStructCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Device{
			ID:             "test-peer-id",
			ReferenceToken: "test-token",
			Status:         DeviceStatusDiscovered,
			SysInfo:        util.SystemInformation{},
			Timestamp:      time.Now(),
		}
	}
}

func BenchmarkNodeStructCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Node{
			Bootstrapper:   true,
			RelayNode:      false,
			ReferenceToken: "test-token",
			Policy:         nil,
			Host:           nil,
			DHT:            nil,
			Devices:        make(map[string]Device),
			CTX:            nil,
			Cancel:         nil,
		}
	}
}
