package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"stellar/p2p/protocols/common/protocol"
	base_service "stellar/p2p/protocols/common/service"

	"golang.org/x/sync/errgroup"
)

// Server handles server-side proxy operations
type Server struct {
	*base_service.BaseServer
	manager *Manager
}

// NewServer creates a new proxy server with automatic multiplexer setup
func NewServer(conn io.ReadWriteCloser) *Server {
	if conn == nil {
		return nil
	}

	handshakeHandler := &proxyHandshakeHandler{}
	dispatcher := &proxyPacketDispatcher{}

	base := base_service.NewBaseServer(conn, handshakeHandler, dispatcher)
	if base == nil {
		return nil
	}

	server := &Server{
		BaseServer: base,
		manager:    NewManager(),
	}

	// Set server reference in dispatcher
	dispatcher.server = server

	return server
}

// Accept performs handshake and accepts a client connection
// Delegates to BaseServer.Accept()
func (s *Server) Accept() error {
	if s == nil || s.BaseServer == nil {
		return fmt.Errorf("server is nil")
	}
	return s.BaseServer.Accept()
}

// Serve runs the control-plane event loop until the context is canceled or an error occurs.
// Delegates to BaseServer.Serve()
func (s *Server) Serve(ctx context.Context) error {
	if s == nil || s.BaseServer == nil {
		return fmt.Errorf("server is nil")
	}
	return s.BaseServer.Serve(ctx)
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
	if s == nil || s.BaseServer == nil {
		return ""
	}
	return s.BaseServer.ClientID()
}

// Close closes the server and all proxies
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.manager.CloseAll()
	if s.BaseServer != nil {
		return s.BaseServer.Close()
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
	streamID := s.NextStreamID()
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
		return s.ControlConn().WritePacket(responsePacket)
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

	return s.ControlConn().WritePacket(responsePacket)
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

	return s.ControlConn().WritePacket(responsePacket)
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

func (s *Server) sendSuccess(proxyID string, streamID uint32) error {
	packet, err := protocol.NewProxyOpenedPacket(proxyID, true, streamID, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendError(proxyID, msg string) error {
	packet, err := protocol.NewProxyOpenedPacket(proxyID, false, 0, msg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
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
		if s.Multiplexer() != nil {
			_ = s.Multiplexer().CloseStream(streamID)
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
	mux := s.Multiplexer()
	if mux == nil {
		return nil
	}

	const maxAttempts = 30
	const delay = 100 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		if stream, err := mux.GetStream(streamID); err == nil {
			return stream
		}
		time.Sleep(delay)
	}

	return nil
}
