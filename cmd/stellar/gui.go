package main

import (
	"flag"
	"stellar/core/gui"
)

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
