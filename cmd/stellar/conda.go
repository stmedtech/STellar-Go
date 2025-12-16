package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"stellar/core/conda"
	"stellar/p2p/protocols/compute/service"
)

func condaCommand() {
	if len(os.Args) < 3 {
		printCondaUsage()
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "install-conda":
		condaInstallCondaCommand()
	case "path":
		condaPathCommand()
	case "version":
		condaVersionCommand()
	case "list":
		condaListCommand()
	case "create":
		condaCreateCommand()
	case "remove":
		condaRemoveCommand()
	case "update":
		condaUpdateCommand()
	case "install":
		condaInstallCommand()
	case "get":
		condaGetCommand()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
		printCondaUsage()
		os.Exit(1)
	}
}

func printCondaUsage() {
	fmt.Fprintf(os.Stderr, `Usage: stellar conda <subcommand> [options]

Conda Installation & Setup:
  install-conda [version] Download and install conda (default: py313)
  path                    Show conda executable path
  version                 Show conda version

Environment Management:
  list                    List all conda environments
  create <name>           Create a new conda environment
  remove <name>           Remove a conda environment
  update <name> <yaml>    Update environment from YAML file
  install <env> <pkg>     Install a package in an environment
  get <name>              Get the path of an environment

Examples:
  stellar conda install-conda py313
  stellar conda path
  stellar conda version
  stellar conda list
  stellar conda create --python 3.13 myenv
  stellar conda remove myenv
  stellar conda update myenv environment.yml
  stellar conda install myenv numpy
  stellar conda get myenv
`)
}

func getCondaManager() (*conda.CondaManager, error) {
	// Find conda path
	condaPath, err := conda.FindCondaPath()
	if err != nil {
		return nil, fmt.Errorf("conda not found: %w\nHint: Install conda or set PATH to include conda", err)
	}

	// Create RawExecutor for local operations
	executor := service.NewRawExecutor()

	// Create CondaManager
	manager := conda.NewCondaManager(executor, condaPath)

	return manager, nil
}

func streamExecution(exec *service.RawExecution, showProgress bool) error {
	if !showProgress {
		// Just wait for completion without streaming
		<-exec.Done
		exitCode := <-exec.ExitCode
		if exitCode != 0 {
			return fmt.Errorf("command exited with code %d", exitCode)
		}
		return nil
	}

	// Stream stdout and stderr in real-time
	var wg sync.WaitGroup
	var stdoutErr, stderrErr error

	// Stream stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stdoutErr = io.Copy(os.Stdout, exec.Stdout)
	}()

	// Stream stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stderrErr = io.Copy(os.Stderr, exec.Stderr)
	}()

	// Wait for completion
	doneErr := <-exec.Done
	exitCode := <-exec.ExitCode

	// Wait for streams to finish
	wg.Wait()

	// Check for errors and provide user-friendly messages
	if doneErr != nil {
		errMsg := doneErr.Error()
		// Check for common error patterns
		if strings.Contains(errMsg, "Connection broken") || strings.Contains(errMsg, "IncompleteRead") {
			return fmt.Errorf("network connection error during download. This may be due to:\n  - Unstable network connection\n  - Firewall/proxy issues\n  - Conda repository temporarily unavailable\n\nTry running the command again, or check your network connection")
		}
		if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
			return fmt.Errorf("operation timed out. This may be due to:\n  - Slow network connection\n  - Large package downloads\n\nTry increasing the timeout or check your network connection")
		}
		return fmt.Errorf("execution failed: %v", doneErr)
	}
	if stdoutErr != nil && stdoutErr != io.EOF {
		return fmt.Errorf("stdout read error: %w", stdoutErr)
	}
	if stderrErr != nil && stderrErr != io.EOF {
		return fmt.Errorf("stderr read error: %w", stderrErr)
	}
	if exitCode != 0 {
		// Provide more context for non-zero exit codes
		if exitCode == 1 {
			return fmt.Errorf("command failed with exit code %d. Check the output above for details", exitCode)
		}
		return fmt.Errorf("command exited with code %d", exitCode)
	}

	return nil
}

func condaListCommand() {
	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	envs, err := manager.ListEnvironments(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing environments: %v\n", err)
		os.Exit(1)
	}

	if len(envs) == 0 {
		fmt.Println("No conda environments found (excluding base)")
		return
	}

	fmt.Println("Conda environments:")
	for name, path := range envs {
		fmt.Printf("  %s\t%s\n", name, path)
	}
}

func condaCreateCommand() {
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	pythonVersion := createCmd.String("python", "3.13", "Python version (e.g., 3.13, 3.14)")
	quiet := createCmd.Bool("quiet", false, "Suppress progress output")
	createCmd.Parse(os.Args[3:])

	if createCmd.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: environment name required\n")
		fmt.Fprintf(os.Stderr, "Usage: stellar conda create <name> [--python <version>] [--quiet]\n")
		os.Exit(1)
	}

	envName := createCmd.Args()[0]

	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Check if environment already exists
	existingPath, err := manager.GetEnvironment(ctx, envName)
	if err == nil {
		if !*quiet {
			fmt.Printf("Environment '%s' already exists at: %s\n", envName, existingPath)
		} else {
			fmt.Println(existingPath)
		}
		return
	}

	if !*quiet {
		fmt.Printf("Creating conda environment '%s' with Python %s...\n", envName, *pythonVersion)
	}

	// For create, stream manually for real-time output
	condaPath, err := conda.FindCondaPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	executor := service.NewRawExecutor()
	exec, err := executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: condaPath,
		Args:    []string{"create", "--name", envName, fmt.Sprintf("python=%s", *pythonVersion), "-y"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := streamExecution(exec, !*quiet); err != nil {
		fmt.Fprintf(os.Stderr, "\nError creating environment: %v\n", err)
		os.Exit(1)
	}

	// Get the environment path after creation
	path, err := manager.GetEnvironment(ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Environment created but path not found: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Environment created successfully at: %s\n", path)
	} else {
		fmt.Println(path)
	}
}

