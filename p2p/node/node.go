package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"stellar/p2p/bootstrap"
	"stellar/p2p/constant"
	"stellar/p2p/identity"
	"stellar/p2p/policy"
	"stellar/p2p/util"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	golog "github.com/ipfs/go-log/v2"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
)

var logger = golog.Logger("stellar-p2p-node")

type DeviceStatus string

const (
	DeviceStatusDiscovered DeviceStatus = "discovered"
	DeviceStatusHealthy    DeviceStatus = "healthy"
)

type Device struct {
	ID             peer.ID
	ReferenceToken string
	Status         DeviceStatus
	SysInfo        util.SystemInformation
	Timestamp      time.Time
}

type Node struct {
	Bootstrapper   bool
	RelayNode      bool
	ReferenceToken string
	Policy         *policy.ProtocolPolicy

	Host    host.Host
	DHT     *dht.IpfsDHT
	Devices map[string]Device

	CTX    context.Context
	Cancel context.CancelFunc
	DLock  sync.RWMutex
}

func buildListenAddrOptions(listenHost string, listenPort uint64) []libp2p.Option {
	return []libp2p.Option{
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/%s/tcp/%d", listenHost, listenPort),
			fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", listenHost, listenPort),
		),
	}
}

func NewBootstrapper(
	listenHost string,
	listenPort uint64,
	b64privkey string,
	relayNode bool,
	debug bool,
	opts ...libp2p.Option,
) (n *Node, err error) {
	lopts := []libp2p.Option{
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),

		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,

		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
	}
	lopts = append(lopts, buildListenAddrOptions(listenHost, listenPort)...)

	if debug {
		// prevent "connections per ip limit exceeded" error
		lopts = append(lopts, libp2p.ResourceManager(&network.NullResourceManager{}))
	}

	if b64privkey != "" {
		opt, privkeyErr := LoadPrivateKey(b64privkey)
		if privkeyErr != nil {
			logger.Fatalln(privkeyErr)
		}
		lopts = append(lopts, opt)
	} else {
		opt, privkeyErr := GeneratePrivateKey(0)
		if privkeyErr != nil {
			logger.Fatalln(privkeyErr)
		}
		lopts = append(lopts, opt)
	}

	host, err := libp2p.New(lopts...)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	n = &Node{
		Bootstrapper: true,
		RelayNode:    relayNode,

		CTX:    ctx,
		Cancel: cancel,

		Host:    host,
		DHT:     nil,
		Devices: make(map[string]Device),
	}
	logger.Infof("Node ID: %v", n.ID())

	logger.Infof("Start listening to %v...", n.Host.Addrs())

	if n.RelayNode {
		_, relayErr := relay.New(n.Host)
		if relayErr != nil {
			err = relayErr
			return
		}

		relayInfo := peer.AddrInfo{
			ID:    n.Host.ID(),
			Addrs: n.Host.Addrs(),
		}
		logger.Infof("Connect to this relay node using: %v", relayInfo)
	}

	if _, dhtErr := n.InitDHT(true); dhtErr != nil {
		err = dhtErr
		return
	}

	return
}

func NewNode(
	listenHost string,
	listenPort uint64,
	opts ...libp2p.Option,
) (n *Node, err error) {
	peerChan := make(chan peer.AddrInfo, 100)
	lopts := []libp2p.Option{
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),

		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,

		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),

		libp2p.EnableRelay(),
		libp2p.EnableAutoRelayWithPeerSource(
			func(ctx context.Context, num int) <-chan peer.AddrInfo {
				ch := make(chan peer.AddrInfo, num)

				go func() {
					ctxDone := false
					for i := 0; i < num; i++ {
						select {
						case ai := <-peerChan:
							ch <- ai
						case <-ctx.Done():
							ctxDone = true
						}
						if ctxDone {
							break
						}
					}
					close(ch)
				}()
				return ch
			},
		),
	}
	lopts = append(lopts, buildListenAddrOptions(listenHost, listenPort)...)

	lopts = append(lopts, opts...)

	host, err := libp2p.New(lopts...)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	n = &Node{
		Bootstrapper: false,
		RelayNode:    false,
		Policy: &policy.ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		},

		CTX:    ctx,
		Cancel: cancel,

		Host:    host,
		DHT:     nil,
		Devices: make(map[string]Device),
	}
	logger.Infof("Node ID: %v", n.ID())

	logger.Infof("Start listening to %v...", n.Host.Addrs())

	if _, dhtErr := n.InitDHT(false); dhtErr != nil {
		err = dhtErr
		return
	}

	return
}

