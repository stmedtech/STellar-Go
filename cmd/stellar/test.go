package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"stellar/core/conda"
	"stellar/core/device"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
)

var logger = golog.Logger("stellar-conda-test")

var dev device.Device

func testCommand() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(r)
			logger.Error("stacktrace from panic: \n" + string(debug.Stack()))
		}
	}()

	testCmd := flag.NewFlagSet("test", flag.ExitOnError)
	b64privkey := testCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	deviceAddress := testCmd.String("deviceAddr", "", "connect to another device with address")

	testCmd.Parse(os.Args[2:])

	var p *peer.AddrInfo
	if *deviceAddress != "" {
		peer, addrErr := peer.AddrInfoFromString(*deviceAddress)
		if addrErr != nil {
			logger.Fatalln(addrErr)
		}
		p = peer
	}

	dev.ImportKey(*b64privkey)
	dev.Init("0.0.0.0", 0)
	dev.StartDiscovery()
	dev.Node.Policy.Enable = false

	if *deviceAddress != "" && p != nil {
		device, connectErr := dev.Node.ConnectDevice(*p)
		if connectErr != nil {
			logger.Fatalln(connectErr)
		}

		logger.Infof("Connected to Device %s", device.ID.String())
	}

	test()

	<-make(chan struct{}) // hang forever
}

func test() {

}

func testConda() {
	condaPath, err := conda.CommandPath()
	if err != nil {
		err := conda.Install("py313")
		if err != nil {
			panic(err)
		} else {
			condaPath, err = conda.CommandPath()
			if err != nil {
				panic(err)
			}
		}
	}
	logger.Infoln(condaPath)

	env := "stellar"

	if envPath, envErr := conda.CreateEnv(condaPath, env, "3.11"); envErr != nil {
		panic(envErr)
	} else {
		logger.Infoln(envPath)
	}

	envs, err := conda.EnvList(condaPath)
	if err != nil {
		panic(err)
	}
	for k, v := range envs {
		msg := fmt.Sprintf("Environment: %v, Path: %v", k, v)
		logger.Infoln(msg)
	}

	// err = RemoveCondaEnv(condaPath, "ttt")
	// if err != nil {
	// 	panic(err)
	// }

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// err = conda.UpdateEnv(condaPath, env, filepath.Join(pwd, "environment.yaml"))
	// if err != nil {
	// 	panic(err)
	// }

	// if installEror := conda.EnvInstallPackage(condaPath, env, "pip"); installEror != nil {
	// 	panic(installEror)
	// }

	if result, runErr := conda.RunCommand(condaPath, env, "python", filepath.Join(pwd, "test.py")); runErr != nil {
		panic(runErr)
	} else {
		logger.Infoln(result)
	}
}
