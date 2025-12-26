package main

import (
	"flag"
	"os"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-cli")

const help = `Stellar cli`

func main() {
	golog.SetLogLevelRegex("stellar.*", "info")

	// golog.SetAllLoggers(golog.LevelInfo)

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

	args := os.Args
	subCommand := ""
	if len(os.Args) < 2 {
		logger.Warn("expected 'key', 'bootstrapper', 'node', 'gui', 'conda' subcommands")
		// os.Exit(1)
		subCommand = "gui"
		args = os.Args[1:]
	} else {
		subCommand = args[1]
		args = args[2:]
	}

	switch subCommand {
	case "key":
		keyCommand(args)
	case "bootstrapper":
		bootstrapperCommand(args)
	case "node":
		nodeCommand(args)
	case "gui":
		guiCommand(args)
	case "conda":
		condaCommand(args)
	default:
		logger.Fatalf("unknown command: %s", subCommand)
	}
}
