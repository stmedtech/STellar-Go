package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"stellar/p2p/protocols/common/multiplexer"
	"stellar/p2p/protocols/common/protocol"
	"golang.org/x/sync/errgroup"
)

// Server handles server-side proxy operations
type Server struct {
	controlConn      *protocol.PacketReadWriteCloser
	multiplexer      *multiplexer.Multiplexer
	manager          *Manager
	control          *controlPlane
	clientID         string
	handshakeDone    bool
	streamIDCounter  uint32
	HandshakeTimeout time.Duration
	mu               sync.RWMutex
	acceptMu         sync.Mutex // Protects Accept() from concurrent calls
}

// NewServer creates a new proxy server with automatic multiplexer setup
func NewServer(conn io.ReadWriteCloser) *Server {
	if conn == nil {
		return nil
	}

	mux := multiplexer.NewMultiplexer(conn)
	controlStream, err := mux.ControlStream()
	if err != nil {
		return nil
	}

	packetConn := protocol.NewPacketReadWriteCloser(controlStream)
	return &Server{
		controlConn:      packetConn,
		multiplexer:      mux,
		manager:          NewManager(),
		HandshakeTimeout: 30 * time.Second,
		control:          newControlPlane(packetConn),
	}
}

// Accept performs handshake and accepts a client connection
// This method is protected by acceptMu to prevent concurrent Accept() calls
func (s *Server) Accept() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	// Use exclusive lock to prevent concurrent Accept() calls
	s.acceptMu.Lock()
	defer s.acceptMu.Unlock()

	// Check if already accepted
	s.mu.RLock()
	alreadyAccepted := s.handshakeDone
	s.mu.RUnlock()

	if alreadyAccepted {
		return fmt.Errorf("already accepted")
	}

	// Read hello with timeout
	handshake, err := s.readHandshakeWithTimeout(s.HandshakeTimeout)
	if err != nil {
		return err
	}

	if handshake.Type != protocol.HandshakeTypeHello {
		return fmt.Errorf("unexpected type: %s, expected hello", handshake.Type)
	}

	var helloPayload protocol.HelloPayload
	if err := handshake.UnmarshalPayload(&helloPayload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	// Send ack
	ackPacket, err := protocol.NewHelloAckPacket()
	if err != nil {
		return fmt.Errorf("create ack: %w", err)
	}

	if err := s.controlConn.WritePacket(ackPacket); err != nil {
		return fmt.Errorf("send ack: %w", err)
	}

	// Mark as accepted
	s.mu.Lock()
	s.clientID = helloPayload.ClientID
	s.handshakeDone = true
	s.mu.Unlock()
	return nil
}

// Serve runs the control-plane event loop until the context is canceled or an error occurs.
func (s *Server) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureAccepted(); err != nil {
		return err
	}

	for {
		packet, err := s.control.Next(ctx)
		if err != nil {
			return err
		}
		if err := s.dispatchControlPacket(packet); err != nil {
			return err
		}
	}
}

func (s *Server) dispatchControlPacket(handshake *protocol.HandshakePacket) error {
	if handshake == nil {
		return fmt.Errorf("nil handshake packet")
	}

	switch handshake.Type {
	case protocol.HandshakeTypeProxyOpen:
		return s.handleOpen(handshake)
	case protocol.HandshakeTypeProxyClose:
		return s.handleClose(handshake)
	case protocol.HandshakeTypeProxyList:
		return s.handleList(handshake)
	default:
		return fmt.Errorf("unknown type: %s", handshake.Type)
	}
}

// Get gets a proxy by ID
func (s *Server) Get(proxyID string) (*Proxy, bool) {
	if s == nil {
		return nil, false
	}
	return s.manager.GetProxy(proxyID)
}

// Count returns the number of active proxies
func (s *Server) Count() int {
	if s == nil {
		return 0
	}
	return s.manager.Count()
}

// ClientID returns the client ID
func (s *Server) ClientID() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientID
}

// Close closes the server and all proxies
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.control != nil {
		s.control.Close()
	}
	s.manager.CloseAll()
	if s.multiplexer != nil {
		return s.multiplexer.Close()
	}
	return nil
}

// Internal handlers

func (s *Server) handleOpen(handshake *protocol.HandshakePacket) error {
	var request protocol.ProxyOpenRequest
	if err := handshake.UnmarshalPayload(&request); err != nil {
		return s.sendError(request.ProxyID, fmt.Sprintf("invalid request: %v", err))
	}

	// Validate request
	if request.ProxyID == "" {
		return s.sendError("", "proxy ID required")
	}
	if request.RemoteAddr == "" {
		return s.sendError(request.ProxyID, "remote address required")
	}
	if request.Protocol == "" {
		request.Protocol = "tcp"
	}

	// Connect to remote
	remoteConn, err := s.connectRemote(request.RemoteAddr, request.Protocol)
	if err != nil {
		return s.sendError(request.ProxyID, fmt.Sprintf("connect failed: %v", err))
	}

	// Assign stream ID and send success response
	streamID := s.nextStreamID()
	if err := s.sendSuccess(request.ProxyID, streamID); err != nil {
		remoteConn.Close()
		return err
	}

	// Add proxy and start forwarding
	proxy := s.manager.AddProxy(request.ProxyID, request.RemoteAddr, request.Protocol, nil)
	// Store remote connection in proxy for cleanup
	proxy.mu.Lock()
	proxy.remoteConn = remoteConn
	proxy.mu.Unlock()
	go s.forward(proxy, remoteConn, streamID)

	return nil
}

