package main

import (
	"flag"
	"net"
	"stellar/core/config"
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

func nodeCommand(args []string) {
	// Load config from file
	cfg, configExists, err := config.LoadConfig()
	if err != nil {
		logger.Warnf("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
		configExists = false
	}

	nodeCmd := flag.NewFlagSet("node", flag.ExitOnError)

	// Connection
	listenHost := nodeCmd.String("host", cfg.ListenHost, "set listening host")
	listenPort := nodeCmd.Int("port", cfg.ListenPort, "set listening port")

	// Bootstrapper settings
	bootstrapper := nodeCmd.Bool("bootstrapper", cfg.Bootstrapper, "run as bootstrapper node")
	relay := nodeCmd.Bool("relay", cfg.Relay, "use this node as relay node for relaying")
	disableNode := nodeCmd.Bool("disable-node", false, "disable normal node functionality (only works with --bootstrapper)")

	// Key
	seed := nodeCmd.Int64("seed", cfg.Seed, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", cfg.B64PrivKey, "import base64 encoded Ed25519 private key raw bytes")

	// Miscellaneous
	referenceToken := nodeCmd.String("reference_token", cfg.ReferenceToken, "specify custom reference token")
	metrics := nodeCmd.Bool("metrics", cfg.Metrics, "open metrics server or not")
	metricsPort := nodeCmd.Int("metrics-port", cfg.MetricsPort, "set metrics server port")
	disablePolicy := nodeCmd.Bool("disable-policy", cfg.DisablePolicy, "disable policy or not")
	noSocketServer := nodeCmd.Bool("no-socket", cfg.NoSocketServer, "open socket server or not")
	apiServer := nodeCmd.Bool("api", cfg.APIServer, "open api server or not")
	apiPort := nodeCmd.Int("api-port", cfg.APIPort, "set api server port")
	debug := nodeCmd.Bool("debug", cfg.Debug, "debug mode")

	nodeCmd.Parse(args)

	// Validate argument combinations
	if *disableNode && !*bootstrapper {
		logger.Fatalln("--disable-node can only be used with --bootstrapper. Cannot start any p2p node without bootstrapper.")
	}
	if *relay && !*bootstrapper {
		logger.Fatalln("--relay can only be used with --bootstrapper. Cannot start relay without bootstrapper.")
	}

	// Auto-generate config.json if it doesn't exist (with CLI overrides applied)
	// This allows users to edit config.json manually without it being overwritten on subsequent runs
	if !configExists {
		cfg.ListenHost = *listenHost
		cfg.ListenPort = *listenPort
		cfg.Bootstrapper = *bootstrapper
		cfg.Relay = *relay
		cfg.Seed = *seed
		// Only override B64PrivKey if explicitly provided via CLI
		if *b64privkey != "" {
			cfg.B64PrivKey = *b64privkey
		}
		// If no key was provided and default config generated one, keep it
		cfg.ReferenceToken = *referenceToken
		cfg.Metrics = *metrics
		cfg.MetricsPort = *metricsPort
		cfg.DisablePolicy = *disablePolicy
		cfg.NoSocketServer = *noSocketServer
		cfg.APIServer = *apiServer
		cfg.APIPort = *apiPort
		cfg.Debug = *debug
		if saveErr := config.SaveConfig(cfg); saveErr != nil {
			logger.Warnf("Failed to save config: %v", saveErr)
		}
	}

	// Run as node (bootstrapper or regular)
	device := device.Device{}

	// Only set key via opts if not using b64privkey directly
	// (b64privkey will be passed to InitWithOptions)
	if *b64privkey == "" {
		device.GenerateKey(*seed)
	}

	if *disablePolicy {
		logger.Warn("Device Policy disabled, it is recommended to turn it on in production environment.")
	}

	// Initialize device with bootstrapper options
	device.InitWithOptions(
		*listenHost,
		uint64(*listenPort),
		*bootstrapper,
		*relay,
		*b64privkey,
		*debug,
	)

	device.SetReferenceToken(*referenceToken)

	device.Node.Policy.Enable = !*disablePolicy

	if *metrics {
		device.Node.StartMetricsServer(uint64(*metricsPort))
	}

	// If --disable-node is set with --bootstrapper, only run as bootstrapper
	// (no discovery, no API, no socket server)
	if *disableNode && *bootstrapper {
		logger.Infof("Running in bootstrapper-only mode (normal node functionality disabled)")
	} else {
		// Normal node functionality: discovery, API, socket server
		device.StartDiscovery()

		if !*noSocketServer {
			device.StartUnixSocket()
		}

		if *apiServer {
			device.StartAPI(uint64(*apiPort))
		}
	}

	<-make(chan struct{}) // hang forever
}
