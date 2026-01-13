package main

import (
	"flag"
	"stellar/core/config"
)

func configCommand(args []string) {
	configCmd := flag.NewFlagSet("config", flag.ExitOnError)
	configCmd.Parse(args)

	cfg, configExists, err := config.LoadConfig()
	if err != nil {
		logger.Errorf("Failed to load config: %v", err)
		return
	}

	if !configExists {
		logger.Infof("Config file not found, using defaults")
		cfg = config.DefaultConfig()
	}

	logger.Infof("Config: %+v", cfg)
}
