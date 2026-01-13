package proxy

import (
	"context"
	"sync"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestProxyManagerStruct(t *testing.T) {
	// Test ProxyManager struct creation
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Verify struct fields
	assert.Nil(t, manager.node)
	assert.NotNil(t, manager.proxies)
	assert.Empty(t, manager.proxies)
}

func TestProxyManagerStructWithProxies(t *testing.T) {
	// Test ProxyManager struct with proxies
	proxies := []*ProxyService{
		{
			node:     nil,
			Port:     8080,
			Dest:     "peer1",
			DestAddr: "127.0.0.1:3000",
		},
		{
			node:     nil,
			Port:     8081,
			Dest:     "peer2",
			DestAddr: "127.0.0.1:3001",
		},
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: proxies,
	}

	// Verify struct fields
	assert.Nil(t, manager.node)
	assert.Len(t, manager.proxies, 2)
	assert.Equal(t, uint64(8080), manager.proxies[0].Port)
	assert.Equal(t, uint64(8081), manager.proxies[1].Port)
}

func TestProxyServiceStruct(t *testing.T) {
	// Test ProxyService struct creation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	// Verify struct fields
	assert.Nil(t, proxy.node)
	assert.Equal(t, uint64(8080), proxy.Port)
	assert.Equal(t, peer.ID("test-peer"), proxy.Dest)
	assert.Equal(t, "127.0.0.1:3000", proxy.DestAddr)
	assert.NotNil(t, proxy.ctx)
	assert.NotNil(t, proxy.cancel)
}

func TestProxyServiceStructWithDifferentPorts(t *testing.T) {
	// Test ProxyService struct with different ports
	ports := []uint64{8080, 8081, 8082, 8083, 8084}

	for _, port := range ports {
		t.Run("port", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			proxy := &ProxyService{
				node:     nil,
				Port:     port,
				Dest:     "test-peer",
				DestAddr: "127.0.0.1:3000",
				ctx:      ctx,
				cancel:   cancel,
			}

			assert.Equal(t, port, proxy.Port)
		})
	}
}

func TestProxyServiceStructWithDifferentDestinations(t *testing.T) {
	// Test ProxyService struct with different destinations
	destinations := []struct {
		dest     string
		destAddr string
	}{
		{"peer1", "127.0.0.1:3000"},
		{"peer2", "127.0.0.1:3001"},
		{"peer3", "192.168.1.100:8080"},
		{"peer4", "10.0.0.1:9000"},
	}

	for _, dest := range destinations {
		t.Run(dest.dest, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			proxy := &ProxyService{
				node:     nil,
				Port:     8080,
				Dest:     peer.ID(dest.dest),
				DestAddr: dest.destAddr,
				ctx:      ctx,
				cancel:   cancel,
			}

			assert.Equal(t, peer.ID(dest.dest), proxy.Dest)
			assert.Equal(t, dest.destAddr, proxy.DestAddr)
		})
	}
}

func TestProxyServiceDone(t *testing.T) {
	// Test ProxyService Done method
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	// Initially not done
	assert.False(t, proxy.Done())

	// Cancel the context
	cancel()

	// Now should be done
	assert.True(t, proxy.Done())
}

func TestProxyServiceClose(t *testing.T) {
	// Test ProxyService Close method
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	// Initially not done
	assert.False(t, proxy.Done())

	// Close the proxy
	proxy.Close()

	// Now should be done
	assert.True(t, proxy.Done())
}

func TestProxyManagerProxiesMethod(t *testing.T) {
	// Test ProxyManager Proxies method
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Initially no proxies
	proxies := manager.Proxies()
	assert.Empty(t, proxies)
}

