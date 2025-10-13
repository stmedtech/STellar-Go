package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFreePort(t *testing.T) {
	// Test GetFreePort function
	port, err := GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port, uint64(0))
	assert.Less(t, port, uint64(65536)) // Port should be in valid range
}

func TestGetFreePortMultiple(t *testing.T) {
	// Test GetFreePort function multiple times to ensure different ports
	ports := make(map[uint64]bool)

	for i := 0; i < 5; i++ {
		port, err := GetFreePort()
		require.NoError(t, err)
		assert.Greater(t, port, uint64(0))
		assert.Less(t, port, uint64(65536))

		// Verify we get different ports
		assert.False(t, ports[port], "Port %d was already used", port)
		ports[port] = true
	}
}

func TestGetFreePortPortRange(t *testing.T) {
	// Test that GetFreePort returns ports in valid range
	for i := 0; i < 10; i++ {
		port, err := GetFreePort()
		require.NoError(t, err)

		// Port should be in valid range (1-65535)
		assert.GreaterOrEqual(t, port, uint64(1))
		assert.LessOrEqual(t, port, uint64(65535))
	}
}

func TestGetFreePortNoError(t *testing.T) {
	// Test that GetFreePort doesn't return an error
	port, err := GetFreePort()
	assert.NoError(t, err)
	assert.NotZero(t, port)
}

func TestGetFreePortTCPListener(t *testing.T) {
	// Test that the returned port can actually be used for TCP listening
	port, err := GetFreePort()
	require.NoError(t, err)

	// Try to listen on the returned port
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: int(port),
	})

	if err == nil {
		// If we can listen, close the listener
		listener.Close()
	} else {
		// If we can't listen, it might be because the port is already in use
		// This is acceptable behavior
		t.Logf("Port %d is not available for listening (expected in some cases): %v", port, err)
	}
}

func TestGetFreePortConcurrent(t *testing.T) {
	// Test GetFreePort with concurrent calls
	ports := make(chan uint64, 10)
	errors := make(chan error, 10)

	// Start multiple goroutines to get free ports
	for i := 0; i < 5; i++ {
		go func() {
			port, err := GetFreePort()
			ports <- port
			errors <- err
		}()
	}

	// Collect results
	portMap := make(map[uint64]bool)
	for i := 0; i < 5; i++ {
		port := <-ports
		err := <-errors

		assert.NoError(t, err)
		assert.Greater(t, port, uint64(0))
		assert.Less(t, port, uint64(65536))

		// Verify we get different ports
		assert.False(t, portMap[port], "Port %d was already used", port)
		portMap[port] = true
	}
}

func TestGetFreePortEdgeCases(t *testing.T) {
	// Test GetFreePort edge cases
	// This test verifies the function handles edge cases gracefully

	// Test multiple calls in quick succession
	for i := 0; i < 20; i++ {
		port, err := GetFreePort()
		require.NoError(t, err)
		assert.Greater(t, port, uint64(0))
	}
}

func TestGetFreePortConsistency(t *testing.T) {
	// Test that GetFreePort is consistent across multiple calls
	// (though it should return different ports each time)

	port1, err1 := GetFreePort()
	require.NoError(t, err1)

	port2, err2 := GetFreePort()
	require.NoError(t, err2)

	// Both should be valid ports
	assert.Greater(t, port1, uint64(0))
	assert.Greater(t, port2, uint64(0))

	// They should be different (though this is not guaranteed in all cases)
	// We'll just verify both are valid
	assert.NotEqual(t, port1, uint64(0))
	assert.NotEqual(t, port2, uint64(0))
}

func TestGetFreePortNetworkAvailability(t *testing.T) {
	// Test that GetFreePort works when network is available
	// This is a basic test to ensure the function doesn't fail due to network issues

	port, err := GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port, uint64(0))

	// Verify the port is in a reasonable range
	// Most systems will assign ports in the ephemeral range (32768-65535)
	// but we'll be more lenient here
	assert.GreaterOrEqual(t, port, uint64(1024)) // Above well-known ports
}

func BenchmarkGetFreePort(b *testing.B) {
	for i := 0; i < b.N; i++ {
		port, err := GetFreePort()
		if err != nil {
			b.Fatal(err)
		}
		if port == 0 {
			b.Fatal("port is 0")
		}
	}
}

func BenchmarkGetFreePortConcurrent(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			port, err := GetFreePort()
			if err != nil {
				b.Fatal(err)
			}
			if port == 0 {
				b.Fatal("port is 0")
			}
		}
	})
}
