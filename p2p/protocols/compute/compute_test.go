package compute

import (
	"context"
	"io"
	"runtime"
	"testing"
	"time"

	"stellar/p2p/node"
	"stellar/p2p/protocols/compute/service"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindComputeStream_DirectRunEcho(t *testing.T) {
	// Use a bounded context for dialing/handshake, but do not tie execution lifetime to it.
	// The compute client intentionally completes handles when the Run() ctx is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer serverNode.Close()

	// Policy is enabled by default; allow the client node explicitly.
	// (This test validates the Phase 6 binding under the real policy wrapper.)
	// Note: clientNode isn't created yet; we will add after clientNode creation.

	BindComputeStream(serverNode)

	clientNode, err := node.NewNode("127.0.0.1", 0)
	require.NoError(t, err)
	defer clientNode.Close()

	require.NoError(t, serverNode.Policy.AddWhiteList(clientNode.ID().String()))

	// Connect hosts
	require.NoError(t, clientNode.Host.Connect(ctx, peer.AddrInfo{ID: serverNode.ID(), Addrs: serverNode.Host.Addrs()}))

	client, err := DialComputeClient(ctx, clientNode, serverNode.ID())
	require.NoError(t, err)
	defer client.Close()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		cmd = "echo"
		args = []string{"hello"}
	}

	h, err := client.Run(context.Background(), service.RunRequest{RunID: "phase6-echo", Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}