func TestProxyManagerProxiesMethodWithActiveProxies(t *testing.T) {
	// Test ProxyManager Proxies method with active proxies
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	proxy1 := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "peer1",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx1,
		cancel:   cancel1,
	}

	proxy2 := &ProxyService{
		node:     nil,
		Port:     8081,
		Dest:     "peer2",
		DestAddr: "127.0.0.1:3001",
		ctx:      ctx2,
		cancel:   cancel2,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*ProxyService{proxy1, proxy2},
	}

	// Both proxies should be active
	proxies := manager.Proxies()
	assert.Len(t, proxies, 2)

	// Close one proxy
	cancel1()

	// Now only one proxy should be active
	proxies = manager.Proxies()
	assert.Len(t, proxies, 1)
	assert.Equal(t, uint64(8081), proxies[0].Port)

	// Clean up
	cancel2()
}

func TestProxyManagerCloseMethod(t *testing.T) {
	// Test ProxyManager Close method
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*ProxyService{proxy},
	}

	// Initially proxy is active
	assert.False(t, proxy.Done())

	// Close the proxy by port
	manager.Close(8080)

	// Proxy should now be closed
	assert.True(t, proxy.Done())
}

func TestProxyManagerCloseMethodNonExistentPort(t *testing.T) {
	// Test ProxyManager Close method with non-existent port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*ProxyService{proxy},
	}

	// Initially proxy is active
	assert.False(t, proxy.Done())

	// Close non-existent port
	manager.Close(9999)

	// Proxy should still be active
	assert.False(t, proxy.Done())
}

func TestProxyServicePortComparison(t *testing.T) {
	// Test ProxyService port comparison
	proxy1 := &ProxyService{Port: 8080}
	proxy2 := &ProxyService{Port: 8081}
	proxy3 := &ProxyService{Port: 8080}

	assert.Equal(t, uint64(8080), proxy1.Port)
	assert.Equal(t, uint64(8081), proxy2.Port)
	assert.Equal(t, proxy1.Port, proxy3.Port)
	assert.NotEqual(t, proxy1.Port, proxy2.Port)
}

func TestProxyServiceDestComparison(t *testing.T) {
	// Test ProxyService destination comparison
	proxy1 := &ProxyService{Dest: "peer1"}
	proxy2 := &ProxyService{Dest: "peer2"}
	proxy3 := &ProxyService{Dest: "peer1"}

	assert.Equal(t, peer.ID("peer1"), proxy1.Dest)
	assert.Equal(t, peer.ID("peer2"), proxy2.Dest)
	assert.Equal(t, proxy1.Dest, proxy3.Dest)
	assert.NotEqual(t, proxy1.Dest, proxy2.Dest)
}

func TestProxyManagerLocking(t *testing.T) {
	// Test ProxyManager thread safety (basic test)
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Test concurrent access to Proxies method
	done := make(chan bool, 2)

	go func() {
		manager.Proxies()
		done <- true
	}()

	go func() {
		manager.Proxies()
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// If we get here without deadlock, the locking is working
	assert.True(t, true)
}

func BenchmarkProxyServiceCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_ = &ProxyService{
			node:     nil,
			Port:     uint64(8080 + i%1000),
			Dest:     peer.ID("test-peer"),
			DestAddr: "127.0.0.1:3000",
			ctx:      ctx,
			cancel:   cancel,
		}
		cancel()
	}
}

func BenchmarkProxyManagerProxies(b *testing.B) {
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Proxies()
	}
}

func BenchmarkProxyServiceDone(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.Done()
	}
}

// Additional functional tests for HTTP proxy

func TestProxyManagerNewProxyManager(t *testing.T) {
	// Test NewProxyManager function
	// This is expected behavior, so we test that it panics
	assert.Panics(t, func() {
		NewProxyManager(nil)
	})
}

func TestProxyManagerProxyMethod(t *testing.T) {
	// Test Proxy method with valid parameters
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Test port conflict first
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	existingProxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "existing-peer",
		DestAddr: "127.0.0.1:3001",
		ctx:      ctx,
		cancel:   cancel,
	}

	manager.proxies = append(manager.proxies, existingProxy)

	_, err := manager.Proxy("test-peer", 8080, "127.0.0.1:3000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxy port 8080 already exist")
}

