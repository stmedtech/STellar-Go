//go:build integration

package integration

import (
	"context"
	"io"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"stellar/p2p/node"
	"stellar/p2p/protocols/compute"
	compute_service "stellar/p2p/protocols/compute/service"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	autorelay "github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNode(t *testing.T) *node.Node {
	t.Helper()
	n, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	// Phase 7 focuses on libp2p transport behavior, not auth policy; disable to avoid 403s.
	n.Policy.Enable = false
	return n
}

func echoCmd() (cmd string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", "hello"}
	}
	return "echo", []string{"hello"}
}

func runEcho(t *testing.T, c *compute_service.Client, runID string) {
	t.Helper()
	cmd, args := echoCmd()

	h, err := c.Run(context.Background(), compute_service.RunRequest{
		RunID:   runID,
		Command: cmd,
		Args:    args,
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestE2E_DirectConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server := newTestNode(t)
	defer server.Close()
	compute.BindComputeStream(server)

	clientNode := newTestNode(t)
	defer clientNode.Close()

	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server.ID(), Addrs: server.Host.Addrs()}))
	c, err := compute.DialComputeClient(ctx, clientNode, server.ID())
	require.NoError(t, err)
	defer c.Close()

	runEcho(t, c, "e2e-direct")
}

func TestE2E_RelayConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Dedicated relay host.
	hRelay, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(tcp.NewTCPTransport),
	)
	require.NoError(t, err)
	defer hRelay.Close()
	_, err = relayv2.New(hRelay)
	require.NoError(t, err)

	relayInfo := peer.AddrInfo{ID: hRelay.ID(), Addrs: hRelay.Addrs()}

	server := newTestNode(t)
	defer server.Close()
	compute.BindComputeStream(server)

	clientNode := newTestNode(t)
	defer clientNode.Close()

	// Connect both to relay and reserve slots.
	require.NoError(t, server.Host.Connect(ctx, relayInfo))
	_, err = relayclient.Reserve(ctx, server.Host, relayInfo)
	require.NoError(t, err)

	require.NoError(t, clientNode.Host.Connect(ctx, relayInfo))
	_, err = relayclient.Reserve(ctx, clientNode.Host, relayInfo)
	require.NoError(t, err)

	// Build relay multiaddr to server: /.../p2p/<relay>/p2p-circuit/p2p/<server>
	base := hRelay.Addrs()[0]
	relayPart, _ := ma.NewMultiaddr("/p2p/" + hRelay.ID().String())
	circuit, _ := ma.NewMultiaddr("/p2p-circuit")
	serverPart, _ := ma.NewMultiaddr("/p2p/" + server.ID().String())
	relayToServer := base.Encapsulate(relayPart).Encapsulate(circuit).Encapsulate(serverPart)
	serverInfo, err := peer.AddrInfoFromP2pAddr(relayToServer)
	require.NoError(t, err)

	// Clear dial backoff before connecting via relay.
	if sw, ok := clientNode.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(server.ID())
	}
	require.NoError(t, clientNode.Host.Connect(ctx, *serverInfo))

	// Dial compute over the relayed connection.
	c, err := compute.DialComputeClient(ctx, clientNode, server.ID())
	require.NoError(t, err)
	defer c.Close()

	runEcho(t, c, "e2e-relay")
}

func TestE2E_ConcurrentExecutions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := newTestNode(t)
	defer server.Close()
	compute.BindComputeStream(server)

	clientNode := newTestNode(t)
	defer clientNode.Close()

	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server.ID(), Addrs: server.Host.Addrs()}))
	// Concurrency is tested by running multiple independent compute clients (separate libp2p streams)
	// in parallel. This avoids coupling this E2E gate to the internal scheduling of a single control
	// plane connection while still validating server correctness under concurrent load.
	errCh := make(chan error, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := compute.DialComputeClient(ctx, clientNode, server.ID())
			if err != nil {
				errCh <- err
				return
			}
			defer c.Close()
			runEcho(t, c, "e2e-concurrent-"+strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestE2E_MultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server1 := newTestNode(t)
	defer server1.Close()
	compute.BindComputeStream(server1)

	server2 := newTestNode(t)
	defer server2.Close()
	compute.BindComputeStream(server2)

	clientNode := newTestNode(t)
	defer clientNode.Close()

	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server1.ID(), Addrs: server1.Host.Addrs()}))
	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server2.ID(), Addrs: server2.Host.Addrs()}))

	c1, err := compute.DialComputeClient(ctx, clientNode, server1.ID())
	require.NoError(t, err)
	defer c1.Close()

	c2, err := compute.DialComputeClient(ctx, clientNode, server2.ID())
	require.NoError(t, err)
	defer c2.Close()

	runEcho(t, c1, "e2e-multi-1")
	runEcho(t, c2, "e2e-multi-2")
}

func TestE2E_Reconnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := newTestNode(t)
	defer server.Close()
	compute.BindComputeStream(server)

	clientNode := newTestNode(t)
	defer clientNode.Close()

	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server.ID(), Addrs: server.Host.Addrs()}))

	c1, err := compute.DialComputeClient(ctx, clientNode, server.ID())
	require.NoError(t, err)
	runEcho(t, c1, "e2e-reconnect-1")
	require.NoError(t, c1.Close())

	// Drop the peer connection and re-connect (simulates a reconnect scenario).
	_ = clientNode.Host.Network().ClosePeer(server.ID())
	if sw, ok := clientNode.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(server.ID())
	}
	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: server.ID(), Addrs: server.Host.Addrs()}))

	c2, err := compute.DialComputeClient(ctx, clientNode, server.ID())
	require.NoError(t, err)
	defer c2.Close()

	runEcho(t, c2, "e2e-reconnect-2")
}

// Ensure relay-related dependencies remain linked in integration builds.
var _ network.Stream
var _ = autorelay.WithMinCandidates
