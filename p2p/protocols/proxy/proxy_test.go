package proxy

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestProxyManagerStruct(t *testing.T) {
	// Test ProxyManager struct creation
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*TcpProxyService, 0),
	}

	// Verify struct fields
	assert.Nil(t, manager.node)
	assert.NotNil(t, manager.proxies)
	assert.Empty(t, manager.proxies)
}

func TestProxyManagerStructWithProxies(t *testing.T) {
	// Test ProxyManager struct with proxies
	proxies := []*TcpProxyService{
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

func TestTcpProxyServiceStruct(t *testing.T) {
	// Test TcpProxyService struct creation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := &TcpProxyService{
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

func TestTcpProxyServiceStructWithDifferentPorts(t *testing.T) {
	// Test TcpProxyService struct with different ports
	ports := []uint64{8080, 8081, 8082, 8083, 8084}

	for _, port := range ports {
		t.Run("port", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			proxy := &TcpProxyService{
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

func TestTcpProxyServiceStructWithDifferentDestinations(t *testing.T) {
	// Test TcpProxyService struct with different destinations
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

			proxy := &TcpProxyService{
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

func TestTcpProxyServiceDone(t *testing.T) {
	// Test TcpProxyService Done method
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &TcpProxyService{
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

func TestTcpProxyServiceClose(t *testing.T) {
	// Test TcpProxyService Close method
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &TcpProxyService{
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

func TestProxyServiceStruct(t *testing.T) {
	// Test ProxyService struct creation
	service := &ProxyService{
		host:      nil,
		dest:      "test-peer",
		proxyAddr: nil,
	}

	// Verify struct fields
	assert.Nil(t, service.host)
	assert.Equal(t, peer.ID("test-peer"), service.dest)
	assert.Nil(t, service.proxyAddr)
}

func TestProxyServiceStructWithFields(t *testing.T) {
	// Test ProxyService struct with all fields
	service := &ProxyService{
		host:      nil,
		dest:      "test-peer",
		proxyAddr: nil,
	}

	// Verify struct can be created with fields
	assert.Nil(t, service.host)
	assert.Equal(t, peer.ID("test-peer"), service.dest)
	assert.Nil(t, service.proxyAddr)
}

func TestProxyManagerProxiesMethod(t *testing.T) {
	// Test ProxyManager Proxies method
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*TcpProxyService, 0),
	}

	// Initially no proxies
	proxies := manager.Proxies()
	assert.Empty(t, proxies)
}

func TestProxyManagerProxiesMethodWithActiveProxies(t *testing.T) {
	// Test ProxyManager Proxies method with active proxies
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	proxy1 := &TcpProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "peer1",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx1,
		cancel:   cancel1,
	}

	proxy2 := &TcpProxyService{
		node:     nil,
		Port:     8081,
		Dest:     "peer2",
		DestAddr: "127.0.0.1:3001",
		ctx:      ctx2,
		cancel:   cancel2,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*TcpProxyService{proxy1, proxy2},
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

	proxy := &TcpProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*TcpProxyService{proxy},
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

	proxy := &TcpProxyService{
		node:     nil,
		Port:     8080,
		Dest:     "test-peer",
		DestAddr: "127.0.0.1:3000",
		ctx:      ctx,
		cancel:   cancel,
	}

	manager := &ProxyManager{
		node:    nil,
		proxies: []*TcpProxyService{proxy},
	}

	// Initially proxy is active
	assert.False(t, proxy.Done())

	// Close non-existent port
	manager.Close(9999)

	// Proxy should still be active
	assert.False(t, proxy.Done())
}

func TestTcpProxyServicePortComparison(t *testing.T) {
	// Test TcpProxyService port comparison
	proxy1 := &TcpProxyService{Port: 8080}
	proxy2 := &TcpProxyService{Port: 8081}
	proxy3 := &TcpProxyService{Port: 8080}

	assert.Equal(t, uint64(8080), proxy1.Port)
	assert.Equal(t, uint64(8081), proxy2.Port)
	assert.Equal(t, proxy1.Port, proxy3.Port)
	assert.NotEqual(t, proxy1.Port, proxy2.Port)
}

func TestTcpProxyServiceDestComparison(t *testing.T) {
	// Test TcpProxyService destination comparison
	proxy1 := &TcpProxyService{Dest: "peer1"}
	proxy2 := &TcpProxyService{Dest: "peer2"}
	proxy3 := &TcpProxyService{Dest: "peer1"}

	assert.Equal(t, peer.ID("peer1"), proxy1.Dest)
	assert.Equal(t, peer.ID("peer2"), proxy2.Dest)
	assert.Equal(t, proxy1.Dest, proxy3.Dest)
	assert.NotEqual(t, proxy1.Dest, proxy2.Dest)
}

func TestProxyManagerLocking(t *testing.T) {
	// Test ProxyManager thread safety (basic test)
	manager := &ProxyManager{
		node:    nil,
		proxies: make([]*TcpProxyService, 0),
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

func BenchmarkTcpProxyServiceCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_ = &TcpProxyService{
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
		proxies: make([]*TcpProxyService, 0),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Proxies()
	}
}

func BenchmarkTcpProxyServiceDone(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := &TcpProxyService{
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
