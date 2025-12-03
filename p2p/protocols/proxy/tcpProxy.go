package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/proxy/service"
	"time"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

var logger = golog.Logger("stellar-p2p-protocols-proxy")

// tcpStreamHandler handles incoming libp2p streams for proxy server
func tcpStreamHandler(stream network.Stream) {
	defer stream.Close()

	// Create server with the libp2p stream (which implements io.ReadWriteCloser)
	srv := service.NewServer(stream)
	if srv == nil {
		logger.Warn("Failed to create proxy server")
		return
	}

	// Accept handshake
	if err := srv.Accept(); err != nil {
		logger.Warnf("Proxy handshake failed: %v", err)
		return
	}

	// Serve control plane in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Serve(ctx); err != nil {
			logger.Debugf("Proxy server stopped: %v", err)
		}
	}()

	// Keep handler alive until stream closes
	<-stream.Context().Done()
	srv.Close()
}

type TcpProxyService struct {
	node     *node.Node
	Port     uint64
	Dest     peer.ID
	DestAddr string
	ctx      context.Context
	cancel   context.CancelFunc
	client   *service.Client
	clientMu sync.Mutex
}

func NewTcpProxyService(n *node.Node, port uint64, dest peer.ID, destAddr string) *TcpProxyService {
	ctx, cancel := context.WithCancel(context.Background())

	return &TcpProxyService{
		node:     n,
		Port:     port,
		Dest:     dest,
		DestAddr: destAddr,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (p *TcpProxyService) Bind() {
	p.node.Host.SetStreamHandler(constant.StellarProxyProtocol, p.node.Policy.AuthorizeStream(tcpStreamHandler))
	logger.Info("TCP Proxy server is ready")
}

func (p *TcpProxyService) Close() {
	p.cancel()
	if p.client != nil {
		p.client.CloseAll()
	}
}

func (p *TcpProxyService) Done() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// getOrCreateClient gets or creates a client connection to the destination peer
func (p *TcpProxyService) getOrCreateClient() (*service.Client, error) {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	// Create new stream to destination
	stream, err := p.node.Host.NewStream(p.ctx, p.Dest, constant.StellarProxyProtocol)
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

func (p *TcpProxyService) Serve() error {
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
	defer listener.Close()

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

func (p *TcpProxyService) acceptConnection(conn *net.TCPConn, id string, laddr *net.TCPAddr) {
	defer conn.Close()

	if _, deviceErr := p.node.GetDevice(id); deviceErr != nil {
		p.Close()
		return
	}

	// Get or create client
	client, err := p.getOrCreateClient()
	if err != nil {
		logger.Errorf("Failed to get client: %v", err)
		return
	}

	// Generate proxy ID
	proxyID := fmt.Sprintf("%s:%d", id, p.Port)

	// Open proxy connection using the service client
	proxy, err := client.OpenWithLocalConn(proxyID, p.DestAddr, "tcp", conn)
	if err != nil {
		logger.Errorf("Failed to open proxy: %v", err)
		return
	}

	logger.Debugf("Proxy connection established: %s -> %s", proxyID, p.DestAddr)

	// Wait for context cancellation (connection will be closed by forwarding goroutines)
	<-p.ctx.Done()
	proxy.Close()
}
