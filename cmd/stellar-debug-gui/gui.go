package main

import (
	"flag"
	"os"
	"stellar/core/gui"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-cli")

const help = `Stellar debug gui`

func guiCommand(args []string) {
	guiCmd := flag.NewFlagSet("gui", flag.ExitOnError)
	bypass := guiCmd.Bool("bypass", false, "start node directly with default settings")
	guiCmd.Parse(args)

	app, err := gui.NewGUIApp()
	if err != nil {
		logger.Fatalln(err)
	}
	app.Bypass = *bypass

	app.Run()
}

func main() {
	golog.SetLogLevelRegex("stellar.*", "info")

	// golog.SetAllLoggers(golog.LevelInfo)

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

	guiCommand(os.Args[1:])
}