func condaRemoveCommand() {
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
	quiet := removeCmd.Bool("quiet", false, "Suppress progress output")
	removeCmd.Parse(os.Args[3:])

	if removeCmd.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: environment name required\n")
		fmt.Fprintf(os.Stderr, "Usage: stellar conda remove <name> [--quiet]\n")
		os.Exit(1)
	}

	envName := removeCmd.Args()[0]

	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check if environment exists first
	_, err = manager.GetEnvironment(ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: environment '%s' not found\n", envName)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Removing conda environment '%s'...\n", envName)
	}

	// For remove, we need to stream the output manually since CondaManager doesn't expose streaming
	// We'll use the executor directly for streaming
	condaPath, err := conda.FindCondaPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	executor := service.NewRawExecutor()
	exec, err := executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: condaPath,
		Args:    []string{"env", "remove", "--name", envName, "-y"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := streamExecution(exec, !*quiet); err != nil {
		fmt.Fprintf(os.Stderr, "\nError removing environment: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Environment '%s' removed successfully\n", envName)
	}
}

func condaUpdateCommand() {
	updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
	quiet := updateCmd.Bool("quiet", false, "Suppress progress output")
	updateCmd.Parse(os.Args[3:])

	if updateCmd.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Error: environment name and YAML file required\n")
		fmt.Fprintf(os.Stderr, "Usage: stellar conda update <name> <yaml-file> [--quiet]\n")
		os.Exit(1)
	}

	envName := updateCmd.Args()[0]
	yamlPath := updateCmd.Args()[1]

	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Check if environment exists first
	_, err = manager.GetEnvironment(ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: environment '%s' not found\n", envName)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Updating conda environment '%s' from '%s'...\n", envName, yamlPath)
	}

	// For update, stream manually
	condaPath, err := conda.FindCondaPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	executor := service.NewRawExecutor()
	exec, err := executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: condaPath,
		Args:    []string{"env", "update", "--name", envName, "--file", yamlPath},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := streamExecution(exec, !*quiet); err != nil {
		fmt.Fprintf(os.Stderr, "\nError updating environment: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Environment '%s' updated successfully\n", envName)
	}
}

func condaInstallCommand() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	quiet := installCmd.Bool("quiet", false, "Suppress progress output")
	installCmd.Parse(os.Args[3:])

	if installCmd.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Error: environment name and package name required\n")
		fmt.Fprintf(os.Stderr, "Usage: stellar conda install <env> <package> [--quiet]\n")
		os.Exit(1)
	}

	envName := installCmd.Args()[0]
	packageName := installCmd.Args()[1]

	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Check if environment exists first
	_, err = manager.GetEnvironment(ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: environment '%s' not found\n", envName)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Installing package '%s' in environment '%s'...\n", packageName, envName)
	}

	// For install, stream manually
	condaPath, err := conda.FindCondaPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	executor := service.NewRawExecutor()
	exec, err := executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: condaPath,
		Args:    []string{"run", "--name", envName, "pip", "install", packageName},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := streamExecution(exec, !*quiet); err != nil {
		// Check if it's "already satisfied" which is actually success
		if strings.Contains(err.Error(), "already satisfied") {
			if !*quiet {
				fmt.Printf("Package '%s' is already installed in environment '%s'\n", packageName, envName)
			}
			return
		}
		fmt.Fprintf(os.Stderr, "\nError installing package: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Package '%s' installed successfully in environment '%s'\n", packageName, envName)
	}
}

func condaGetCommand() {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getCmd.Parse(os.Args[3:])

	if getCmd.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: environment name required\n")
		fmt.Fprintf(os.Stderr, "Usage: stellar conda get <name>\n")
		os.Exit(1)
	}

	envName := getCmd.Args()[0]

	manager, err := getCondaManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	path, err := manager.GetEnvironment(ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(path)
}

func condaInstallCondaCommand() {
	installCmd := flag.NewFlagSet("install-conda", flag.ExitOnError)
	quiet := installCmd.Bool("quiet", false, "Suppress progress output")
	installCmd.Parse(os.Args[3:])

	version := "py313"
	if installCmd.NArg() > 0 {
		version = installCmd.Args()[0]
	}

	if !*quiet {
		fmt.Printf("Installing conda (version: %s)...\n", version)
	}

	err := conda.Install(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error installing conda: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Println("Conda installed successfully")
		// Show the path
		path, pathErr := conda.CommandPath()
		if pathErr == nil {
			fmt.Printf("Conda path: %s\n", path)
		}
	}
}

func condaPathCommand() {
	pathCmd := flag.NewFlagSet("path", flag.ExitOnError)
	pathCmd.Parse(os.Args[3:])

	path, err := conda.CommandPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(path)
}

func condaVersionCommand() {
	versionCmd := flag.NewFlagSet("version", flag.ExitOnError)
	versionCmd.Parse(os.Args[3:])

	condaPath, err := conda.FindCondaPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: conda not found: %v\n", err)
		os.Exit(1)
	}

	version, err := conda.GetCondaVersion(condaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting conda version: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(version)
}
