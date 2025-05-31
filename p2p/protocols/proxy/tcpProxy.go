package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type TcpProxyService struct {
	node   *node.Node
	dest   peer.ID
	port   uint64
	ctx    context.Context
	cancel context.CancelFunc
}

func NewTcpProxyService(n *node.Node, port uint64, dest peer.ID) *TcpProxyService {
	ctx, cancel := context.WithCancel(context.Background())

	return &TcpProxyService{
		node:   n,
		dest:   dest,
		port:   port,
		ctx:    ctx,
		cancel: cancel,
	}
}

func tcpStreamHandler(stream network.Stream) {
	defer stream.Close()

	buf := bufio.NewReader(stream)

	str, err := buf.ReadString('\n')
	if err != nil {
		logger.Warn("proxy object required")
		return
	}
	str = strings.Trim(str, "\n")

	var p RemoteProxy
	err = json.Unmarshal([]byte(str), &p)
	if err != nil {
		logger.Warnf("proxy object decode error: %v", err)
		return
	}
	p.stream = stream
	defer p.Close("proxy closed", nil)

	p.Start()
}

func (p *TcpProxyService) Bind() {
	p.node.Host.SetStreamHandler(constant.StellarProxyProtocol, tcpStreamHandler)

	logger.Info("TCP Proxy server is ready")
	logger.Info("libp2p-peer addresses:")
	for _, a := range p.node.Host.Addrs() {
		logger.Infof("%s/ipfs/%s", a, p.node.Host.ID())
	}
}

func (p *TcpProxyService) Close() {
	p.cancel()
}

func (p *TcpProxyService) Done() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

func (p *TcpProxyService) Serve(destAddr string) error {
	if destAddr != "" {
		laddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("0.0.0.0:%d", p.port))
		if err != nil {
			logger.Warn("Failed to resolve local address: %s", err)
			return err
		}
		raddr, err := net.ResolveTCPAddr("tcp", destAddr)
		if err != nil {
			logger.Warn("Failed to resolve remote address: %s", err)
			return err
		}
		listener, err := net.ListenTCP("tcp", laddr)
		if err != nil {
			logger.Warn("Failed to open local port to listen: %s", err)
			return err
		}

		logger.Infof("proxy listening on %v for %v", laddr, raddr)

		id := p.dest.String()
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
						logger.Debugf("Failed to accept connection '%s'", err)
						continue
					}

					go p.acceptConnection(conn, id, laddr, raddr)
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
	}

	return nil
}

func (p *TcpProxyService) acceptConnection(conn *net.TCPConn, id string, laddr, raddr *net.TCPAddr) {
	defer conn.Close()

	if _, deviceErr := p.node.GetDevice(id); deviceErr != nil {
		p.Close()
		return
	}

	stream, err := p.node.Host.NewStream(p.ctx, p.dest, constant.StellarProxyProtocol)
	if err != nil {
		logger.Error(err)
		return
	}
	defer stream.Close()

	proxy := NewLocal(conn, laddr, raddr, stream)
	proxy.Nagles = false
	defer proxy.Close("proxy closed", nil)

	remoteProxy := proxy.ToRemoteProxy()
	jsonData, err := json.Marshal(remoteProxy)
	if err != nil {
		logger.Error(err)
		return
	}

	_, err = stream.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
	if err != nil {
		logger.Error(err)
		return
	}

	proxy.Start(p.ctx)
}

type RemoteProxy struct {
	Raddr      *net.TCPAddr
	rconn      io.ReadWriteCloser
	stream     network.Stream
	closed     bool
	errsig     chan bool
	tlsUnwrapp bool
	tlsAddress string

	// Settings
	Nagles bool
}

func NewRemote(raddr *net.TCPAddr, Nagles bool) *RemoteProxy {
	return &RemoteProxy{
		Raddr:  raddr,
		closed: false,
		errsig: make(chan bool),
		Nagles: Nagles,
	}
}

func NewRemoteTLSUnwrapped(raddr *net.TCPAddr, destAddr string, Nagles bool) *RemoteProxy {
	p := NewRemote(raddr, Nagles)
	p.tlsUnwrapp = true
	p.tlsAddress = destAddr
	return p
}

