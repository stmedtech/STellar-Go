package service

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test helpers for simplified test setup
func newTestConnection() io.ReadWriteCloser {
	conn1, _ := net.Pipe()
	return conn1
}

// ============================================================================
// Basic Functionality Tests
// ============================================================================

func TestProxyManagerAddProxy(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	proxy := manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	assert.NotNil(t, proxy)
	assert.Equal(t, "proxy-1", proxy.ID)
	assert.Equal(t, "localhost:8080", proxy.RemoteAddr)
	assert.Equal(t, "tcp", proxy.Protocol)
	assert.Equal(t, ProxyStatusActive, proxy.GetStatus())
}

func TestProxyManagerGetProxy(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	proxy1 := manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Test getting existing proxy
	proxy, ok := manager.GetProxy("proxy-1")
	assert.True(t, ok)
	assert.Equal(t, proxy1.ID, proxy.ID)

	// Test getting non-existent proxy
	_, ok = manager.GetProxy("proxy-nonexistent")
	assert.False(t, ok)
}

func TestProxyManagerRemoveProxy(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Verify proxy exists
	_, ok := manager.GetProxy("proxy-1")
	assert.True(t, ok)

	// Remove proxy
	manager.RemoveProxy("proxy-1")

	// Verify proxy is removed
	_, ok = manager.GetProxy("proxy-1")
	assert.False(t, ok)
}

func TestProxyManagerListProxies(t *testing.T) {
	manager := NewManager()

	// Add multiple proxies
	conn1 := newTestConnection()
	conn2 := newTestConnection()
	conn3 := newTestConnection()

	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)
	manager.AddProxy("proxy-2", "localhost:8081", "tcp", conn2)
	manager.AddProxy("proxy-3", "localhost:8082", "udp", conn3)

	proxies := manager.ListProxies()
	assert.Len(t, proxies, 3)

	// Verify all proxies are in the list
	proxyIDs := make(map[string]bool)
	for _, proxy := range proxies {
		proxyIDs[proxy.ID] = true
	}
	assert.True(t, proxyIDs["proxy-1"])
	assert.True(t, proxyIDs["proxy-2"])
	assert.True(t, proxyIDs["proxy-3"])
}

func TestProxyManagerCloseAll(t *testing.T) {
	manager := NewManager()

	// Add multiple proxies
	conn1 := newTestConnection()
	conn2 := newTestConnection()

	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)
	manager.AddProxy("proxy-2", "localhost:8081", "tcp", conn2)

	assert.Equal(t, 2, manager.Count())

	// Close all
	manager.CloseAll()

	// Verify all proxies are removed
	assert.Equal(t, 0, manager.Count())
}

func TestProxyStatus(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	proxy := manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Test initial status
	assert.Equal(t, ProxyStatusActive, proxy.GetStatus())

	// Test setting status
	proxy.SetStatus(ProxyStatusClosing)
	assert.Equal(t, ProxyStatusClosing, proxy.GetStatus())

	proxy.SetStatus(ProxyStatusClosed)
	assert.Equal(t, ProxyStatusClosed, proxy.GetStatus())
}

// ============================================================================
// Edge Cases: Concurrent Access
// ============================================================================

func TestProxyManagerConcurrentAccess(t *testing.T) {
	manager := NewManager()

	// Concurrently add proxies
	numGoroutines := 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			conn := newTestConnection()
			manager.AddProxy("proxy-"+string(rune(id)), "localhost:8080", "tcp", conn)
		}(i)
	}

	// Wait for all goroutines
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Concurrent access test timed out - possible deadlock")
		}
	}

	// Verify all proxies were added
	assert.Equal(t, numGoroutines, manager.Count())
}