func (n *Node) StartMetricsServer(port uint64) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.Handle("/debug/metrics/prometheus", promhttp.Handler())
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
	}()
}

func (n *Node) ID() peer.ID {
	return n.Host.ID()
}

func (n *Node) Close() {
	n.Cancel()
	n.Host.Close()
	n.DHT.Close()
}

func LoadPrivateKey(b64PrivKey string) (libp2p.Option, error) {
	privkey, privkeyErr := identity.DecodePrivateKey(b64PrivKey)
	if privkeyErr != nil {
		return nil, privkeyErr
	}
	return libp2p.Identity(privkey), nil
}

func GeneratePrivateKey(seed int64) (libp2p.Option, error) {
	privkey, privkeyErr := identity.GeneratePrivateKey(seed)
	if privkeyErr != nil {
		return nil, privkeyErr
	}
	return libp2p.Identity(privkey), nil
}

func (n *Node) InitDHT(bootstrapper bool) (anyConnected bool, err error) {
	ctx := n.CTX
	cancel := n.Cancel
	host := n.Host

	var options []dht.Option

	options = append(options, dht.Mode(dht.ModeAutoServer))
	if !bootstrapper {
		options = append(options, dht.BootstrapPeers(bootstrap.Bootstrappers...))
	}

	kademliaDHT, err := dht.New(ctx, host, options...)

	if err != nil {
		cancel()
		return
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		cancel()
		return
	}

	var wg sync.WaitGroup
	for _, bootstrap := range bootstrap.Bootstrappers {
		if bootstrap.ID == n.ID() {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := host.Connect(ctx, bootstrap); err != nil {
				logger.Warnf("Error while connecting to bootstrap node: %q: %v", bootstrap.ID, err)
				return
			}

			n.Policy.AddWhiteList(bootstrap.ID.String())

			anyConnected = true
			logger.Infof("Connection established with bootstrap node: %q", bootstrap.ID)
		}()
	}
	wg.Wait()

	n.DHT = kademliaDHT

	return anyConnected, nil
}

func (n *Node) discoverDevice(peer peer.AddrInfo) (device *Device, err error) {
	id := peer.ID.String()

	if peer.ID == n.ID() {
		sysInfo, sysInfoErr := util.GetSystemInformation()
		if sysInfoErr != nil {
			logger.Fatalln(sysInfoErr)
			return
		}
		n.DLock.Lock()
		device = &Device{
			ID:      peer.ID,
			Status:  "healthy",
			SysInfo: sysInfo,
		}
		n.Devices[id] = *device
		n.DLock.Unlock()
		return
	}

	if pingPongErr := n.Ping(peer.ID); pingPongErr != nil {
		n.Host.Network().ClosePeer(peer.ID)
		n.Host.Peerstore().RemovePeer(peer.ID)
		n.Host.Peerstore().ClearAddrs(peer.ID)
		n.DHT.RoutingTable().RemovePeer(peer.ID)
		n.DHT.RefreshRoutingTable()
		err = pingPongErr
		return
	}

	logger.Debugf("Found peer: %v", peer)

	n.DLock.Lock()
	device = &Device{
		ID:     peer.ID,
		Status: DeviceStatusDiscovered,
	}
	n.Devices[id] = *device
	n.DLock.Unlock()

	connectErr := n.Host.Connect(n.CTX, peer)
	if connectErr != nil {
		err = connectErr
		return
	}

	return
}

func (n *Node) healthCheckDevice(device *Device) (err error) {
	deviceId := device.ID.String()

	disconnect := func() {
		logger.Debugf("disconnecting device %v", device.ID)
		n.DLock.Lock()
		delete(n.Devices, deviceId)
		n.DLock.Unlock()
		if n.Bootstrapper {
			n.Host.Peerstore().RemovePeer(device.ID)
		}
	}

	switch device.Status {
	case DeviceStatusDiscovered:
		deviceInfoStr, deviceInfoErr := n.GetEcho(device.ID, constant.StellarEchoDeviceInfo)
		if deviceInfoErr != nil {
			err = deviceInfoErr
			defer disconnect()
			return
		}

		var d Device
		decodeErr := json.Unmarshal([]byte(deviceInfoStr), &d)
		if decodeErr != nil {
			err = decodeErr
			defer disconnect()
			return
		}
		if d.ID != device.ID {
			err = fmt.Errorf("fatal device handshake, device id not matched: %v, %v", device.ID, d.ID)
			logger.Error(err)
			defer disconnect()
			return
		}

		n.DLock.Lock()
		device.ReferenceToken = d.ReferenceToken
		device.Status = DeviceStatusHealthy
		device.SysInfo = d.SysInfo
		device.Timestamp = time.Now().UTC()
		n.Devices[deviceId] = *device
		n.DLock.Unlock()
	case DeviceStatusHealthy:
		if pingPongErr := n.Ping(device.ID); pingPongErr != nil {
			err = pingPongErr
			defer disconnect()
			return
		}

		n.DLock.Lock()
		device.Timestamp = time.Now().UTC()
		n.Devices[deviceId] = *device
		n.DLock.Unlock()
	}

	return
}

