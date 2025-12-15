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
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	ma "github.com/multiformats/go-multiaddr"
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

	Host host.Host
	DHT  *dht.IpfsDHT
	// Internal device storage - use Devices() and DiscoveredDevices() methods
	discoveredDevices map[string]Device
	healthyDevices    map[string]Device

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
		Policy: &policy.ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		},

		CTX:    ctx,
		Cancel: cancel,

		Host:              host,
		DHT:               nil,
		discoveredDevices: make(map[string]Device),
		healthyDevices:    make(map[string]Device),
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

		Host:              host,
		DHT:               nil,
		discoveredDevices: make(map[string]Device),
		healthyDevices:    make(map[string]Device),
	}
	logger.Infof("Node ID: %v", n.ID())

	logger.Infof("Start listening to %v...", n.Host.Addrs())

	// Start providing peers for auto-relay
	go n.Provide(peerChan)

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
		go func(bs peer.AddrInfo) {
			defer wg.Done()

			if err := host.Connect(ctx, bs); err != nil {
				logger.Warnf("Error while connecting to bootstrap node: %q: %v", bs.ID, err)
				return
			}

			n.Policy.AddWhiteList(bs.ID.String())

			anyConnected = true
			logger.Infof("Connection established with bootstrap node: %q", bs.ID)

			// Try to reserve relay slot if bootstrap is a relay node
			resCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, relayErr := relayclient.Reserve(resCtx, host, bs)
			cancel()
			if relayErr == nil {
				logger.Infof("Relay reservation established with bootstrap node: %q", bs.ID)
			} else {
				logger.Debugf("Bootstrap node %q is not a relay or reservation failed: %v", bs.ID, relayErr)
			}
		}(bootstrap)
	}
	wg.Wait()

	n.DHT = kademliaDHT

	return anyConnected, nil
}

func (n *Node) discoverDevice(peerInfo peer.AddrInfo, relayInfos []peer.AddrInfo) (device *Device, err error) {
	id := peerInfo.ID.String()

	if peerInfo.ID == n.ID() {
		sysInfo, sysInfoErr := util.GetSystemInformation()
		if sysInfoErr != nil {
			logger.Fatalln(sysInfoErr)
			return
		}
		n.DLock.Lock()
		device = &Device{
			ID:      peerInfo.ID,
			Status:  DeviceStatusHealthy,
			SysInfo: sysInfo,
		}
		n.healthyDevices[id] = *device
		n.DLock.Unlock()
		return
	}

	logger.Debugf("Found peer: %v", peerInfo)

	n.DLock.Lock()
	device = &Device{
		ID:     peerInfo.ID,
		Status: DeviceStatusDiscovered,
	}
	n.discoveredDevices[id] = *device
	n.DLock.Unlock()

	// Try direct connection first
	connectCtx, cancel := context.WithTimeout(n.CTX, 10*time.Second)
	connectErr := n.Host.Connect(connectCtx, peerInfo)
	cancel()

	if connectErr != nil {
		logger.Debugf("Direct connection to %s failed: %v, trying relay...", peerInfo.ID, connectErr)
		// Direct connection failed, try via relay
		if len(relayInfos) > 0 {
			connected := false
			for _, relayInfo := range relayInfos {
				// Ensure we're connected to the relay first
				if n.Host.Network().Connectedness(relayInfo.ID) != network.Connected {
					logger.Debugf("Not connected to relay %s, skipping", relayInfo.ID)
					continue
				}
				// Get relay addresses from peerstore (may have been updated)
				relayAddrs := n.Host.Peerstore().Addrs(relayInfo.ID)
				if len(relayAddrs) == 0 {
					relayAddrs = relayInfo.Addrs
				}
				if len(relayAddrs) == 0 {
					logger.Debugf("No addresses for relay %s", relayInfo.ID)
					continue
				}
				// Build relay circuit address: relay-addr/p2p/relay-id/p2p-circuit/p2p/target-id
				circuitPath, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit/p2p/%s", relayInfo.ID, peerInfo.ID))
				if err != nil {
					logger.Debugf("Failed to create circuit path: %v", err)
					continue
				}
				for _, relayBaseAddr := range relayAddrs {
					circuitAddr := relayBaseAddr.Encapsulate(circuitPath)
					circuitInfo, err := peer.AddrInfoFromP2pAddr(circuitAddr)
					if err != nil {
						logger.Debugf("Failed to parse circuit address: %v", err)
						continue
					}
					connectCtx, cancel := context.WithTimeout(n.CTX, 15*time.Second)
					err = n.Host.Connect(connectCtx, *circuitInfo)
					cancel()
					if err == nil {
						logger.Infof("Connected to peer %s via relay %s", peerInfo.ID, relayInfo.ID)
						connected = true
						break
					} else {
						logger.Debugf("Relay connection attempt failed via %s: %v", relayInfo.ID, err)
					}
				}
				if connected {
					break
				}
			}
			if !connected {
				n.DLock.Lock()
				delete(n.discoveredDevices, id)
				delete(n.healthyDevices, id)
				n.DLock.Unlock()
				err = fmt.Errorf("failed to connect to %s (direct and relay)", peerInfo.ID)
				logger.Debugf("Connection failed: %v", err)
				return
			}
		} else {
			n.DLock.Lock()
			delete(n.discoveredDevices, id)
			delete(n.healthyDevices, id)
			n.DLock.Unlock()
			err = fmt.Errorf("direct connection failed and no relays available: %v", connectErr)
			logger.Debugf("Connection failed: %v", err)
			return
		}
	} else {
		logger.Debugf("Direct connection to %s succeeded", peerInfo.ID)
	}

	// Verify connection with ping after connecting
	if pingPongErr := n.Ping(peerInfo.ID); pingPongErr != nil {
		n.Host.Network().ClosePeer(peerInfo.ID)
		n.Host.Peerstore().RemovePeer(peerInfo.ID)
		n.Host.Peerstore().ClearAddrs(peerInfo.ID)
		if n.DHT != nil {
			n.DHT.RoutingTable().RemovePeer(peerInfo.ID)
			n.DHT.RefreshRoutingTable()
		}
		n.DLock.Lock()
		delete(n.discoveredDevices, id)
		delete(n.healthyDevices, id)
		n.DLock.Unlock()
		err = pingPongErr
		return
	}

	return
}

