package main

import (
	"bufio"
	"flag"
	"net"
	"os"
	"stellar/p2p/node"
	"stellar/p2p/protocols/echo"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"
	"strconv"
	"strings"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
)

func GetFreePort() (port uint64, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "0.0.0.0:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return uint64(l.Addr().(*net.TCPAddr).Port), nil
		}
	}
	return
}

func nodeCommand() {
	var logger = golog.Logger("stellar")

	nodeCmd := flag.NewFlagSet("node", flag.ExitOnError)

	// Connection
	listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := nodeCmd.Int("port", 0, "set listening port")
	bootstrapper := nodeCmd.Bool("bootstrapper", false, "use this node as bootstrapper node for bootstrapping")

	// Key
	seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")

	// Miscellaneous
	referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")

	nodeCmd.Parse(os.Args[2:])

	var n *node.Node
	var opts []libp2p.Option = make([]libp2p.Option, 0)

	if *b64privkey != "" {
		opt, privkeyErr := node.LoadPrivateKey(*b64privkey)
		if privkeyErr != nil {
			logger.Fatalln(privkeyErr)
		}
		opts = append(opts, opt)
	} else {
		opt, privkeyErr := node.GeneratePrivateKey(*seed)
		if privkeyErr != nil {
			logger.Fatalln(privkeyErr)
		}
		opts = append(opts, opt)
	}

	n, nodeErr := node.NewNode(
		*listenHost,
		uint64(*listenPort),
		*bootstrapper,
		opts...,
	)
	if nodeErr != nil {
		logger.Fatalln(nodeErr)
	}
	n.ReferenceToken = *referenceToken
	echo.BindEchoStream(n)
	file.BindFileStream(n)

	pManager := proxy.NewProxyManager(n)

	go n.DiscoverDevices()
	go n.HealthcheckDevices()

	c := make(chan os.Signal)
	select {
	case sign := <-c:
		logger.Infof("Got %s signal. Aborting...", sign)
		n.Close()
	default:
		r := bufio.NewScanner(os.Stdin)
		for r.Scan() {
			line := r.Text()
			words := strings.Split(line, " ")
			logger.Warnf("cmd: %v", words)
			switch strings.ToLower(words[0]) {
			case "tree":
				if len(words) != 2 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				id := words[1]
				device, deviceErr := n.GetDevice(id)
				if deviceErr != nil {
					logger.Warn(deviceErr)
					continue
				}

				go func() {
					files, lsErr := file.ListFullTree(n, device.ID)
					if lsErr != nil {
						logger.Warn(lsErr)
						return
					}

					var logRecur func(depth int, fs []file.FileEntry)
					logRecur = func(depth int, fs []file.FileEntry) {
						for _, f := range fs {
							ending := ""
							if f.IsDir {
								ending = "/"
							}
							logger.Infof("%v%v%v", strings.Repeat("-", depth*2)+" ", f.Filename, ending)
							if len(f.Children) > 0 {
								logRecur(depth+1, f.Children)
							}
						}
					}
					logger.Infof("Device %v ls full tree:", device.ID)
					logRecur(0, files)
				}()
			case "ls":
				if len(words) != 3 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				id := words[1]
				device, deviceErr := n.GetDevice(id)
				if deviceErr != nil {
					logger.Warn(deviceErr)
					continue
				}

				relativePath := words[2]

				go func() {
					files, lsErr := file.List(n, device.ID, relativePath)
					if lsErr != nil {
						logger.Warn(lsErr)
					}
					logger.Infof("Device %v ls: %v", device.ID, files)
				}()
			case "gf":
				if len(words) != 3 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				id := words[1]
				device, deviceErr := n.GetDevice(id)
				if deviceErr != nil {
					logger.Warn(deviceErr)
					continue
				}
				fileName := words[2]

				go func() {
					if filePath, err := file.Download(n, device.ID, fileName); err != nil {
						logger.Warn(err)
					} else {
						logger.Infof("file downloaded at %v", filePath)
					}
				}()
			case "ps":
				logger.Infof("Number of proxies: %v", len(pManager.Proxies()))
			case "p":
				if len(words) != 4 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				id := words[1]
				device, deviceErr := n.GetDevice(id)
				if deviceErr != nil {
					logger.Warn(deviceErr)
					continue
				}

				port, portErr := strconv.ParseUint(words[3], 10, 64)
				if portErr != nil {
					logger.Warnf("port err: %v", portErr)
					continue
				}

				destAddr := words[2]
				pManager.Proxy(device.ID, port, destAddr)
			case "sp":
				if len(words) != 2 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				port, portErr := strconv.ParseUint(words[1], 10, 64)
				if portErr != nil {
					logger.Warnf("port err: %v", portErr)
					continue
				}

				pManager.Close(port)
			case "get":
				if len(words) != 2 {
					logger.Warnf("args wrong: %v", words)
					continue
				}

				id := words[1]
				device, deviceErr := n.GetDevice(id)
				if deviceErr != nil {
					logger.Warn(deviceErr)
					continue
				}

				logger.Infof("Device: %v", device)
			case "addrs":
				for _, addr := range n.Host.Addrs() {
					logger.Info(addr.Encapsulate(ma.StringCast("/p2p/" + n.ID().String())))
				}
			case "np":
				logger.Infof("Number of peers: %v", len(n.Host.Network().Peers()))
			case "ds":
				logger.Infof("Total %v of devices", len(n.Devices))
				for _, device := range n.Devices {
					logger.Infof("Device: %v", device)
				}
			}
		}
	}
	<-make(chan struct{}) // hang forever
}
