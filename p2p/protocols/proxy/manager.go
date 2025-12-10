package proxy

import (
	"fmt"
	"stellar/p2p/node"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

type ProxyManager struct {
	node    *node.Node
	proxies []*TcpProxyService
	lock    sync.RWMutex
}

func NewProxyManager(node *node.Node) *ProxyManager {
	manager := ProxyManager{
		node:    node,
		proxies: make([]*TcpProxyService, 0),
	}

	proxy := NewTcpProxyService(node, 0, "", "")
	proxy.Bind()

	return &manager
}

func (m *ProxyManager) Proxy(peer peer.ID, hostPort uint64, destAddr string) (proxy *TcpProxyService, err error) {
	for _, p := range m.proxies {
		if p.Port == hostPort {
			err = fmt.Errorf("proxy port %d already exist", hostPort)
			return
		}
	}

	proxy = NewTcpProxyService(m.node, hostPort, peer, destAddr)
	m.lock.Lock()
	m.proxies = append(m.proxies, proxy)
	m.lock.Unlock()

	proxy.Serve()
	return
}

func (m *ProxyManager) Proxies() []*TcpProxyService {
	proxies := make([]*TcpProxyService, 0)
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
