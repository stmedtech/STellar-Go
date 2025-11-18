package main

import (
	"flag"
	"fmt"
	"os"
	"stellar/core/conda"
)

func condaCommand() {
	condaCmd := flag.NewFlagSet("conda", flag.ExitOnError)
	version := condaCmd.String("version", "py313", "set conda version")

	condaCmd.Parse(os.Args[2:])

	err := conda.Install(*version)
	if err != nil {
		panic(err)
	}

	condaPath, err := conda.CommandPath()
	if err != nil {
		panic(err)
	}
	fmt.Println(condaPath)
}