func (n *Node) ConnectDevice(peer peer.AddrInfo) (device *Device, err error) {
	id := peer.ID.String()

	// Skip existing
	if _, deviceErr := n.GetDevice(id); deviceErr == nil {
		err = fmt.Errorf("device %s already connected", id)
		return
	}

	if peer.ID == n.ID() {
		err = fmt.Errorf("device %s is self", id)
		return
	}

	device, discoverErr := n.discoverDevice(peer)
	if discoverErr != nil {
		err = discoverErr
		return
	}

	healthCheckErr := n.healthCheckDevice(device)
	if healthCheckErr != nil {
		err = healthCheckErr
		return
	}

	return
}

func (n *Node) DiscoverDevices() {
	ctx := n.CTX

	kademliaDHT := n.DHT
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, constant.StellarRendezvous)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peerChan, err := routingDiscovery.FindPeers(ctx, constant.StellarRendezvous)
			if err != nil {
				logger.Warn(err)
				continue
			}

			var wg sync.WaitGroup
			for peer := range peerChan {
				id := peer.ID.String()

				// Skip existing
				if _, deviceErr := n.GetDevice(id); deviceErr == nil {
					continue
				}

				wg.Add(1)
				go func() {
					defer wg.Done()

					device, discoverErr := n.discoverDevice(peer)
					if discoverErr != nil {
						logger.Debugf("Failed connecting to %s, error: %s\n", peer.ID.String(), discoverErr)
						return
					}
					logger.Infof("Connected to peer %s", device.ID.String())
				}()
			}
			wg.Wait()
		}
	}
}

func (n *Node) HealthcheckDevices() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-n.CTX.Done():
			return
		case <-ticker.C:
			n.UpdateDevices()
		}
	}
}

func (n *Node) UpdateDevices() {
	var wg sync.WaitGroup
	for _, device := range n.Devices {
		if device.ID == n.ID() {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			healthCheckErr := n.healthCheckDevice(&device)
			if healthCheckErr != nil {
				return
			}
		}()
	}
	wg.Wait()
}

func (n *Node) GetEcho(peer peer.ID, echoCmd string) (echoData string, err error) {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarEchoProtocol)
	if err != nil {
		logger.Debugf("[%v] dial error: %v", echoCmd, err)
		return
	}
	defer s.Close()

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", echoCmd)))
	if err != nil {
		logger.Debugf("[%v] write error: %v", echoCmd, err)
		return
	}

	data, err := io.ReadAll(s)
	if err != nil {
		logger.Debugf("[%v] read error: %v", echoCmd, err)
		return
	}
	if string(data) == constant.StellarEchoUnknownCommand {
		logger.Debugf("[%v] read rejected: unknown command", echoCmd)
		return
	}
	echoData = string(data)
	return
}

func (n *Node) Ping(peer peer.ID) (err error) {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarEchoProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPing)))
	if err != nil {
		return
	}

	data, err := io.ReadAll(s)
	if err != nil {
		return
	}
	if string(data) == constant.StellarEchoUnknownCommand {
		return
	}

	if string(data) != constant.StellarPong {
		err = fmt.Errorf("ping result from %v is not %v", peer, constant.StellarPong)
		return
	}

	return
}

func (n *Node) GetDevice(id string) (device Device, err error) {
	n.DLock.Lock()
	device, exist := n.Devices[id]
	n.DLock.Unlock()
	if !exist {
		err = fmt.Errorf("device not exist: %v", id)
		return
	}

	return device, nil
}

func (n *Node) Provide(peerChan chan peer.AddrInfo) {
	sub, err := n.Host.EventBus().Subscribe(new(event.EvtPeerIdentificationCompleted))
	if err != nil {
		log.Printf("subscription failed: %s", err)
		return
	}
	for {
		select {
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			evt := e.(event.EvtPeerIdentificationCompleted)
			peerChan <- peer.AddrInfo{ID: evt.Peer}
		case <-n.CTX.Done():
			return
		}
	}
}
