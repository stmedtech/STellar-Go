package proxy

import (
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

	// proxy := NewHttpProxyService(node.Host, nil, "")
	// proxy.Bind()

	proxy := NewTcpProxyService(node, 0, "")
	proxy.Bind()

	return &manager
}

func (m *ProxyManager) Proxy(peer peer.ID, hostPort uint64, destAddr string) error {
	// proxyAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", hostPort))
	// if err != nil {
	// 	return err
	// }
	// proxy := NewHttpProxyService(n.Host, proxyAddr, peer)
	// proxy.Serve()s

	proxy := NewTcpProxyService(m.node, hostPort, peer)
	m.lock.Lock()
	m.proxies = append(m.proxies, proxy)
	m.lock.Unlock()

	proxy.Serve(destAddr)
	return nil
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
		if proxy.port == port {
			if !proxy.Done() {
				proxy.Close()
			}
		}
	}
	m.lock.Unlock()
}
