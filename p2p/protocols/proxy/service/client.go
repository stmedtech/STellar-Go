package service

import (
	"context"
	"fmt"
	"io"

	"stellar/p2p/protocols/common/protocol"
	base_service "stellar/p2p/protocols/common/service"
)

// Client handles client-side proxy operations
type Client struct {
	*base_service.BaseClient
	manager *Manager
	events  chan *protocol.HandshakePacket
}

// proxyEventsHandler implements EventsHandler for proxy protocol
type proxyEventsHandler struct {
	events chan *protocol.HandshakePacket
}

func (h *proxyEventsHandler) HandleEvent(packet *protocol.HandshakePacket) {
	select {
	case h.events <- packet:
	default:
		// Channel full, drop event
	}
}

// NewClient creates a new proxy client with automatic multiplexer setup
func NewClient(clientID string, conn io.ReadWriteCloser) *Client {
	if conn == nil || clientID == "" {
		return nil
	}

	helloHandler := &proxyHelloHandler{}
	events := make(chan *protocol.HandshakePacket, 32)
	eventsHandler := &proxyEventsHandler{events: events}

	base := base_service.NewBaseClient(clientID, conn, helloHandler, eventsHandler)
	if base == nil {
		return nil
	}

	return &Client{
		BaseClient: base,
		manager:    NewManager(),
		events:     events,
	}
}

// Connect performs handshake and establishes connection
// Delegates to BaseClient.Connect()
func (c *Client) Connect() error {
	if c == nil || c.BaseClient == nil {
		return fmt.Errorf("client is nil")
	}
	return c.BaseClient.Connect()
}

// Open creates a new proxy connection
func (c *Client) Open(proxyID, remoteAddr, proto string) (*Proxy, error) {
	if err := c.BaseClient.EnsureConnected(); err != nil {
		return nil, err
	}

	if proxyID == "" {
		return nil, fmt.Errorf("proxy ID required")
	}
	if remoteAddr == "" {
		return nil, fmt.Errorf("remote address required")
	}
	if proto == "" {
		proto = "tcp"
	}

	// Send request and read response
	requestPacket, err := protocol.NewProxyOpenPacket(proxyID, remoteAddr, proto)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	response, err := c.BaseClient.SendRequest(context.Background(), requestPacket, matchProxyOpened(proxyID))
	if err != nil {
		return nil, err
	}

	// Parse response
	var openResponse protocol.ProxyOpenResponse
	if err := response.UnmarshalPayload(&openResponse); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !openResponse.Success {
		return nil, fmt.Errorf("%s", openResponse.Error)
	}

	// Get or create stream
	stream, err := c.BaseClient.GetOrCreateStream(openResponse.StreamID)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}

	proxy := c.manager.AddProxy(proxyID, remoteAddr, proto, stream)

	// Start forwarding data between local connection and stream
	go c.forwardProxy(proxy, stream)

	return proxy, nil
}

// forwardProxy handles bidirectional data forwarding for a proxy
func (c *Client) forwardProxy(proxy *Proxy, stream io.ReadWriteCloser) {
	// This will be called from ProxyService when a local connection is accepted
	// The actual forwarding logic will be handled by the caller
}

// OpenWithLocalConn creates a proxy and attaches a local connection for forwarding
func (c *Client) OpenWithLocalConn(proxyID, remoteAddr, proto string, localConn io.ReadWriteCloser) (*Proxy, error) {
	proxy, err := c.Open(proxyID, remoteAddr, proto)
	if err != nil {
		return nil, err
	}

	// Start bidirectional forwarding
	go c.forwardLocalToRemote(localConn, proxy.Stream)
	go c.forwardRemoteToLocal(proxy.Stream, localConn)

	return proxy, nil
}

