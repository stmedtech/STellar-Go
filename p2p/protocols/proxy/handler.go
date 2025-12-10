package proxy

import (
	"context"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/proxy/service"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
)

var logger = golog.Logger("stellar-p2p-protocols-proxy")

// streamHandler handles incoming libp2p streams for the proxy protocol.
// This is the transport layer adapter that bridges libp2p streams to the service layer.
func streamHandler(stream network.Stream) {
	defer stream.Close()

	srv := service.NewServer(stream)
	if srv == nil {
		logger.Warn("Failed to create proxy server")
		return
	}

	if err := srv.Accept(); err != nil {
		logger.Warnf("Proxy handshake failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.Serve(ctx)
	}()

	select {
	case <-serveDone:
		srv.Close()
	case <-ctx.Done():
		srv.Close()
	}
}

// registerStreamHandler registers the proxy protocol stream handler on the node's host.
// This should be called once during ProxyManager initialization.
func registerStreamHandler(node *node.Node) {
	node.Host.SetStreamHandler(
		constant.StellarProxyProtocol,
		node.Policy.AuthorizeStream(streamHandler),
	)
	logger.Info("Proxy protocol stream handler registered")
}
