package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"stellar/core/conda"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-conda-test")

func testCommand() {
	golog.SetLogLevel("*", "info")
	golog.SetLogLevel("stellar-conda", "debug")

	defer func() {
		if r := recover(); r != nil {
			logger.Error(r)
			logger.Error("stacktrace from panic: \n" + string(debug.Stack()))
		}
	}()

	test()
}

func test() {
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
