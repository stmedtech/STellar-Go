package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"stellar/p2p/protocols/compute/service/conda"
	"stellar/p2p/protocols/compute/streams"
)

func condaCommand() {
	if len(os.Args) < 3 {
		// Create handler to get usage (handler is single source of truth)
		ops, _ := conda.NewCondaOperations("")
		handler := conda.NewCondaHandler(ops)
		fmt.Fprint(os.Stderr, handler.PrintUsage())
		os.Exit(1)
	}

	subcommand := os.Args[2]
	args := os.Args[3:]

	// Use handler (single source of truth) for all commands
	ctx := context.Background()

	// Get conda operations (allow install-conda to work without conda)
	ops, err := conda.NewCondaOperations("")
	if err != nil && subcommand != "install-conda" {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Hint: Install conda or set PATH to include conda\n")
		os.Exit(1)
	}

	// For install-conda, create operations without conda path
	if err != nil && subcommand == "install-conda" {
		ops, err = conda.NewCondaOperations("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Create handler (single source of truth for command handling)
	handler := conda.NewCondaHandler(ops)

	// Parse flags for commands that need them
	var showProgress bool = true
	var stdin io.Reader = nil

	switch subcommand {
	case "create", "remove", "update", "install", "install-conda":
		// These commands support --quiet flag
		showProgress = !hasFlag(args, "quiet")
		args = removeFlag(args, "quiet")
	}

	// Handle subcommand using handler (single source of truth)
	execution, err := handler.HandleSubcommand(ctx, subcommand, args, stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if subcommand == "install-conda" {
			fmt.Fprintf(os.Stderr, "Hint: This command installs conda, so it should work even if conda is not found\n")
		}
		os.Exit(1)
	}

	// Stream output
	if err := streamCommandExecution(execution, showProgress); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// hasFlag checks if a flag exists in args
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == "--"+flag || arg == "-"+flag {
			return true
		}
	}
	return false
}

// removeFlag removes a flag and its value from args
func removeFlag(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--"+flag || args[i] == "-"+flag {
			// Skip flag (and its value if present)
			continue
		}
		result = append(result, args[i])
	}
	return result
}

// streamCommandExecution streams an ExecutionStreamReader to stdout/stderr
func streamCommandExecution(exec streams.ExecutionStreamReader, showProgress bool) error {
	if !showProgress {
		// Just wait for completion without streaming
		<-exec.GetDone()
		exitCode := <-exec.GetExitCode()
		if exitCode != 0 {
			return fmt.Errorf("command exited with code %d", exitCode)
		}
		return nil
	}

	// Stream stdout and stderr in real-time (raw redirection, no transformation)
	var wg sync.WaitGroup
	var stdoutErr, stderrErr error

	// Stream stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stdoutErr = io.Copy(os.Stdout, exec.GetStdout())
	}()

	// Stream stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stderrErr = io.Copy(os.Stderr, exec.GetStderr())
	}()

	// Wait for completion
	doneErr := <-exec.GetDone()
	exitCode := <-exec.GetExitCode()

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
