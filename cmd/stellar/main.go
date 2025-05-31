package main

import (
	"flag"
	"fmt"
	"os"

	golog "github.com/ipfs/go-log/v2"
)

const help = `Stellar cli`

func main() {
	var logger = golog.Logger("stellar")

	golog.SetLogLevel("stellar", "info")
	golog.SetLogLevel("stellar-p2p-node", "debug")
	golog.SetLogLevel("stellar-p2p-bootstrap", "info")
	golog.SetLogLevel("stellar-p2p-protocols-proxy", "info")
	golog.SetLogLevel("stellar-p2p-protocols-echo", "info")
	golog.SetLogLevel("stellar-p2p-protocols-file", "info")
	// golog.SetAllLoggers(golog.LevelInfo)

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		fmt.Println("expected 'key' or 'node' subcommands")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "key":
		keyCommand()
	case "node":
		nodeCommand()
	case "test":
		testCommand()
	default:
		os.Exit(1)
	}
}
