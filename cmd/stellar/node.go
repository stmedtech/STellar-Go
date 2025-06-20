package main

import (
	"flag"
	"net"
	"os"
	"stellar/core/device"
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
	nodeCmd := flag.NewFlagSet("node", flag.ExitOnError)

	// Connection
	listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := nodeCmd.Int("port", 0, "set listening port")

	// Key
	seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")

	// Miscellaneous
	referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
	metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
	disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

	nodeCmd.Parse(os.Args[2:])

	device := device.Device{}

	if *b64privkey != "" {
		device.ImportKey(*b64privkey)
	} else {
		device.GenerateKey(*seed)
	}

	if *disablePolicy {
		logger.Warn("Device Policy disabled, it is recommended to turn it on in production environment.")
	}

	device.Init(*listenHost, uint64(*listenPort))

	device.SetReferenceToken(*referenceToken)

	device.Node.Policy.Enable = !*disablePolicy

	if *metrics {
		device.Node.StartMetricsServer(5001)
	}

	device.StartDiscovery()
	device.StartUnixSocket()

	<-make(chan struct{}) // hang forever
}
