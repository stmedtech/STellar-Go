package main

import (
	"flag"
	"os"
	"stellar/core/gui"

	golog "github.com/ipfs/go-log/v2"
)

func guiCommand() {
	var logger = golog.Logger("stellar")

	guiCmd := flag.NewFlagSet("gui", flag.ExitOnError)
	bypass := guiCmd.Bool("bypass", false, "start node directly with default settings")
	guiCmd.Parse(os.Args[2:])

	app, err := gui.NewGUIApp()
	if err != nil {
		logger.Fatalln(err)
	}
	app.Bypass = *bypass

	app.Run()
}