func (n *Node) healthCheckDevice(device *Device) (err error) {
	deviceId := device.ID.String()

	disconnect := func() {
		logger.Debugf("disconnecting device %v", device.ID)
		n.DLock.Lock()
		delete(n.discoveredDevices, deviceId)
		delete(n.healthyDevices, deviceId)
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
		// Move from discovered to healthy
		delete(n.discoveredDevices, deviceId)
		n.healthyDevices[deviceId] = *device
		n.DLock.Unlock()
	case DeviceStatusHealthy:
		if pingPongErr := n.Ping(device.ID); pingPongErr != nil {
			err = pingPongErr
			defer disconnect()
			return
		}

		n.DLock.Lock()
		device.Timestamp = time.Now().UTC()
		n.healthyDevices[deviceId] = *device
		n.DLock.Unlock()
	}

	return
}

func (n *Node) ConnectDevice(peerInfo peer.AddrInfo) (device *Device, err error) {
	id := peerInfo.ID.String()

	// Skip existing
	if _, deviceErr := n.GetDevice(id); deviceErr == nil {
		err = fmt.Errorf("device %s already connected", id)
		return
	}

	if peerInfo.ID == n.ID() {
		err = fmt.Errorf("device %s is self", id)
		return
	}

	// Get relay addresses from bootstrap nodes
	relayInfos := make([]peer.AddrInfo, 0)
	for _, bootstrap := range bootstrap.Bootstrappers {
		if bootstrap.ID == n.ID() || bootstrap.ID == peerInfo.ID {
			continue
		}
		if n.Host.Network().Connectedness(bootstrap.ID) == network.Connected {
			resCtx, cancel := context.WithTimeout(n.CTX, 5*time.Second)
			_, relayErr := relayclient.Reserve(resCtx, n.Host, bootstrap)
			cancel()
			if relayErr == nil {
				relayInfos = append(relayInfos, bootstrap)
			}
		}
	}

	device, discoverErr := n.discoverDevice(peerInfo, relayInfos)
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
	if kademliaDHT == nil {
		logger.Warn("DHT not initialized, cannot discover devices")
		return
	}

	// Wait for DHT bootstrap to complete
	time.Sleep(3 * time.Second)

	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	connectedPeers := make(map[peer.ID]bool)
	advertised := false
	advertiseTime := time.Time{}

	// Function to refresh relay list from bootstrap nodes
	refreshRelayInfos := func() []peer.AddrInfo {
		relayInfos := make([]peer.AddrInfo, 0)
		for _, bootstrap := range bootstrap.Bootstrappers {
			if bootstrap.ID == n.ID() {
				continue
			}
			// Check if bootstrap node is connected
			if n.Host.Network().Connectedness(bootstrap.ID) == network.Connected {
				// Check if we have a relay reservation (by checking peerstore for relay addresses)
				// If connected, assume it might be a relay and include it
				// The actual relay check happens during connection attempt
				relayInfos = append(relayInfos, bootstrap)
			}
		}
		return relayInfos
	}

	// Initial relay list
	relayInfos := refreshRelayInfos()

	advertiseTicker := time.NewTicker(2 * time.Second)
	defer advertiseTicker.Stop()

	findTicker := time.NewTicker(5 * time.Second)
	defer findTicker.Stop()

	refreshTicker := time.NewTicker(30 * time.Second)
	defer refreshTicker.Stop()

	relayRefreshTicker := time.NewTicker(15 * time.Second)
	defer relayRefreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-advertiseTicker.C:
			if !advertised {
				ttl, err := routingDiscovery.Advertise(ctx, constant.StellarRendezvous)
				if err != nil {
					logger.Debugf("Failed to advertise: %v", err)
					continue
				}
				logger.Debugf("discovery: advertising on '%s' (TTL: %v)", constant.StellarRendezvous, ttl)
				advertised = true
				advertiseTime = time.Now()
				advertiseTicker.Reset(30 * time.Second)
			} else {
				routingDiscovery.Advertise(ctx, constant.StellarRendezvous)
			}
		case <-refreshTicker.C:
			if kademliaDHT != nil {
				go kademliaDHT.RefreshRoutingTable()
			}
		case <-relayRefreshTicker.C:
			// Periodically refresh relay list as connections are established
			relayInfos = refreshRelayInfos()
			if len(relayInfos) > 0 {
				logger.Debugf("Refreshed relay list: %d relay(s) available", len(relayInfos))
			}
		case <-findTicker.C:
			if !advertised {
				continue
			}
			// Wait for advertise to propagate before finding peers
			if !advertiseTime.IsZero() && time.Since(advertiseTime) < 20*time.Second {
				continue
			}
			findCtx, findCancel := context.WithTimeout(ctx, 30*time.Second)
			peerChan, err := routingDiscovery.FindPeers(findCtx, constant.StellarRendezvous)
			if err != nil {
				findCancel()
				logger.Debugf("Failed to find peers: %v", err)
				continue
			}

			foundCount := 0
			newPeers := 0
			var wg sync.WaitGroup
			for p := range peerChan {
				if p.ID == "" || p.ID == n.ID() {
					continue
				}
				foundCount++

				// Skip if already connected
				if connectedPeers[p.ID] && n.Host.Network().Connectedness(p.ID) == network.Connected {
					continue
				}
				if connectedPeers[p.ID] {
					delete(connectedPeers, p.ID)
				}

				// Skip existing devices (check both discovered and healthy)
				id := p.ID.String()
				n.DLock.RLock()
				_, existsHealthy := n.healthyDevices[id]
				_, existsDiscovered := n.discoveredDevices[id]
				n.DLock.RUnlock()
				if existsHealthy || existsDiscovered {
					continue
				}

				wg.Add(1)
				go func(peerInfo peer.AddrInfo) {
					defer wg.Done()

					device, discoverErr := n.discoverDevice(peerInfo, relayInfos)
					if discoverErr != nil {
						logger.Debugf("Failed connecting to %s, error: %s", peerInfo.ID.String(), discoverErr)
						return
					}
					connectedPeers[peerInfo.ID] = true
					newPeers++
					logger.Infof("Connected to peer %s", device.ID.String())
				}(p)
			}
			wg.Wait()
			findCancel()
			if foundCount > 0 {
				logger.Debugf("discovery: found %d peer(s) (%d new connections)", foundCount, newPeers)
			}
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
	// Check both discovered and healthy devices
	n.DLock.RLock()
	allDevices := make([]Device, 0, len(n.discoveredDevices)+len(n.healthyDevices))
	for _, device := range n.discoveredDevices {
		allDevices = append(allDevices, device)
	}
	for _, device := range n.healthyDevices {
		allDevices = append(allDevices, device)
	}
	n.DLock.RUnlock()

	for _, device := range allDevices {
		if device.ID == n.ID() {
			continue
		}

		wg.Add(1)
		go func(d Device) {
			defer wg.Done()

			healthCheckErr := n.healthCheckDevice(&d)
			if healthCheckErr != nil {
				return
			}
		}(device)
	}
	wg.Wait()
}

