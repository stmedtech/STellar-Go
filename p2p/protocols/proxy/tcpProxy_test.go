package proxy

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestProxyServiceNextProxyIDUnique(t *testing.T) {
	p := &ProxyService{
		Port: 9000,
		Dest: peer.ID("peer"),
		ctx:  nil,
		node: nil,
	}

	id1 := p.nextProxyID("peer")
	id2 := p.nextProxyID("peer")

	require.NotEqual(t, id1, id2, "proxy IDs must be unique per connection")
	require.Equal(t, "peer:9000:1", id1)
	require.Equal(t, "peer:9000:2", id2)
}
