package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// RawExecutor implements the Executor interface using os/exec
type RawExecutor struct{}

// NewRawExecutor creates a new raw executor
func NewRawExecutor() *RawExecutor {
	return &RawExecutor{}
}

// ExecuteRaw executes a raw command with the given request
func (e *RawExecutor) ExecuteRaw(ctx context.Context, req RawExecutionRequest) (*RawExecution, error) {
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Create context with cancellation
	execCtx, cancel := context.WithCancel(ctx)

	// Create command
	cmd := exec.CommandContext(execCtx, req.Command, req.Args...)

	// Set environment variables
	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Set up pipes
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		stdinPipe.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		stdinPipe.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		cancel()
		stdinPipe.Close()
		stdoutPipe.Close()
		stderrPipe.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Create channels for completion and exit code
	done := make(chan error, 1)
	exitCode := make(chan int, 1)

	// Start goroutine to wait for command completion
	go func() {
		err := cmd.Wait()
		if err != nil {
			// Try to extract exit code from error
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode <- exitError.ExitCode()
			} else {
				exitCode <- -1
			}
		} else {
			exitCode <- 0
		}
		done <- err
		close(done)
		close(exitCode)
	}()

	// Connect stdin if provided
	if req.Stdin != nil {
		go func() {
			_, _ = io.Copy(stdinPipe, req.Stdin)
			stdinPipe.Close()
		}()
	} else {
		// Close stdin immediately if not provided
		// This allows commands that don't need stdin to proceed
		stdinPipe.Close()
	}

	// Create execution handle
	execution := &RawExecution{
		RunID:    generateRunID(),
		Stdin:    stdinPipe,
		Stdout:   stdoutPipe,
		Stderr:   stderrPipe,
		Done:     done,
		ExitCode: exitCode,
		Cancel: func() {
			cancel()
			// Kill the process if it's still running
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		},
	}

	return execution, nil
}

// WaitForCompletion waits for execution to complete and returns the exit code
func WaitForCompletion(execution *RawExecution, timeout time.Duration) (int, error) {
	select {
	case err := <-execution.Done:
		if err != nil {
			// Check if it's an exit error
			if exitError, ok := err.(*exec.ExitError); ok {
				return exitError.ExitCode(), nil
			}
			return -1, err
		}
		// Get exit code
		select {
		case code := <-execution.ExitCode:
			return code, nil
		case <-time.After(timeout):
			return -1, fmt.Errorf("timeout waiting for exit code")
		}
	case code := <-execution.ExitCode:
		// Exit code received before done (shouldn't happen, but handle it)
		<-execution.Done
		return code, nil
	case <-time.After(timeout):
		return -1, fmt.Errorf("timeout waiting for execution to complete")
	}
}
