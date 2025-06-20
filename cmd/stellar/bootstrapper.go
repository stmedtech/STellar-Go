package main

import (
	"flag"
	"os"
	"stellar/p2p/node"

	golog "github.com/ipfs/go-log/v2"
)

func bootstrapperCommand() {
	var logger = golog.Logger("stellar")

	bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ExitOnError)

	// Connection
	listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
	relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")

	// Key
	b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")

	debug := bootstrapperCmd.Bool("debug", false, "debug mode")

	bootstrapperCmd.Parse(os.Args[2:])

	n, nodeErr := node.NewBootstrapper(
		*listenHost,
		uint64(*listenPort),
		*b64privkey,
		*relay,
		*debug,
	)
	if nodeErr != nil {
		logger.Fatalln(nodeErr)
	}

	n.StartMetricsServer(5001)

	<-make(chan struct{}) // hang forever
}
