package proxy

import (
	"fmt"
	"stellar/p2p/node"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ProxyManager manages multiple proxy services and handles libp2p protocol registration.
type ProxyManager struct {
	node    *node.Node
	proxies []*ProxyService
	lock    sync.RWMutex
}

// NewProxyManager creates a new proxy manager and registers the libp2p stream handler.
// This should be called once per node during initialization.
func NewProxyManager(node *node.Node) *ProxyManager {
	manager := &ProxyManager{
		node:    node,
		proxies: make([]*ProxyService, 0),
	}

	// Register the stream handler once during initialization
	registerStreamHandler(node)

	return manager
}

// Proxy creates a new proxy service that listens on the specified local port
// and forwards connections to the remote peer's destination address.
func (m *ProxyManager) Proxy(peer peer.ID, hostPort uint64, destAddr string) (proxy *ProxyService, err error) {
	for _, p := range m.proxies {
		if p.Port == hostPort {
			err = fmt.Errorf("proxy port %d already exist", hostPort)
			return
		}
	}

	proxy = NewProxyService(m.node, hostPort, peer, destAddr)
	m.lock.Lock()
	m.proxies = append(m.proxies, proxy)
	m.lock.Unlock()

	if serveErr := proxy.Serve(); serveErr != nil {
		return nil, serveErr
	}
	return
}

// Proxies returns a list of all active proxy services.
func (m *ProxyManager) Proxies() []*ProxyService {
	proxies := make([]*ProxyService, 0)
	m.lock.Lock()
	for _, proxy := range m.proxies {
		if !proxy.Done() {
			proxies = append(proxies, proxy)
		}
	}
	m.proxies = proxies
	m.lock.Unlock()
	return proxies
}

// Close stops and removes the proxy service listening on the specified port.
func (m *ProxyManager) Close(port uint64) {
	m.lock.Lock()
	for _, proxy := range m.proxies {
		if proxy.Port == port {
			if !proxy.Done() {
				proxy.Close()
			}
		}
	}
	m.lock.Unlock()
}