// Start - open connection to remote and start proxying data.
func (p *RemoteProxy) Start() {
	var err error

	//connect to remote
	if p.tlsUnwrapp {
		p.rconn, err = tls.Dial("tcp", p.tlsAddress, nil)
	} else {
		p.rconn, err = net.DialTCP("tcp", nil, p.Raddr)
	}
	if err != nil {
		logger.Warnf("Remote connection failed: %s", err)
		return
	}
	defer p.rconn.Close()

	//nagles?
	if p.Nagles {
		if conn, ok := p.rconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
	}

	//bidirectional copy
	go p.pipe(p.rconn, p.stream)
	go p.pipe(p.stream, p.rconn)

	//wait for close...
	<-p.errsig
}

func (p *RemoteProxy) Close(s string, err error) {
	if p.closed {
		return
	}
	if err != io.EOF {
		logger.Warn(s, err)
	}
	p.errsig <- true
	p.closed = true
}

func (p *RemoteProxy) pipe(src, dst io.ReadWriter) {
	//directional copy (64k buffer)
	buff := make([]byte, 0xffff)
	for {
		if p.closed {
			return
		}

		n, err := src.Read(buff)
		if err != nil {
			p.Close("Read failed '%s'", err)
			return
		}
		b := buff[:n]

		//write out result
		_, err = dst.Write(b)
		if err != nil {
			p.Close("Write failed '%s'", err)
			return
		}
	}
}

// Proxy - Manages a Proxy connection, piping data between local and remote.
type Proxy struct {
	laddr, raddr *net.TCPAddr
	lconn        io.ReadWriteCloser
	stream       network.Stream
	closed       bool
	errsig       chan bool
	tlsUnwrapp   bool
	tlsAddress   string

	// Settings
	Nagles bool
}

// NewLocal - Create a new Proxy instance. Takes over local connection passed in,
// and closes it when finished.
func NewLocal(lconn *net.TCPConn, laddr, raddr *net.TCPAddr, stream network.Stream) *Proxy {
	return &Proxy{
		lconn:  lconn,
		laddr:  laddr,
		raddr:  raddr,
		stream: stream,
		closed: false,
		errsig: make(chan bool),
	}
}

// NewLocalTLSUnwrapped - Create a new Proxy instance with a remote TLS server for
// which we want to unwrap the TLS to be able to connect without encryption
// locally
func NewLocalTLSUnwrapped(lconn *net.TCPConn, laddr, raddr *net.TCPAddr, addr string, stream network.Stream) *Proxy {
	p := NewLocal(lconn, laddr, raddr, stream)
	p.tlsUnwrapp = true
	p.tlsAddress = addr
	return p
}

func (p *Proxy) ToRemoteProxy() *RemoteProxy {
	remoteProxy := NewRemote(p.raddr, p.Nagles)
	return remoteProxy
}

type setNoDelayer interface {
	SetNoDelay(bool) error
}

// Start - open connection to remote and start proxying data.
func (p *Proxy) Start(ctx context.Context) {
	defer p.lconn.Close()

	//nagles?
	if p.Nagles {
		if conn, ok := p.lconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
	}

	//bidirectional copy
	go p.pipe(ctx, p.lconn, p.stream)
	go p.pipe(ctx, p.stream, p.lconn)

	//wait for close...
	<-p.errsig
}

func (p *Proxy) Close(s string, err error) {
	if p.closed {
		return
	}
	if err != io.EOF {
		logger.Warnf(s, err)
	}
	p.errsig <- true
	p.closed = true
}

func (p *Proxy) pipe(ctx context.Context, src, dst io.ReadWriter) {
	//directional copy (64k buffer)
	buff := make([]byte, 0xffff)
	for {
		if p.closed {
			return
		}

		select {
		case <-ctx.Done():
			p.Close("context done", nil)
			return
		default:
			n, err := src.Read(buff)
			if err != nil {
				p.Close("Read failed '%s'", err)
				return
			}
			b := buff[:n]

			//write out result
			_, err = dst.Write(b)
			if err != nil {
				p.Close("Write failed '%s'", err)
				return
			}
		}
	}
}