func (c *Client) forwardLocalToRemote(local, remote io.ReadWriteCloser) {
	_, _ = io.Copy(remote, local)
	// Try half-close so the reverse direction can continue.
	if cw, ok := remote.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

func (c *Client) forwardRemoteToLocal(remote, local io.ReadWriteCloser) {
	defer func() {
		if remote != nil {
			remote.Close()
		}
		if local != nil {
			local.Close()
		}
	}()
	_, _ = io.Copy(local, remote)
}

// Close closes a proxy
func (c *Client) Close(proxyID string) error {
	if err := c.BaseClient.EnsureConnected(); err != nil {
		return err
	}

	if proxyID == "" {
		return fmt.Errorf("proxy ID required")
	}

	requestPacket, err := protocol.NewProxyClosePacket(proxyID)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	response, err := c.BaseClient.SendRequest(context.Background(), requestPacket, matchProxyClosed(proxyID))
	if err != nil {
		return err
	}

	var closeResp protocol.ProxyClosedResponse
	if err := response.UnmarshalPayload(&closeResp); err == nil && !closeResp.Success {
		return fmt.Errorf("%s", closeResp.Error)
	}

	c.manager.RemoveProxy(proxyID)
	return nil
}

// List returns all active proxies
func (c *Client) List() ([]*Proxy, error) {
	if err := c.BaseClient.EnsureConnected(); err != nil {
		return nil, err
	}

	requestPacket, err := protocol.NewProxyListPacket()
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	response, err := c.BaseClient.SendRequest(context.Background(), requestPacket, matchHandshakeType(protocol.HandshakeTypeProxyListResp))
	if err != nil {
		return nil, err
	}

	var listResponse protocol.ProxyListResponse
	if err := response.UnmarshalPayload(&listResponse); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Convert to Proxy list
	proxies := make([]*Proxy, 0, len(listResponse.Proxies))
	for _, info := range listResponse.Proxies {
		proxies = append(proxies, &Proxy{
			ID:         info.ProxyID,
			RemoteAddr: info.RemoteAddr,
			Protocol:   info.Protocol,
			status:     ProxyStatus(info.Status),
		})
	}

	return proxies, nil
}

// Get gets a proxy by ID
func (c *Client) Get(proxyID string) (*Proxy, bool) {
	if c == nil {
		return nil, false
	}
	return c.manager.GetProxy(proxyID)
}

// Count returns the number of active proxies
func (c *Client) Count() int {
	if c == nil {
		return 0
	}
	return c.manager.Count()
}

// Events exposes unsolicited control-plane events (e.g. server push notifications).
func (c *Client) Events() <-chan *protocol.HandshakePacket {
	if c == nil {
		return nil
	}
	return c.events
}

// ClientID returns the client ID
func (c *Client) ClientID() string {
	if c == nil || c.BaseClient == nil {
		return ""
	}
	return c.BaseClient.ClientID()
}

// CloseAll closes the client and all proxies
func (c *Client) CloseAll() error {
	if c == nil {
		return nil
	}
	c.manager.CloseAll()
	if c.BaseClient != nil {
		return c.BaseClient.Close()
	}
	return nil
}

// Helper methods

func matchProxyOpened(expectedProxy string) base_service.Matcher {
	return func(h *protocol.HandshakePacket) (bool, error) {
		if h.Type != protocol.HandshakeTypeProxyOpened {
			return false, nil
		}
		var payload protocol.ProxyOpenResponse
		if err := h.UnmarshalPayload(&payload); err != nil {
			return false, err
		}
		return payload.ProxyID == expectedProxy, nil
	}
}

func matchProxyClosed(expectedProxy string) base_service.Matcher {
	return func(h *protocol.HandshakePacket) (bool, error) {
		if h.Type != protocol.HandshakeTypeProxyClosed {
			return false, nil
		}
		var payload protocol.ProxyClosedResponse
		if err := h.UnmarshalPayload(&payload); err != nil {
			return false, err
		}
		return payload.ProxyID == expectedProxy, nil
	}
}

func matchHandshakeType(ht protocol.HandshakeType) base_service.Matcher {
	return func(h *protocol.HandshakePacket) (bool, error) {
		return h.Type == ht, nil
	}
}
