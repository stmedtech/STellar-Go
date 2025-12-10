package service

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"stellar/p2p/protocols/common/multiplexer"
	"stellar/p2p/protocols/common/protocol"
)

// Client handles client-side proxy operations
type Client struct {
	clientID         string
	controlConn      *protocol.PacketReadWriteCloser
	multiplexer      *multiplexer.Multiplexer
	manager          *Manager
	control          *controlPlane
	handshakeDone    bool
	HandshakeTimeout time.Duration
	mu               sync.RWMutex
	connectMu        sync.Mutex // Protects Connect() from concurrent calls
	writeMu          sync.Mutex // Serializes control-plane writes
	dispatchOnce     sync.Once
	dispatchCtx      context.Context
	dispatchCancel   context.CancelFunc
	pendingMu        sync.Mutex
	pending          map[uint64]*pendingRequest
	nextRequestID    uint64
	events           chan *protocol.HandshakePacket
}

// NewClient creates a new proxy client with automatic multiplexer setup
func NewClient(clientID string, conn io.ReadWriteCloser) *Client {
	if conn == nil || clientID == "" {
		return nil
	}

	mux := multiplexer.NewMultiplexer(conn)
	controlStream, err := mux.ControlStream()
	if err != nil {
		return nil
	}

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return &Client{
		clientID:         clientID,
		controlConn:      packetConn,
		multiplexer:      mux,
		manager:          NewManager(),
		HandshakeTimeout: 30 * time.Second,
		control:          newControlPlane(packetConn),
		pending:          make(map[uint64]*pendingRequest),
		events:           make(chan *protocol.HandshakePacket, 32),
	}
}

// Connect performs handshake and establishes connection
// This method is protected by connectMu to prevent concurrent Connect() calls
func (c *Client) Connect() error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	// Use exclusive lock to prevent concurrent Connect() calls
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	// Check if already connected
	c.mu.RLock()
	alreadyConnected := c.handshakeDone
	c.mu.RUnlock()

	if alreadyConnected {
		return fmt.Errorf("already connected")
	}

	// Send hello
	helloPacket, err := protocol.NewHelloPacket("1.0", c.clientID)
	if err != nil {
		return fmt.Errorf("create hello: %w", err)
	}

	if err := c.controlConn.WritePacket(helloPacket); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	// Read response with timeout
	response, err := c.readHandshakeWithTimeout(c.HandshakeTimeout)
	if err != nil {
		return err
	}

	// Check for error response
	if response.Type == protocol.HandshakeTypeError {
		var errorPayload protocol.ErrorPayload
		if err := response.UnmarshalPayload(&errorPayload); err == nil {
			return fmt.Errorf("%s: %s", errorPayload.Code, errorPayload.Message)
		}
		return fmt.Errorf("handshake error")
	}

	// Verify ack
	if response.Type != protocol.HandshakeTypeHelloAck {
		return fmt.Errorf("unexpected type: %s", response.Type)
	}

	if err := c.ensureDispatcher(); err != nil {
		return err
	}

	// Mark as connected
	c.mu.Lock()
	c.handshakeDone = true
	c.mu.Unlock()
	return nil
}

// Open creates a new proxy connection
func (c *Client) Open(proxyID, remoteAddr, proto string) (*Proxy, error) {
	if err := c.ensureConnected(); err != nil {
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

	response, err := c.sendRequest(context.Background(), requestPacket, matchProxyOpened(proxyID))
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
	stream, err := c.getOrCreateStream(openResponse.StreamID)
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
	defer func() {
		if local != nil {
			local.Close()
		}
		if remote != nil {
			remote.Close()
		}
	}()
	io.Copy(remote, local)
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
	io.Copy(local, remote)
}

// Close closes a proxy
func (c *Client) Close(proxyID string) error {
	if err := c.ensureConnected(); err != nil {
		return err
	}

	if proxyID == "" {
		return fmt.Errorf("proxy ID required")
	}

	requestPacket, err := protocol.NewProxyClosePacket(proxyID)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	response, err := c.sendRequest(context.Background(), requestPacket, matchProxyClosed(proxyID))
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
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	requestPacket, err := protocol.NewProxyListPacket()
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	response, err := c.sendRequest(context.Background(), requestPacket, matchHandshakeType(protocol.HandshakeTypeProxyListResp))
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
	if c == nil {
		return ""
	}
	return c.clientID
}

// CloseAll closes the client and all proxies
func (c *Client) CloseAll() error {
	if c == nil {
		return nil
	}
	if c.dispatchCancel != nil {
		c.dispatchCancel()
	}
	if c.control != nil {
		c.control.Close()
	}
	c.manager.CloseAll()
	if c.multiplexer != nil {
		return c.multiplexer.Close()
	}
	return nil
}

// Helper methods

func (c *Client) ensureConnected() error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	c.mu.RLock()
	done := c.handshakeDone
	c.mu.RUnlock()
	if !done {
		return fmt.Errorf("not connected - call Connect() first")
	}
	return nil
}