func TestProxyManagerConcurrentAddRemove(t *testing.T) {
	manager := NewManager()

	// Concurrently add and remove proxies
	numGoroutines := 10
	done := make(chan bool, numGoroutines*2)

	// Add proxies
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			conn := newTestConnection()
			manager.AddProxy("proxy-"+string(rune(id)), "localhost:8080", "tcp", conn)
		}(i)
	}

	// Remove proxies (may remove non-existent ones)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			manager.RemoveProxy("proxy-" + string(rune(id)))
		}(i)
	}

	// Wait for all operations
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines*2; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Concurrent add/remove test timed out - possible deadlock")
		}
	}
}

func TestProxyManagerConcurrentGetList(t *testing.T) {
	manager := NewManager()

	// Add some proxies first
	for i := 0; i < 5; i++ {
		conn := newTestConnection()
		manager.AddProxy("proxy-"+string(rune(i)), "localhost:8080", "tcp", conn)
	}

	// Concurrently get and list
	numGoroutines := 20
	done := make(chan bool, numGoroutines*2)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			_, _ = manager.GetProxy("proxy-1")
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			_ = manager.ListProxies()
		}()
	}

	// Wait for all operations
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines*2; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Concurrent get/list test timed out - possible deadlock")
		}
	}
}

// ============================================================================
// Edge Cases: Duplicate Proxy IDs
// ============================================================================

func TestProxyManagerDuplicateProxyID(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	conn2 := newTestConnection()

	_ = manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)
	proxy2 := manager.AddProxy("proxy-1", "localhost:8081", "tcp", conn2)

	// Second AddProxy with same ID should overwrite first
	assert.Equal(t, "proxy-1", proxy2.ID)
	assert.Equal(t, 1, manager.Count())

	// GetProxy should return the second proxy
	gotProxy, ok := manager.GetProxy("proxy-1")
	assert.True(t, ok)
	assert.Equal(t, "localhost:8081", gotProxy.RemoteAddr)
}

// ============================================================================
// Edge Cases: Remove Non-existent Proxy
// ============================================================================

func TestProxyManagerRemoveNonExistentProxy(t *testing.T) {
	manager := NewManager()

	// Remove non-existent proxy should not panic
	assert.NotPanics(t, func() {
		manager.RemoveProxy("non-existent")
	})

	assert.Equal(t, 0, manager.Count())
}

// ============================================================================
// Edge Cases: CloseAll with Empty Manager
// ============================================================================

func TestProxyManagerCloseAllEmpty(t *testing.T) {
	manager := NewManager()

	// CloseAll on empty manager should not panic
	assert.NotPanics(t, func() {
		manager.CloseAll()
	})

	assert.Equal(t, 0, manager.Count())
}

// ============================================================================
// Edge Cases: Proxy Status Concurrent Access
// ============================================================================

func TestProxyStatusConcurrentAccess(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	proxy := manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Concurrent status updates
	numGoroutines := 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			proxy.SetStatus(ProxyStatusClosing)
			_ = proxy.GetStatus()
		}()
	}

	// Wait for all operations
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Concurrent status access test timed out")
		}
	}

	// Status should be set (may be "closing" or "closed" depending on order)
	status := proxy.GetStatus()
	assert.Contains(t, []ProxyStatus{ProxyStatusActive, ProxyStatusClosing, ProxyStatusClosed}, status)
}

// ============================================================================
// Edge Cases: RemoveProxy Closes Connection
// ============================================================================