func (s *Server) handleClose(handshake *protocol.HandshakePacket) error {
	var request protocol.ProxyCloseRequest
	if err := handshake.UnmarshalPayload(&request); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.ProxyID == "" {
		responsePacket, err := protocol.NewProxyClosedPacket("", false, "proxy ID required")
		if err != nil {
			return err
		}
		return s.controlConn.WritePacket(responsePacket)
	}

	success := true
	var errMsg string

	// Get proxy before removing it so we can close connections
	proxy, exists := s.manager.GetProxy(request.ProxyID)
	if exists && proxy != nil {
		proxy.SetStatus(ProxyStatusClosing)
		_ = proxy.Close()
	}

	s.manager.RemoveProxy(request.ProxyID)

	responsePacket, err := protocol.NewProxyClosedPacket(request.ProxyID, success, errMsg)
	if err != nil {
		return err
	}

	return s.controlConn.WritePacket(responsePacket)
}

func (s *Server) handleList(handshake *protocol.HandshakePacket) error {
	proxies := s.manager.ListProxies()

	infos := make([]protocol.ProxyInfo, 0, len(proxies))
	for _, proxy := range proxies {
		infos = append(infos, proxy.ToProtocolProxyInfo())
	}

	responsePacket, err := protocol.NewProxyListResponsePacket(infos)
	if err != nil {
		return err
	}

	return s.controlConn.WritePacket(responsePacket)
}

func (s *Server) connectRemote(addr, protocol string) (net.Conn, error) {
	switch protocol {
	case "tcp":
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.Dial("tcp", addr)
	case "udp":
		return nil, fmt.Errorf("UDP not supported")
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

func (s *Server) nextStreamID() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamIDCounter == 0 {
		s.streamIDCounter = 1 // Stream 0 is control
	}
	id := s.streamIDCounter
	s.streamIDCounter++
	return id
}

func (s *Server) sendSuccess(proxyID string, streamID uint32) error {
	packet, err := protocol.NewProxyOpenedPacket(proxyID, true, streamID, "")
	if err != nil {
		return err
	}
	return s.controlConn.WritePacket(packet)
}

func (s *Server) sendError(proxyID, msg string) error {
	packet, err := protocol.NewProxyOpenedPacket(proxyID, false, 0, msg)
	if err != nil {
		return err
	}
	return s.controlConn.WritePacket(packet)
}

func (s *Server) forward(proxy *Proxy, remoteConn net.Conn, streamID uint32) {
	if proxy == nil || remoteConn == nil {
		if remoteConn != nil {
			_ = remoteConn.Close()
		}
		return
	}

	// Wait for client to open the data stream.
	stream := s.waitForStream(streamID)
	if stream == nil {
		_ = remoteConn.Close()
		proxy.SetStatus(ProxyStatusClosed)
		return
	}

	proxy.mu.Lock()
	proxy.Stream = stream
	proxy.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	proxy.setForwardCancel(cancel)
	defer proxy.cancelForwarding()

	defer func() {
		proxy.SetStatus(ProxyStatusClosed)
		if remoteConn != nil {
			remoteConn.Close()
		}
		if stream != nil {
			stream.Close()
		}
		if s.multiplexer != nil {
			_ = s.multiplexer.CloseStream(streamID)
		}
	}()

	group, _ := errgroup.WithContext(ctx)

	group.Go(func() error {
		_, err := io.Copy(stream, remoteConn)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	})

	group.Go(func() error {
		_, err := io.Copy(remoteConn, stream)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	})

	_ = group.Wait()
}

func (s *Server) waitForStream(streamID uint32) io.ReadWriteCloser {
	if s.multiplexer == nil {
		return nil
	}

	const maxAttempts = 30
	const delay = 100 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		if stream, err := s.multiplexer.GetStream(streamID); err == nil {
			return stream
		}
		time.Sleep(delay)
	}

	return nil
}

func (s *Server) readHandshakeWithTimeout(timeout time.Duration) (*protocol.HandshakePacket, error) {
	type result struct {
		handshake *protocol.HandshakePacket
		err       error
	}

	done := make(chan result, 1)

	go func() {
		packet, err := s.controlConn.ReadPacket()
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

func (s *Server) ensureAccepted() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}
	s.mu.RLock()
	done := s.handshakeDone
	s.mu.RUnlock()
	if !done {
		return fmt.Errorf("not accepted - call Accept() first")
	}
	return nil
}