func (c *Client) sendRequest(ctx context.Context, requestPacket *protocol.Packet, match matcher) (*protocol.HandshakePacket, error) {
	if err := c.ensureDispatcher(); err != nil {
		return nil, err
	}

	req := c.registerPending(match)

	if err := c.writeControlPacket(requestPacket); err != nil {
		c.completePending(req.id, controlResponse{err: fmt.Errorf("send request: %w", err)})
		return nil, err
	}

	select {
	case resp := <-req.response:
		if resp.err != nil {
			return nil, resp.err
		}
		return resp.packet, nil
	case <-ctx.Done():
		c.completePending(req.id, controlResponse{err: ctx.Err()})
		return nil, ctx.Err()
	}
}

func (c *Client) ensureDispatcher() error {
	var startErr error
	c.dispatchOnce.Do(func() {
		if err := c.control.ensureStarted(); err != nil {
			startErr = err
			return
		}
		c.dispatchCtx, c.dispatchCancel = context.WithCancel(context.Background())
		go c.runDispatcher()
	})
	return startErr
}

func (c *Client) writeControlPacket(packet *protocol.Packet) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.controlConn.WritePacket(packet)
}

func (c *Client) runDispatcher() {
	for {
		packet, err := c.control.Next(c.dispatchCtx)
		if err != nil {
			c.failAllPending(err)
			close(c.events)
			return
		}

		if c.dispatchPending(packet) {
			continue
		}

		select {
		case c.events <- packet:
		default:
		}
	}
}

func (c *Client) dispatchPending(packet *protocol.HandshakePacket) bool {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, req := range c.pending {
		matched, err := req.match(packet)
		if err != nil {
			delete(c.pending, id)
			req.deliver(controlResponse{err: err})
			return true
		}
		if matched {
			delete(c.pending, id)
			req.deliver(controlResponse{packet: packet})
			return true
		}
	}

	return false
}

func (c *Client) registerPending(match matcher) *pendingRequest {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	c.nextRequestID++
	req := &pendingRequest{
		id:       c.nextRequestID,
		match:    match,
		response: make(chan controlResponse, 1),
	}
	c.pending[req.id] = req
	return req
}

func (c *Client) completePending(id uint64, resp controlResponse) {
	c.pendingMu.Lock()
	req, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	if ok {
		req.deliver(resp)
	}
}

func (c *Client) failAllPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, req := range c.pending {
		req.deliver(controlResponse{err: err})
		delete(c.pending, id)
	}
}

func (c *Client) readHandshakeWithTimeout(timeout time.Duration) (*protocol.HandshakePacket, error) {
	type result struct {
		handshake *protocol.HandshakePacket
		err       error
	}

	done := make(chan result, 1)

	go func() {
		packet, err := c.controlConn.ReadPacket()
		if err != nil {
			done <- result{nil, fmt.Errorf("read: %w", err)}
			return
		}

		h, err := protocol.UnmarshalHandshakePacket(packet)
		if err != nil {
			done <- result{nil, fmt.Errorf("unmarshal: %w", err)}
			return
		}

		done <- result{h, nil}
	}()

	select {
	case res := <-done:
		return res.handshake, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func (c *Client) getOrCreateStream(streamID uint32) (io.ReadWriteCloser, error) {
	// Stream ID 0 is reserved for control - should never be used for proxy streams
	// If we get streamID 0, something is wrong
	if streamID == 0 {
		return nil, fmt.Errorf("invalid stream ID 0 - reserved for control stream")
	}

	if c.multiplexer == nil {
		// CRITICAL: If multiplexer is nil, we cannot create proxy streams
		// Returning controlConn would cause HTTP data to be written to stream ID 0
		// This is a serious bug - fail instead of silently using control stream
		return nil, fmt.Errorf("multiplexer is nil - cannot create proxy stream with ID %d. This would cause HTTP data to be written to control stream (ID 0)", streamID)
	}

	// Try to get existing stream
	if stream, err := c.multiplexer.GetStream(streamID); err == nil {
		// Verify the stream is not the control stream
		if stream.ID == 0 {
			return nil, fmt.Errorf("CRITICAL BUG: Retrieved stream has ID 0 (control stream). This would cause HTTP data to be written to control stream")
		}
		return stream, nil
	}

	// Create new stream
	stream, err := c.multiplexer.OpenStreamWithID(streamID)
	if err != nil {
		return nil, err
	}

	// Verify the stream is not the control stream
	if stream.ID == 0 {
		return nil, fmt.Errorf("CRITICAL BUG: Created stream has ID 0 (control stream). This would cause HTTP data to be written to control stream")
	}

	return stream, nil
}

type matcher func(*protocol.HandshakePacket) (bool, error)

type controlResponse struct {
	packet *protocol.HandshakePacket
	err    error
}

type pendingRequest struct {
	id       uint64
	match    matcher
	response chan controlResponse
}

func (p *pendingRequest) deliver(resp controlResponse) {
	select {
	case p.response <- resp:
	default:
	}
}

func matchProxyOpened(expectedProxy string) matcher {
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

func matchProxyClosed(expectedProxy string) matcher {
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

func matchHandshakeType(ht protocol.HandshakeType) matcher {
	return func(h *protocol.HandshakePacket) (bool, error) {
		return h.Type == ht, nil
	}
}
