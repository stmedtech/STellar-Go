package proxy

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"stellar/p2p/constant"
	"strings"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	golog "github.com/ipfs/go-log/v2"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var logger = golog.Logger("stellar-p2p-protocols-proxy")

// ProxyService provides HTTP proxying on top of libp2p by launching an
// HTTP server which tunnels the requests to a destination peer running
// ProxyService too.
type ProxyService struct {
	host      host.Host
	dest      peer.ID
	proxyAddr ma.Multiaddr
}

// NewHttpProxyService attaches a proxy service to the given libp2p Host.
// The proxyAddr parameter specifies the address on which the
// HTTP proxy server listens. The dest parameter specifies the peer
// ID of the remote peer in charge of performing the HTTP requests.
//
// ProxyAddr/dest may be nil/"" it is not necessary that this host
// provides a listening HTTP server (and instead its only function is to
// perform the proxied http requests it receives from a different peer.
//
// The addresses for the dest peer should be part of the host's peerstore.
func NewHttpProxyService(h host.Host, proxyAddr ma.Multiaddr, dest peer.ID) *ProxyService {
	return &ProxyService{
		host:      h,
		dest:      dest,
		proxyAddr: proxyAddr,
	}
}

// streamHandler is our function to handle any libp2p-net streams that belong
// to our protocol. The streams should contain an HTTP request which we need
// to parse, make on behalf of the original node, and then write the response
// on the stream, before closing it.
func streamHandler(stream network.Stream) {
	// Remember to close the stream when we are done.
	defer stream.Close()

	// Create a new buffered reader, as ReadRequest needs one.
	// The buffered reader reads from our stream, on which we
	// have sent the HTTP request (see ServeHTTP())
	buf := bufio.NewReader(stream)
	// Read the HTTP request from the buffer
	req, err := http.ReadRequest(buf)
	if err != nil {
		stream.ResetWithError(406)
		logger.Error(err)
		return
	}
	defer req.Body.Close()

	// We need to reset these fields in the request
	// URL as they are not maintained.
	req.URL.Scheme = "http"
	hp := strings.Split(req.Host, ":")
	if len(hp) > 1 && hp[1] == "443" {
		req.URL.Scheme = "https"
	} else {
		req.URL.Scheme = "http"
	}
	req.URL.Host = req.Host

	outreq := new(http.Request)
	*outreq = *req

	// We now make the request
	logger.Infof("Making request to %s", req.URL)
	resp, err := http.DefaultTransport.RoundTrip(outreq)
	if err != nil {
		stream.ResetWithError(404)
		logger.Error(err)
		return
	}

	// resp.Write writes whatever response we obtained for our
	// request back to the stream.
	resp.Write(stream)
}

func (p *ProxyService) Bind() {
	// We let our host know that it needs to handle streams tagged with the
	// protocol id that we have defined, and then handle them to
	// our own streamHandling function.
	p.host.SetStreamHandler(constant.StellarProxyProtocol, streamHandler)

	logger.Info("Proxy server is ready")
}

// Serve listens on the ProxyService's proxy address. This effectively
// allows to set the listening address as http proxy.
func (p *ProxyService) Serve() {
	_, serveArgs, _ := manet.DialArgs(p.proxyAddr)
	logger.Info("proxy listening on ", serveArgs)
	if p.dest != "" {
		http.ListenAndServe(serveArgs, p)
	}
}

// ServeHTTP implements the http.Handler interface. WARNING: This is the
// simplest approach to a proxy. Therefore, we do not do any of the things
// that should be done when implementing a reverse proxy (like handling
// headers correctly). For how to do it properly, see:
// https://golang.org/src/net/http/httputil/reverseproxy.go?s=3845:3920#L121
//
// ServeHTTP opens a stream to the dest peer for every HTTP request.
// Streams are multiplexed over single connections so, unlike connections
// themselves, they are cheap to create and dispose of.
func (p *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Infof("proxying request for %s to peer %s", r.URL, p.dest)
	// We need to send the request to the remote libp2p peer, so
	// we open a stream to it
	stream, err := p.host.NewStream(context.Background(), p.dest, constant.StellarProxyProtocol)
	// If an error happens, we write an error for response.
	if err != nil {
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// r.Write() writes the HTTP request to the stream.
	err = r.Write(stream)
	if err != nil {
		stream.ResetWithError(500)
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Now we read the response that was sent from the dest
	// peer
	buf := bufio.NewReader(stream)
	resp, err := http.ReadResponse(buf, r)
	if err != nil {
		stream.ResetWithError(500)
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Copy any headers
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	// Write response status and headers
	w.WriteHeader(resp.StatusCode)

	// Finally copy the body
	io.Copy(w, resp.Body)
	resp.Body.Close()
}
