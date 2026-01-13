package device

import (
	"stellar/core/config"
	"stellar/core/protocols/compute"
	"stellar/core/socket"
	"stellar/p2p/node"
	"stellar/p2p/protocols/echo"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
)

var logger = golog.Logger("stellar-core-device")

type Device struct {
	Node  *node.Node
	Proxy *proxy.ProxyManager

	opts []libp2p.Option
}

func (d *Device) ImportKey(b64privkey string) {
	opt, privkeyErr := node.LoadPrivateKey(b64privkey)
	if privkeyErr != nil {
		logger.Fatalln(privkeyErr)
	}
	d.opts = append(d.opts, opt)
}

func (d *Device) GenerateKey(seed int64) {
	opt, privkeyErr := node.GeneratePrivateKey(seed)
	if privkeyErr != nil {
		logger.Fatalln(privkeyErr)
	}
	d.opts = append(d.opts, opt)
}

func (d *Device) Init(listenHost string, listenPort uint64) {
	d.InitWithOptions(listenHost, listenPort, false, false, "", false)
}

func (d *Device) InitWithOptions(
	listenHost string,
	listenPort uint64,
	bootstrapper bool,
	relayNode bool,
	b64privkey string,
	debug bool,
) {
	// If b64privkey is provided, use it instead of opts
	var n *node.Node
	var nodeErr error
	if b64privkey != "" {
		n, nodeErr = node.NewNodeWithOptions(
			listenHost,
			listenPort,
			bootstrapper,
			relayNode,
			b64privkey,
			debug,
			d.opts...,
		)
	} else {
		n, nodeErr = node.NewNodeWithOptions(
			listenHost,
			listenPort,
			bootstrapper,
			relayNode,
			"",
			debug,
			d.opts...,
		)
	}
	if nodeErr != nil {
		logger.Fatalln(nodeErr)
	}

	// Initialize file protocol data directory from config
	cfg := config.GetInstance()
	file.InitDataDir(cfg.DataDir)

	echo.BindEchoStream(n)
	file.BindFileStream(n)

	d.Proxy = proxy.NewProxyManager(n)

	compute.BindComputeStream(n)

	d.Node = n
}

func (d *Device) SetReferenceToken(referenceToken string) {
	d.Node.ReferenceToken = referenceToken
}

func (d *Device) StartDiscovery() {
	go d.Node.DiscoverDevices()
	go d.Node.HealthcheckDevices()
}

func (d *Device) StartAPI(port uint64) {
	s := socket.APIServer{Node: d.Node, Proxy: d.Proxy}
	go s.StartServer(port)
}

func (d *Device) StartUnixSocket() {
	s := socket.APIServer{Node: d.Node, Proxy: d.Proxy}
	go s.StartSocket()
}
