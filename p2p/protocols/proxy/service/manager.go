package service

import (
	"fmt"
	"io"
	"sync"

	"stellar/p2p/protocols/common/protocol"
)

// ProxyStatus represents the status of a proxy connection
type ProxyStatus string

const (
	ProxyStatusActive  ProxyStatus = "active"
	ProxyStatusClosing ProxyStatus = "closing"
	ProxyStatusClosed  ProxyStatus = "closed"
)

// Proxy represents an active proxy connection
type Proxy struct {
	ID         string
	RemoteAddr string
	Protocol   string
	Stream     io.ReadWriteCloser
	status     ProxyStatus
	mu         sync.RWMutex
	// remoteConn stores the remote connection for cleanup
	remoteConn io.Closer
	// forwardCancel is a cancellation hook owned by the forwarding goroutines
	forwardCancel func()
}

// SetStatus sets the status of the proxy
func (p *Proxy) SetStatus(status ProxyStatus) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

// GetStatus returns the status of the proxy
func (p *Proxy) GetStatus() ProxyStatus {
	if p == nil {
		return ProxyStatusClosed
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsActive returns true if the proxy is active
func (p *Proxy) IsActive() bool {
	return p.GetStatus() == ProxyStatusActive
}

// IsClosed returns true if the proxy is closed
func (p *Proxy) IsClosed() bool {
	return p.GetStatus() == ProxyStatusClosed
}

// Close closes the proxy stream and remote connection
func (p *Proxy) Close() error {
	if p == nil {
		return nil
	}
	p.cancelForwarding()
	p.SetStatus(ProxyStatusClosed)
	var errs []error
	if p.Stream != nil {
		if err := p.Stream.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if p.remoteConn != nil {
		if err := p.remoteConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Manager manages active proxy connections
type Manager struct {
	proxies map[string]*Proxy
	mu      sync.RWMutex
}

// NewManager creates a new proxy manager
func NewManager() *Manager {
	return &Manager{
		proxies: make(map[string]*Proxy),
	}
}

// AddProxy adds a new proxy to the manager
// If a proxy with the same ID already exists, it will be replaced
func (m *Manager) AddProxy(id, remoteAddr, protocol string, stream io.ReadWriteCloser) *Proxy {
	if m == nil {
		return nil
	}
	if id == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing proxy if it exists
	if existing, ok := m.proxies[id]; ok {
		existing.cancelForwarding()
		if existing.Stream != nil {
			existing.Stream.Close()
		}
	}

	proxy := &Proxy{
		ID:         id,
		RemoteAddr: remoteAddr,
		Protocol:   protocol,
		Stream:     stream,
		status:     ProxyStatusActive,
	}
	m.proxies[id] = proxy
	return proxy
}

// setForwardCancel registers a cancellation function for the proxy's data-plane goroutines.
func (p *Proxy) setForwardCancel(cancel func()) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forwardCancel = cancel
}

// cancelForwarding requests the forwarding goroutines to exit.
func (p *Proxy) cancelForwarding() {
	if p == nil {
		return
	}
	p.mu.Lock()
	cancel := p.forwardCancel
	p.forwardCancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RemoveProxy removes a proxy from the manager and closes its stream
func (m *Manager) RemoveProxy(id string) {
	if m == nil {
		return
	}
	if id == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if proxy, ok := m.proxies[id]; ok {
		if proxy.Stream != nil {
			proxy.Stream.Close()
		}
		delete(m.proxies, id)
	}
}

// GetProxy retrieves a proxy by ID
func (m *Manager) GetProxy(id string) (*Proxy, bool) {
	if m == nil {
		return nil, false
	}
	if id == "" {
		return nil, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	proxy, ok := m.proxies[id]
	return proxy, ok
}

// HasProxy checks if a proxy exists
func (m *Manager) HasProxy(id string) bool {
	_, ok := m.GetProxy(id)
	return ok
}

// ListProxies returns a list of all active proxies
func (m *Manager) ListProxies() []*Proxy {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Proxy, 0, len(m.proxies))
	for _, proxy := range m.proxies {
		list = append(list, proxy)
	}
	return list
}

// Count returns the number of active proxies
func (m *Manager) Count() int {
	if m == nil {
		return 0
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.proxies)
}

// IsEmpty returns true if there are no active proxies
func (m *Manager) IsEmpty() bool {
	return m.Count() == 0
}

// CloseAll closes all active proxies
func (m *Manager) CloseAll() {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, proxy := range m.proxies {
		if proxy.Stream != nil {
			proxy.Stream.Close()
		}
		delete(m.proxies, id)
	}
}

// CloseProxy closes a specific proxy by ID
func (m *Manager) CloseProxy(id string) error {
	proxy, ok := m.GetProxy(id)
	if !ok {
		return nil // Already removed or doesn't exist
	}
	err := proxy.Close()
	m.RemoveProxy(id)
	return err
}

// GetActiveProxies returns only active proxies
func (m *Manager) GetActiveProxies() []*Proxy {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]*Proxy, 0)
	for _, proxy := range m.proxies {
		if proxy.GetStatus() == ProxyStatusActive {
			active = append(active, proxy)
		}
	}
	return active
}

// CloseInactive closes all non-active proxies
func (m *Manager) CloseInactive() {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, proxy := range m.proxies {
		if proxy.GetStatus() != ProxyStatusActive {
			if proxy.Stream != nil {
				proxy.Stream.Close()
			}
			delete(m.proxies, id)
		}
	}
}

// ToProtocolProxyInfo converts Proxy to protocol.ProxyInfo
func (p *Proxy) ToProtocolProxyInfo() protocol.ProxyInfo {
	return protocol.ProxyInfo{
		ProxyID:    p.ID,
		RemoteAddr: p.RemoteAddr,
		Protocol:   p.Protocol,
		Status:     string(p.GetStatus()),
	}
}