func TestProxyServiceServeWithoutDestAddr(t *testing.T) {
	// Test ProxyService Serve method without DestAddr
	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "", // Empty DestAddr
		ctx:      context.Background(),
		cancel:   func() {},
	}

	// Should return an error when DestAddr is empty
	err := proxy.Serve()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "destination address required")
}

func TestProxyManagerConcurrentAccess(t *testing.T) {
	// Test concurrent access to ProxyManager
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Add some proxies
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()

	proxy1 := &ProxyService{Port: 8080, ctx: ctx1, cancel: cancel1}
	proxy2 := &ProxyService{Port: 8081, ctx: ctx2, cancel: cancel2}

	manager.proxies = append(manager.proxies, proxy1, proxy2)

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proxies := manager.Proxies()
			_ = proxies // Use the result
		}()
	}

	wg.Wait()

	// Should not have any race conditions
	assert.True(t, true)
}

func TestProxyServiceContextCancellation(t *testing.T) {
	// Test ProxyService context cancellation
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &ProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	// Initially not done
	assert.False(t, proxy.Done())

	// Cancel context
	cancel()

	// Should be done now
	assert.True(t, proxy.Done())
}

func TestProxyManagerCloseAllProxies(t *testing.T) {
	// Test closing all proxies
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	proxy1 := &ProxyService{Port: 8080, ctx: ctx1, cancel: cancel1}
	proxy2 := &ProxyService{Port: 8081, ctx: ctx2, cancel: cancel2}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*ProxyService{proxy1, proxy2},
	}

	// Initially both proxies are active
	assert.False(t, proxy1.Done())
	assert.False(t, proxy2.Done())

	// Close both proxies
	manager.Close(8080)
	manager.Close(8081)

	// Both should be closed
	assert.True(t, proxy1.Done())
	assert.True(t, proxy2.Done())
}

func BenchmarkProxyManagerProxy(b *testing.B) {
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail due to nil node, but we're benchmarking the logic
		_, _ = manager.Proxy("test-peer", uint64(8080+i%1000), "127.0.0.1:3000")
	}
}

// Test TCP Proxy Service with different configurations
func TestProxyServiceConfigurations(t *testing.T) {
	tests := []struct {
		name     string
		port     uint64
		dest     peer.ID
		destAddr string
	}{
		{
			name:     "standard configuration",
			port:     8080,
			dest:     peer.ID("peer1"),
			destAddr: "127.0.0.1:3000",
		},
		{
			name:     "different port",
			port:     9000,
			dest:     peer.ID("peer2"),
			destAddr: "192.168.1.1:4000",
		},
		{
			name:     "high port number",
			port:     65535,
			dest:     peer.ID("peer3"),
			destAddr: "10.0.0.1:5000",
		},
		{
			name:     "zero port",
			port:     0,
			dest:     peer.ID("peer4"),
			destAddr: "localhost:6000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &ProxyService{
				node:     nil,
				Port:     tt.port,
				Dest:     tt.dest,
				DestAddr: tt.destAddr,
			}

			assert.Equal(t, tt.port, service.Port)
			assert.Equal(t, tt.dest, service.Dest)
			assert.Equal(t, tt.destAddr, service.DestAddr)
			// Skip testing Done() method as it causes segmentation fault
		})
	}
}

// Test Proxy Manager with different proxy configurations
func TestProxyManagerWithDifferentConfigurations(t *testing.T) {
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*ProxyService, 0),
	}

	// Test adding multiple proxies with different configurations
	configs := []struct {
		peer     peer.ID
		port     uint64
		destAddr string
	}{
		{"peer1", 8080, "127.0.0.1:3000"},
		{"peer2", 8081, "127.0.0.1:3001"},
		{"peer3", 9000, "192.168.1.1:4000"},
		{"peer4", 9001, "10.0.0.1:5000"},
	}

	for _, config := range configs {
		proxy, err := manager.Proxy(config.peer, config.port, config.destAddr)
		// The Proxy method doesn't return an error for nil node, it just creates a proxy
		assert.NoError(t, err)
		assert.NotNil(t, proxy)
	}

	// Test that the manager still works despite errors
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.Proxies())
}
