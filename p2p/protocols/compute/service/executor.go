package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	// IMPORTANT:
	// Using cmd.StdoutPipe/StderrPipe while also calling cmd.Wait() concurrently can lead to
	// truncated output because os/exec may close the pipe as part of Wait().
	// To guarantee correct streaming, we use io.Pipe and close the writers after Wait completes.
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	// Start command
	if err := cmd.Start(); err != nil {
		cancel()
		stdinPipe.Close()
		_ = stdoutW.Close()
		_ = stdoutR.Close()
		_ = stderrW.Close()
		_ = stderrR.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Create channels for completion and exit code
	done := make(chan error, 1)
	exitCode := make(chan int, 1)

	// Start goroutine to wait for command completion
	go func() {
		err := cmd.Wait()
		// Ensure stdout/stderr pipes are closed to unblock readers
		_ = stdoutW.Close()
		_ = stderrW.Close()

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
		Stdout:   stdoutR,
		Stderr:   stderrR,
		Done:     done,
		ExitCode: exitCode,
		Cancel: func() {
			cancel()
			// Kill the process if it's still running
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			// Ensure readers are unblocked
			_ = stdoutW.Close()
			_ = stderrW.Close()
		},
	}

	return execution, nil
}

// NOTE: We intentionally do not provide a time-based helper like WaitForCompletion here.
// Tests should be deterministic and rely on `go test -timeout ...` as the safety net.
