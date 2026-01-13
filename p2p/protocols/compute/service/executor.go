package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Set environment variables FIRST, before creating command
	// This ensures PATH is available when exec.CommandContext looks for executables
	env := os.Environ()

	// Apply user-provided environment variables (including PATH from server)
	// The server provides stellar executable path + user's PATH in req.Env["PATH"]
	env = applyUserEnvironment(env, req.Env)

	// Find the executable using the custom PATH
	// This ensures we can find stellar even if it's not in system PATH
	commandPath := resolveExecutablePath(req.Command, env)

	// Create command AFTER setting environment and finding executable
	cmd := exec.CommandContext(execCtx, commandPath, req.Args...)

	// Set the prepared environment
	cmd.Env = env

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

// applyUserEnvironment applies user-provided environment variables to the base environment
// Special handling for PATH: merges with system PATH instead of replacing it
func applyUserEnvironment(baseEnv []string, userEnv map[string]string) []string {
	if len(userEnv) == 0 {
		return baseEnv
	}

	env := baseEnv
	pathVar := pathVarName()

	for k, v := range userEnv {
		// Special handling for PATH: merge with system PATH
		if k == "PATH" || k == pathVar {
			systemPath := getPathFromEnv(env)
			mergedPath := mergePaths([]string{v}, systemPath)
			env = setPathInEnv(env, mergedPath)
		} else {
			// Remove existing env var if present, then add new one
			env = removeEnvVar(env, k)
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return env
}

// resolveExecutablePath resolves a command to its full executable path
// If command is already an absolute/relative path, returns it as-is
// Otherwise, searches in PATH from the environment
func resolveExecutablePath(command string, env []string) string {
	// If command is already a path, use it as-is
	if filepath.IsAbs(command) || strings.Contains(command, string(filepath.Separator)) {
		return command
	}

	// Get PATH from environment and search for executable
	pathValue := getPathFromEnv(env)
	if found := findExecutableInPath(command, pathValue); found != "" {
		return found
	}

	// Fall back to system LookPath
	if found, err := exec.LookPath(command); err == nil {
		return found
	}

	// If still not found, return command as-is and let exec handle the error
	return command
}

// removeEnvVar removes an environment variable from the env slice
func removeEnvVar(env []string, key string) []string {
	result := make([]string, 0, len(env))
	for _, envVar := range env {
		if !strings.HasPrefix(envVar, key+"=") {
			result = append(result, envVar)
		}
	}
	return result
}

// NOTE: We intentionally do not provide a time-based helper like WaitForCompletion here.
// Tests should be deterministic and rely on `go test -timeout ...` as the safety net.