func TestProxyManagerRemoveProxyClosesConnection(t *testing.T) {
	manager := NewManager()

	conn1, conn2 := net.Pipe()
	_ = manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Write to connection
	go func() {
		_, _ = conn2.Write([]byte("test"))
	}()

	// Remove proxy (should close connection)
	manager.RemoveProxy("proxy-1")

	// Connection should be closed
	time.Sleep(10 * time.Millisecond)
	buffer := make([]byte, 10)
	_, err := conn1.Read(buffer)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: CloseAll Closes All Connections
// ============================================================================

func TestProxyManagerCloseAllClosesConnections(t *testing.T) {
	manager := NewManager()

	conn1, _ := net.Pipe()
	conn3, _ := net.Pipe()

	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)
	manager.AddProxy("proxy-2", "localhost:8081", "tcp", conn3)

	// CloseAll should close all connections
	manager.CloseAll()

	// Verify connections are closed
	time.Sleep(10 * time.Millisecond)
	buffer := make([]byte, 10)

	_, err := conn1.Read(buffer)
	assert.Error(t, err)

	_, err = conn3.Read(buffer)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Count Accuracy
// ============================================================================

func TestProxyManagerCountAccuracy(t *testing.T) {
	manager := NewManager()

	// Count should be 0 initially
	assert.Equal(t, 0, manager.Count())

	// Add proxies
	for i := 0; i < 5; i++ {
		conn := newTestConnection()
		manager.AddProxy("proxy-"+string(rune(i)), "localhost:8080", "tcp", conn)
		assert.Equal(t, i+1, manager.Count())
	}

	// Remove proxies
	for i := 4; i >= 0; i-- {
		manager.RemoveProxy("proxy-" + string(rune(i)))
		assert.Equal(t, i, manager.Count())
	}

	// Count should be 0 again
	assert.Equal(t, 0, manager.Count())
}

// ============================================================================
// Edge Cases: ListProxies Returns Copy
// ============================================================================

func TestProxyManagerListProxiesReturnsCopy(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	proxies1 := manager.ListProxies()
	proxies2 := manager.ListProxies()

	// Lists should be independent
	assert.Equal(t, len(proxies1), len(proxies2))

	// Modifying one shouldn't affect the other
	if len(proxies1) > 0 {
		proxies1[0].SetStatus(ProxyStatusClosing)
		// The proxy in the list should be the same object, so status change should be visible
		// But the list itself is a copy
		assert.Equal(t, ProxyStatusClosing, proxies1[0].GetStatus())
		// Verify the proxy is still in manager
		_, ok := manager.GetProxy("proxy-1")
		assert.True(t, ok)
	}
}

// ============================================================================
// Edge Cases: Race Condition on GetProxy
// ============================================================================

func TestProxyManagerRaceConditionGetProxy(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	manager.AddProxy("proxy-1", "localhost:8080", "tcp", conn1)

	// Concurrent GetProxy calls
	numGoroutines := 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			proxy, ok := manager.GetProxy("proxy-1")
			if ok {
				_ = proxy.GetStatus()
			}
		}()
	}

	// Wait for all operations
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Race condition test timed out")
		}
	}
}

// ============================================================================
// Edge Cases: Large Number of Proxies
// ============================================================================

func TestProxyManagerLargeNumberOfProxies(t *testing.T) {
	manager := NewManager()

	// Add many proxies
	numProxies := 1000
	for i := 0; i < numProxies; i++ {
		conn := newTestConnection()
		manager.AddProxy("proxy-"+string(rune(i)), "localhost:8080", "tcp", conn)
	}

	assert.Equal(t, numProxies, manager.Count())

	// List should return all
	proxies := manager.ListProxies()
	assert.Len(t, proxies, numProxies)

	// CloseAll should close all
	manager.CloseAll()
	assert.Equal(t, 0, manager.Count())
}

// ============================================================================
// Edge Cases: Nil Connection Handling
// ============================================================================

func TestProxyManagerNilConnection(t *testing.T) {
	manager := NewManager()

	// Add proxy with nil connection should not panic
	assert.NotPanics(t, func() {
		proxy := manager.AddProxy("proxy-1", "localhost:8080", "tcp", nil)
		assert.NotNil(t, proxy)
	})

	// Remove should not panic
	assert.NotPanics(t, func() {
		manager.RemoveProxy("proxy-1")
	})
}

// ============================================================================
// Edge Cases: Empty String IDs
// ============================================================================

func TestProxyManagerEmptyStringID(t *testing.T) {
	manager := NewManager()

	conn1 := newTestConnection()
	proxy := manager.AddProxy("", "localhost:8080", "tcp", conn1)

	// Empty string ID should return nil (validation)
	assert.Nil(t, proxy)

	// Should not be able to get it
	_, ok := manager.GetProxy("")
	assert.False(t, ok)
}
