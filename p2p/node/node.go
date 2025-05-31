package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"stellar/p2p/bootstrap"
	"stellar/p2p/constant"
	"stellar/p2p/identity"
	"stellar/p2p/util"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	golog "github.com/ipfs/go-log/v2"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

var logger = golog.Logger("stellar-p2p-node")

type Device struct {
	ID             peer.ID
	ReferenceToken string
	Status         string
	RelayAddr      string
	SysInfo        util.SystemInformation
	Timestamp      time.Time
}

type Node struct {
	Bootstrapper   bool
	RelayNode      bool
	ReferenceToken string

	Host      host.Host
	DHT       *dht.IpfsDHT
	Devices   map[string]Device
	RelayAddr string

	CTX    context.Context
	Cancel context.CancelFunc
	DLock  sync.RWMutex
}

func NewNode(
	listenHost string,
	listenPort uint64,
	bootstrapper bool,
	opts ...libp2p.Option,
) (*Node, error) {
	addr := fmt.Sprintf("/ip4/%s/tcp/%d", listenHost, listenPort)
	lopts := []libp2p.Option{
		// libp2p.DisableRelay(),
		// libp2p.NATPortMap(),

		libp2p.ListenAddrStrings(addr),
		// libp2p.EnableAutoRelayWithPeerSource(
		// 	func(ctx context.Context, num int) <-chan peer.AddrInfo {
		// 		ch := make(chan peer.AddrInfo, num)

		// 		go func() {
		// 			ctxDone := false
		// 			for i := 0; i < num; i++ {
		// 				select {
		// 				case ai := <-peerChan:
		// 					ch <- ai
		// 				case <-ctx.Done():
		// 					ctxDone = true
		// 				}
		// 				if ctxDone {
		// 					break
		// 				}
		// 			}
		// 			close(ch)
		// 		}()
		// 		return ch
		// 	},
		// ),

		libp2p.DefaultMuxers,
		libp2p.Security(noise.ID, noise.New),
	}

	relayNode := bootstrapper
	if bootstrapper {
		lopts = append(lopts, []libp2p.Option{
			libp2p.EnableHolePunching(),
		}...)
	} else {

	}

	lopts = append(lopts, opts...)

	logger.Debugf("Start listening to %s...", addr)
	host, relayErr := libp2p.New(lopts...)
	if relayErr != nil {
		return nil, relayErr
	}

	ctx, cancel := context.WithCancel(context.Background())

	n := &Node{
		Bootstrapper: bootstrapper,
		RelayNode:    relayNode,

		CTX:    ctx,
		Cancel: cancel,

		Host:    host,
		DHT:     nil,
		Devices: make(map[string]Device),
	}

	// go func() {
	// 	defer func() {
	// 		if err := recover(); err != nil {
	// 			log.Print(err)
	// 		}
	// 	}()
	// 	http.Handle("/metrics", promhttp.Handler())
	// 	http.ListenAndServe("0.0.0.0:9600", nil)
	// }()

	// peerChan := make(chan peer.AddrInfo, 100)
	// go n.Provide(ctx, peerChan)

	// n.Host.SetStreamHandler(ListProtocol, n.handleFileList)
	// n.Host.SetStreamHandler(GetProtocol, n.sendFile)

	if n.RelayNode {
		_, relayErr = relay.New(n.Host)
		if relayErr != nil {
			return nil, relayErr
		}

		relayInfo := peer.AddrInfo{
			ID:    n.Host.ID(),
			Addrs: n.Host.Addrs(),
		}
		logger.Infof("Connect to relay node: %v", relayInfo)
	}

	if n.Bootstrapper {
		if _, dhtErr := n.InitDHT([]string{}); dhtErr != nil {
			return nil, dhtErr
		}
	} else {
		anyConnected, dhtErr := n.InitDHT(bootstrap.BOOTSTRAPPERS)
		if dhtErr != nil {
			return nil, dhtErr
		}
		if !anyConnected {
			return nil, fmt.Errorf("no any bootstrap connection")
		}
	}

	logger.Infof("Node ID: %v", n.ID())

	return n, nil
}

func (n *Node) ID() peer.ID {
	return n.Host.ID()
}

func (n *Node) Close() {
	n.CTX.Done()
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

func convertPeers(peers []string) []peer.AddrInfo {
	pinfos := make([]peer.AddrInfo, len(peers))
	for i, addr := range peers {
		maddr := ma.StringCast(addr)
		p, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Fatalln(err)
		}
		pinfos[i] = *p
	}
	return pinfos
}