func (n *Node) GetEcho(peer peer.ID, echoCmd string) (echoData string, err error) {
	// Allow limited connections (e.g., relay connections)
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarEchoProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarEchoProtocol)
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
	// Allow limited connections (e.g., relay connections)
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarEchoProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarEchoProtocol)
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

// Devices returns a map of healthy devices only
func (n *Node) Devices() map[string]Device {
	n.DLock.RLock()
	defer n.DLock.RUnlock()
	// Return a copy to prevent external modification
	result := make(map[string]Device, len(n.healthyDevices))
	for k, v := range n.healthyDevices {
		result[k] = v
	}
	return result
}

// DiscoveredDevices returns a map of discovered (but not yet healthy) devices
func (n *Node) DiscoveredDevices() map[string]Device {
	n.DLock.RLock()
	defer n.DLock.RUnlock()
	// Return a copy to prevent external modification
	result := make(map[string]Device, len(n.discoveredDevices))
	for k, v := range n.discoveredDevices {
		result[k] = v
	}
	return result
}

// GetDevice returns a healthy device by ID, or error if not found
func (n *Node) GetDevice(id string) (device Device, err error) {
	n.DLock.RLock()
	device, exist := n.healthyDevices[id]
	n.DLock.RUnlock()
	if !exist {
		err = fmt.Errorf("device not exist: %v", id)
		return
	}

	return device, nil
}

// GetDiscoveredDevice returns a discovered device by ID, or error if not found
func (n *Node) GetDiscoveredDevice(id string) (device Device, err error) {
	n.DLock.RLock()
	device, exist := n.discoveredDevices[id]
	n.DLock.RUnlock()
	if !exist {
		err = fmt.Errorf("discovered device not exist: %v", id)
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
