package proxy

import (
	"context"
	"fmt"
	"net"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/proxy/service"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ProxyService represents a client-side TCP proxy that listens on a local port
// and forwards connections to a remote peer via the proxy protocol.
type ProxyService struct {
	node     *node.Node
	Port     uint64
	Dest     peer.ID
	DestAddr string
	ctx      context.Context
	cancel   context.CancelFunc
	client   *service.Client
	clientMu sync.Mutex
}

// NewProxyService creates a new client-side proxy service.
// The proxy will listen on the specified local port and forward connections
// to the remote peer's destination address.
func NewProxyService(n *node.Node, port uint64, dest peer.ID, destAddr string) *ProxyService {
	ctx, cancel := context.WithCancel(context.Background())

	return &ProxyService{
		node:     n,
		Port:     port,
		Dest:     dest,
		DestAddr: destAddr,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Close stops the proxy service and closes all associated connections.
func (p *ProxyService) Close() {
	p.cancel()
	if p.client != nil {
		p.client.CloseAll()
	}
}

// Done returns true if the proxy service has been closed.
func (p *ProxyService) Done() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// getOrCreateClient gets or creates a client connection to the destination peer.
// The client is reused across multiple proxy connections to the same peer.
func (p *ProxyService) getOrCreateClient() (*service.Client, error) {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	// Create new stream to destination
	allowCtx := network.WithAllowLimitedConn(p.ctx, string(constant.StellarProxyProtocol))
	stream, err := p.node.Host.NewStream(allowCtx, p.Dest, constant.StellarProxyProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Create client with stream
	clientID := p.node.ID().String()
	client := service.NewClient(clientID, stream)
	if client == nil {
		stream.Close()
		return nil, fmt.Errorf("failed to create proxy client")
	}

	// Connect (perform handshake)
	if err := client.Connect(); err != nil {
		stream.Close()
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	p.client = client
	return client, nil
}

// Serve starts listening on the local port and forwarding connections to the remote peer.
// This method blocks until the service is closed via Close().
func (p *ProxyService) Serve() error {
	if p.DestAddr == "" {
		return fmt.Errorf("destination address required")
	}

	laddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("0.0.0.0:%d", p.Port))
	if err != nil {
		logger.Warnf("Failed to resolve local address: %v", err)
		return err
	}

	listener, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		logger.Warnf("Failed to open local port to listen: %v", err)
		return err
	}

	logger.Infof("Proxy listening on %v for %v", laddr, p.DestAddr)

	id := p.Dest.String()
	cleanup := func() {
		listener.Close()
		p.Close()
	}

	go func() {
		defer cleanup()

		for {
			select {
			case <-p.ctx.Done():
				return
			default:
				conn, err := listener.AcceptTCP()
				if err != nil {
					logger.Debugf("Failed to accept connection: %v", err)
					continue
				}

				go p.acceptConnection(conn, id, laddr)
			}
		}
	}()

	go func() {
		defer cleanup()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				if _, deviceErr := p.node.GetDevice(id); deviceErr != nil {
					logger.Warnf("Device %v disconnected", id)
					return
				}
			}
		}
	}()

	return nil
}

// acceptConnection handles an incoming TCP connection and forwards it through the proxy.
func (p *ProxyService) acceptConnection(conn *net.TCPConn, id string, laddr *net.TCPAddr) {
	defer conn.Close()

	if _, deviceErr := p.node.GetDevice(id); deviceErr != nil {
		p.Close()
		return
	}

	client, err := p.getOrCreateClient()
	if err != nil {
		logger.Errorf("Failed to get client: %v", err)
		return
	}

	proxyID := fmt.Sprintf("%s:%d", id, p.Port)
	_, err = client.OpenWithLocalConn(proxyID, p.DestAddr, "tcp", conn)
	if err != nil {
		logger.Errorf("Failed to open proxy: %v", err)
		return
	}

	logger.Debugf("Proxy connection established: %s -> %s", proxyID, p.DestAddr)
	<-p.ctx.Done()
}
