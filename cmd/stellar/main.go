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

	golog.SetLogLevelRegex("stellar.*", "info")

	// golog.SetAllLoggers(golog.LevelInfo)

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		fmt.Println("expected 'key', 'bootstrapper', 'node', 'gui' subcommands")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "key":
		keyCommand()
	case "bootstrapper":
		bootstrapperCommand()
	case "node":
		nodeCommand()
	case "gui":
		guiCommand()
	case "test":
		testCommand()
	default:
		os.Exit(1)
	}
}