func (n *Node) InitDHT(bootstrapPeers []string) (bool, error) {
	anyConnected := false

	ctx := n.CTX
	cancel := n.Cancel
	host := n.Host

	var options []dht.Option

	options = append(options, dht.Mode(dht.ModeAutoServer))
	if len(bootstrapPeers) > 0 {
		// options = append(options, dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...))
		options = append(options, dht.BootstrapPeers(convertPeers(bootstrapPeers)...))
	}

	kademliaDHT, err := dht.New(ctx, host, options...)

	if err != nil {
		cancel()
		return anyConnected, err
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		cancel()
		return anyConnected, err
	}

	for _, peerAddrString := range bootstrapPeers {
		peerAddr, peerAddrErr := ma.NewMultiaddr(peerAddrString)
		if peerAddrErr != nil {
			logger.Warn(peerAddrErr)
		} else {
			peerinfo, peerErr := peer.AddrInfoFromP2pAddr(peerAddr)
			if peerErr != nil {
				logger.Warn(peerErr)
			} else {
				if err := host.Connect(ctx, *peerinfo); err != nil {
					logger.Warnf("Error while connecting to node %q: %-v", peerinfo, err)
				} else {
					anyConnected = true
					logger.Infof("Connection established with bootstrap node: %q, %v", *peerinfo, n.RelayAddr)
				}
			}
		}
	}

	n.DHT = kademliaDHT

	return anyConnected, nil
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

			for peer := range peerChan {
				id := peer.ID.String()

				// Skip existing
				if _, deviceErr := n.GetDevice(id); deviceErr == nil {
					continue
				}

				if peer.ID == n.ID() {
					sysInfo, sysInfoErr := util.GetSystemInformation()
					if sysInfoErr != nil {
						logger.Fatalln(sysInfoErr)
						return
					}
					n.DLock.Lock()
					n.Devices[id] = Device{
						ID:      peer.ID,
						Status:  "healthy",
						SysInfo: sysInfo,
					}
					n.DLock.Unlock()
				} else {
					if pingPongErr := n.Ping(peer.ID); pingPongErr != nil {
						n.Host.Network().ClosePeer(peer.ID)
						n.Host.Peerstore().RemovePeer(peer.ID)
						n.Host.Peerstore().ClearAddrs(peer.ID)
						n.DHT.RoutingTable().RemovePeer(peer.ID)
						n.DHT.RefreshRoutingTable()
						continue
					}

					logger.Debugf("Found peer: %v", peer)

					n.DLock.Lock()
					n.Devices[id] = Device{
						ID:     peer.ID,
						Status: "discovered",
					}
					n.DLock.Unlock()

					err := n.Host.Connect(ctx, peer)
					if err != nil {
						logger.Infof("Failed connecting to %s, error: %s\n", peer.ID, err)
					} else {
						logger.Infof("Connected to peer %s", peer.ID.String())
					}
				}
			}
		}
	}
}

func (n *Node) HealthcheckDevices() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-n.CTX.Done():
			return
		case <-ticker.C:
			var wg sync.WaitGroup
			for deviceId, device := range n.Devices {
				if device.ID == n.ID() {
					continue
				}

				disconnect := func() {
					logger.Debugf("disconnecting device %v", device.ID)
					n.DLock.Lock()
					delete(n.Devices, deviceId)
					n.DLock.Unlock()
					if n.Bootstrapper {
						n.Host.Peerstore().RemovePeer(device.ID)
					}
				}

				if device.Status == "discovered" {
					wg.Add(1)
					go func() {
						defer wg.Done()

						deviceInfoStr, deviceInfoErr := n.GetEcho(device.ID, constant.StellarEchoDeviceInfo)
						if deviceInfoErr != nil {
							defer disconnect()
							return
						}

						var d Device
						decodeErr := json.Unmarshal([]byte(deviceInfoStr), &d)
						if decodeErr != nil {
							defer disconnect()
							return
						}
						if d.ID != device.ID {
							logger.Errorf("fatal device handshake, device id not matched: %v, %v", device.ID, d.ID)
							defer disconnect()
							return
						}

						n.DLock.Lock()
						device.ReferenceToken = d.ReferenceToken
						device.Status = "healthy"
						device.RelayAddr = d.RelayAddr
						device.SysInfo = d.SysInfo
						n.Devices[deviceId] = device
						n.DLock.Unlock()
					}()
				}

				if device.Status == "healthy" {
					wg.Add(1)
					go func() {
						defer wg.Done()

						if pingPongErr := n.Ping(device.ID); pingPongErr != nil {
							defer disconnect()
							return
						}

						n.DLock.Lock()
						device.Timestamp = time.Now().UTC()
						n.Devices[deviceId] = device
						n.DLock.Unlock()
					}()
				}
			}
			wg.Wait()
		}
	}
}

func (n *Node) GetEcho(peer peer.ID, echoCmd string) (string, error) {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarEchoProtocol)
	if err != nil {
		logger.Debugf("[%v] dial error: %v", echoCmd, err)
		return "", err
	}
	defer s.Close()

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", echoCmd)))
	if err != nil {
		logger.Debugf("[%v] write error: %v", echoCmd, err)
		return "", err
	}

	data, err := io.ReadAll(s)
	if err != nil {
		logger.Debugf("[%v] read error: %v", echoCmd, err)
		return "", err
	}
	if string(data) == constant.StellarEchoUnknownCommand {
		logger.Debugf("[%v] read rejected: unknown command", echoCmd)
		return "", err
	}
	return string(data), err
}

func (n *Node) Ping(peer peer.ID) error {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarEchoProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPing)))
	if err != nil {
		return err
	}

	data, err := io.ReadAll(s)
	if err != nil {
		return err
	}
	if string(data) == constant.StellarEchoUnknownCommand {
		return err
	}

	if string(data) != constant.StellarPong {
		return fmt.Errorf("ping result from %v is not %v", peer, constant.StellarPong)
	}

	return nil
}

func (n *Node) GetDevice(id string) (Device, error) {
	var device Device

	n.DLock.Lock()
	device, exist := n.Devices[id]
	n.DLock.Unlock()
	if !exist {
		return device, fmt.Errorf("device not exist: %v", id)
	}

	return device, nil
}

func (n *Node) Provide(ctx context.Context, peerChan chan peer.AddrInfo) {
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
		case <-ctx.Done():
			return
		}
	}
}
