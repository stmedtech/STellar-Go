package compute

import (
	"context"
	"fmt"

	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/compute/service"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

var logger = golog.Logger("stellar-p2p-protocols-compute")

// computeStreamHandler is the transport-layer adapter bridging a libp2p stream into the
// compute service server implementation.
func computeStreamHandler(s network.Stream) {
	defer s.Close()

	executor := service.NewRawExecutor()
	srv := service.NewServer(s, executor)
	if srv == nil {
		logger.Warn("failed to create compute server")
		return
	}
	defer srv.Close()

	if err := srv.Accept(); err != nil {
		logger.Warnf("compute handshake failed: %v", err)
		return
	}

	// Serve until the stream closes.
	_ = srv.Serve(context.Background())
}

// BindComputeStream registers the compute protocol handler on the node's libp2p host.
func BindComputeStream(n *node.Node) {
	if n == nil || n.Host == nil || n.Policy == nil {
		return
	}
	n.Host.SetStreamHandler(
		constant.StellarComputeProtocol,
		n.Policy.AuthorizeStream(computeStreamHandler),
	)
	logger.Info("Compute protocol is ready")
}

// DialComputeClient opens a compute protocol stream to the remote peer and returns a connected
// compute service client.
func DialComputeClient(ctx context.Context, n *node.Node, remote peer.ID) (*service.Client, error) {
	if n == nil || n.Host == nil {
		return nil, fmt.Errorf("node is nil")
	}

	allowCtx := network.WithAllowLimitedConn(ctx, string(constant.StellarComputeProtocol))
	s, err := n.Host.NewStream(allowCtx, remote, constant.StellarComputeProtocol)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", remote, err)
	}

	client := service.NewClient(n.Host.ID().String(), s)
	if client == nil {
		_ = s.Close()
		return nil, fmt.Errorf("failed to create compute client")
	}
	if err := client.Connect(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	return client, nil
}
