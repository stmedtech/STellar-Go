package conda

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"stellar/core/constant"

	golog "github.com/ipfs/go-log/v2"
)

var operationsLogger = golog.Logger("stellar-conda-operations")

// CondaOperations provides all conda operations using plain Go (no Executor dependency)
// This is the single source of truth for all conda domain logic
// Works locally via os/exec, can be called from CLI or server-side __conda handler
// All operations that execute commands return CommandExecution for raw streaming
type CondaOperations struct {
	condaPath string
}

// NewCondaOperations creates a new CondaOperations instance
// If condaPath is empty, it will attempt to find conda automatically
// If conda is not found, operations will still be created (useful for install-conda)
// Operations that require conda will fail with appropriate errors
func NewCondaOperations(condaPath string) (*CondaOperations, error) {
	ops := &CondaOperations{condaPath: condaPath}

	// If condaPath not provided, try to find it (but don't fail if not found)
	if condaPath == "" {
		path, err := FindCondaPath()
		if err == nil {
			ops.condaPath = path
		}
		// If conda not found, ops.condaPath remains empty
		// This allows install-conda to work without conda being present
	}

	return ops, nil
}

// getCondaPath returns the conda path, finding it if necessary
func (o *CondaOperations) getCondaPath() (string, error) {
	if o.condaPath != "" {
		return o.condaPath, nil
	}
	return FindCondaPath()
}

// ============================================================================
// Core Execution Infrastructure (Single Source of Truth)
// ============================================================================

// CommandExecution represents an active command execution with streaming I/O
// Uses the same pattern as RawExecution (proven, tested workflow)
// All conda operations that execute commands return this for raw streaming
type CommandExecution struct {
	RunID    string
	Stdin    io.WriteCloser
	Stdout   io.ReadCloser
	Stderr   io.ReadCloser
	Done     <-chan error
	ExitCode <-chan int
	Cancel   context.CancelFunc
}

// executeCommand executes a command with streaming I/O using RawExecutor's proven pattern
// This is the SINGLE SOURCE OF TRUTH for all command execution in conda operations
// Returns CommandExecution for raw streaming stdout/stderr (no transformation, no buffering)
func (o *CondaOperations) executeCommand(ctx context.Context, command string, args []string, stdin io.Reader) (*CommandExecution, error) {
	if command == "" {
		return nil, fmt.Errorf("command is empty")
	}

	// Create context with cancellation
	execCtx, cancel := context.WithCancel(ctx)

	// Create command
	cmd := exec.CommandContext(execCtx, command, args...)

	// Set up pipes
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// IMPORTANT: Use io.Pipe for stdout/stderr streaming (same as RawExecutor)
	// Using cmd.StdoutPipe/StderrPipe while also calling cmd.Wait() concurrently can lead to
	// truncated output because os/exec may close the pipe as part of Wait().
	// To guarantee correct streaming, we use io.Pipe and close the writers after Wait completes.
	// This provides RAW streaming - no transformation, no buffering, direct redirection.
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

	// Connect stdin if provided (raw redirection, no transformation)
	if stdin != nil {
		go func() {
			_, _ = io.Copy(stdinPipe, stdin)
			stdinPipe.Close()
		}()
	} else {
		// Close stdin immediately if not provided
		// This allows commands that don't need stdin to proceed
		stdinPipe.Close()
	}

	// Create execution handle (same structure as RawExecution)
	execution := &CommandExecution{
		RunID:    generateCommandRunID(),
		Stdin:    stdinPipe,
		Stdout:   stdoutR,
		Stderr:   stderrR,
		Done:     done,
		ExitCode: exitCode,
		Cancel: func() {
			// Cancel the context first (this will signal the process to stop via exec.CommandContext)
			cancel()
			// Kill the process if it's still running (force kill)
			// This ensures the process is terminated even if context cancellation doesn't work
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			// Ensure readers are unblocked by closing pipe writers
			// This is critical - if writers aren't closed, readers will hang forever
			_ = stdoutW.Close()
			_ = stderrW.Close()
		},
	}

	return execution, nil
}

// generateCommandRunID generates a unique run ID for command execution
func generateCommandRunID() string {
	return fmt.Sprintf("conda-cmd-%d", time.Now().UnixNano())
}

// ============================================================================
// Stream Bridging Helper (DRY Principle)
// ============================================================================

// StreamBridge bridges CommandExecution streams to target streams
// This is a reusable helper for all operations that need to bridge streams
// No content transformation - pure raw redirection
type StreamBridge struct {
	execution *CommandExecution
}

// NewStreamBridge creates a new stream bridge for a command execution
func NewStreamBridge(execution *CommandExecution) *StreamBridge {
	return &StreamBridge{execution: execution}
}

// BridgeTo bridges stdout/stderr to target streams
// Returns channels that close when bridging is complete
// This allows callers to wait for completion and handle errors
func (sb *StreamBridge) BridgeTo(stdout, stderr io.Writer) (stdoutDone, stderrDone <-chan error) {
	stdoutCh := make(chan error, 1)
	stderrCh := make(chan error, 1)

	// Bridge stdout (raw copy, no transformation)
	go func() {
		defer close(stdoutCh)
		_, err := io.Copy(stdout, sb.execution.Stdout)
		if err != nil && err != io.EOF {
			stdoutCh <- err
		}
	}()

	// Bridge stderr (raw copy, no transformation)
	go func() {
		defer close(stderrCh)
		_, err := io.Copy(stderr, sb.execution.Stderr)
		if err != nil && err != io.EOF {
			stderrCh <- err
		}
	}()

	return stdoutCh, stderrCh
}

// Wait waits for execution to complete and returns exit code and error
func (sb *StreamBridge) Wait() (int, error) {
	doneErr := <-sb.execution.Done
	code := <-sb.execution.ExitCode
	return code, doneErr
}

// ============================================================================
// Pre-Conda Operations (Streaming)
// ============================================================================

// Install installs conda on the local system
// Returns CommandExecution for streaming installation output
func (o *CondaOperations) Install(ctx context.Context, pythonVersion string) (*CommandExecution, error) {
	if pythonVersion == "" {
		return nil, fmt.Errorf("python version is empty")
	}

	appDir, err := constant.StellarPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get stellar path: %w", err)
	}

	// Get download URL (from infrastructure.go)
	condaDownloadUrl, err := DownloadCondaInstaller(pythonVersion, CONDA_VERSION)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Download installer (from conda.go)
	filePath, err := Download(appDir, condaDownloadUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to download installer: %w", err)
	}
	operationsLogger.Infof("Downloaded: %s to %s", condaDownloadUrl, filePath)

	// Install based on OS - return CommandExecution for streaming
	switch runtime.GOOS {
	case "darwin", "linux":
		installPath := filepath.Join(appDir, "conda")
		return o.executeCommand(ctx, "/bin/sh", []string{filePath, "-b", "-f", "-p", installPath}, nil)
	case "windows":
		// Install Miniconda in headless mode for just this user on Windows
		return o.executeCommand(ctx, filePath, []string{
			"/InstallationType=JustMe",
			"/AddToPath=0",
			"/RegisterPython=0",
			"/RegisterConda=0",
			"/S",
			"/D=" + filepath.Join(appDir, "conda"),
		}, nil)
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// CommandPath returns the conda executable path (no command execution, just lookup)
func (o *CondaOperations) CommandPath(ctx context.Context) (string, error) {
	path, err := o.getCondaPath()
	if err != nil {
		return "", err
	}

	// Verify conda works by getting version (streaming operation)
	versionExec, err := o.GetCondaVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("conda path found but version check failed: %w", err)
	}

	// Discard output, just wait for completion
	bridge := NewStreamBridge(versionExec)
	_, _ = bridge.BridgeTo(io.Discard, io.Discard)
	code, doneErr := bridge.Wait()
	if doneErr != nil || code != 0 {
		return "", fmt.Errorf("conda version check failed")
	}

	return path, nil
}

// GetCondaVersion gets the conda version
// Returns CommandExecution for streaming version output
func (o *CondaOperations) GetCondaVersion(ctx context.Context) (*CommandExecution, error) {
	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	return o.executeCommand(ctx, condaPath, []string{"--version"}, nil)
}

// ============================================================================
// Environment Management Operations (Streaming)
// ============================================================================

// ListEnvironments lists all conda environments
// Returns CommandExecution for streaming env list output
func (o *CondaOperations) ListEnvironments(ctx context.Context) (*CommandExecution, error) {
	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	return o.executeCommand(ctx, condaPath, []string{"env", "list"}, nil)
}

// GetEnvironment returns the path of a specific environment
// Uses ListEnvironments internally and parses output
func (o *CondaOperations) GetEnvironment(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("environment name is empty")
	}

	// Get streaming execution
	listExec, err := o.ListEnvironments(ctx)
	if err != nil {
		return "", err
	}

	// Parse output from stream
	var stdoutBuf strings.Builder
	bridge := NewStreamBridge(listExec)
	stdoutDone, _ := bridge.BridgeTo(&stdoutBuf, io.Discard)

	// Wait for completion with timeout to prevent hanging
	done := make(chan struct{})
	var code int
	var doneErr error
	go func() {
		code, doneErr = bridge.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Wait for stdout bridging to complete
		<-stdoutDone
	case <-ctx.Done():
		return "", fmt.Errorf("context cancelled while getting environment: %w", ctx.Err())
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("timeout waiting for conda env list to complete")
	}

	if doneErr != nil {
		return "", fmt.Errorf("failed to execute conda env list: %w", doneErr)
	}
	if code != 0 {
		return "", fmt.Errorf("conda env list exited with code %d", code)
	}

	// Parse output directly (includes base environment)
	output := stdoutBuf.String()
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		envName := strings.TrimPrefix(parts[0], "*")
		envPath := parts[len(parts)-1]
		if envName == name {
			return envPath, nil
		}
	}

	return "", fmt.Errorf("environment not found: %s", name)
}

// CreateEnvironment creates a new conda environment
// Returns CommandExecution for streaming creation output
func (o *CondaOperations) CreateEnvironment(ctx context.Context, name, pythonVersion string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if pythonVersion == "" {
		return nil, fmt.Errorf("python version is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Check if environment already exists (non-blocking check)
	if envPath, err := o.GetEnvironment(ctx, name); err == nil {
		// Environment already exists, return a no-op execution that immediately completes
		// This maintains the CommandExecution interface contract for streaming
		// Note: Channels are kept open (not closed) so multiple readers can get the values
		// This prevents race conditions where multiple readers try to read from the same channels
		done := make(chan error, 1)
		exitCode := make(chan int, 1)
		done <- nil
		exitCode <- 0
		// Don't close channels - keep them open so multiple readers can get the values
		return &CommandExecution{
			RunID:    generateCommandRunID(),
			Stdin:    nil,
			Stdout:   io.NopCloser(strings.NewReader(fmt.Sprintf("Environment %s already exists at %s\n", name, envPath))),
			Stderr:   io.NopCloser(strings.NewReader("")),
			Done:     done,
			ExitCode: exitCode,
			Cancel:   func() {},
		}, nil
	}

	// Create environment - return streaming execution
	return o.executeCommand(ctx, condaPath, []string{"create", "--name", name, fmt.Sprintf("python=%s", pythonVersion), "-y"}, nil)
}

// RemoveEnvironment removes a conda environment
// Returns CommandExecution for streaming removal output
func (o *CondaOperations) RemoveEnvironment(ctx context.Context, name string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Check if environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, name); err != nil {
		return nil, fmt.Errorf("environment not found: %s", name)
	}

	// Remove environment - return streaming execution
	return o.executeCommand(ctx, condaPath, []string{"env", "remove", "--name", name, "-y"}, nil)
}

// UpdateEnvironment updates a conda environment from a YAML file
// Returns CommandExecution for streaming update output
func (o *CondaOperations) UpdateEnvironment(ctx context.Context, name, yamlPath string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if yamlPath == "" {
		return nil, fmt.Errorf("yaml path is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Check if environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, name); err != nil {
		return nil, fmt.Errorf("environment not found: %s", name)
	}

	// Check if YAML file exists
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("yaml file not found: %s", yamlPath)
	}

	// Update environment - return streaming execution
	return o.executeCommand(ctx, condaPath, []string{"env", "update", "--name", name, "--file", yamlPath}, nil)
}

// InstallPackage installs a package in a conda environment
// Returns CommandExecution for streaming installation output
func (o *CondaOperations) InstallPackage(ctx context.Context, envName, packageName string) (*CommandExecution, error) {
	if envName == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if packageName == "" {
		return nil, fmt.Errorf("package name is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Verify environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, envName); err != nil {
		return nil, fmt.Errorf("environment not found: %s", envName)
	}

	// Install package - return streaming execution
	return o.executeCommand(ctx, condaPath, []string{"install", "--name", envName, packageName, "-y"}, nil)
}

// ============================================================================
// Python Execution Operations (Streaming)
// ============================================================================

// RunPython executes Python code in a conda environment with streaming I/O
// Uses executeCommand (single source of truth) for streaming
// Returns CommandExecution for streaming stdout/stderr
func (o *CondaOperations) RunPython(ctx context.Context, env, code string, stdin io.Reader) (*CommandExecution, error) {
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if code == "" {
		return nil, fmt.Errorf("code is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Verify environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("environment not found: %s", env)
	}

	// Build command: conda run -n <env> --no-capture-output python -c <code>
	// --no-capture-output ensures stdin/stdout/stderr are not captured by conda
	// Use executeCommand (proven RawExecutor pattern)
	return o.executeCommand(ctx, condaPath, []string{"run", "-n", env, "--no-capture-output", "python", "-c", code}, stdin)
}

// RunScript executes a Python script in a conda environment with streaming I/O
// Uses executeCommand (single source of truth) for streaming
// Returns CommandExecution for streaming stdout/stderr
func (o *CondaOperations) RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (*CommandExecution, error) {
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if scriptPath == "" {
		return nil, fmt.Errorf("script path is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Verify environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("environment not found: %s", env)
	}

	// Build command: conda run -n <env> --no-capture-output python <script> [args...]
	// --no-capture-output ensures stdin/stdout/stderr are not captured by conda
	// Use executeCommand (proven RawExecutor pattern)
	scriptArgs := append([]string{scriptPath}, args...)
	return o.executeCommand(ctx, condaPath, append([]string{"run", "-n", env, "--no-capture-output", "python"}, scriptArgs...), stdin)
}

// RunConda executes a raw conda command with arbitrary arguments
// This allows users to execute any conda command: conda <args...>
// Returns CommandExecution for streaming stdout/stderr
func (o *CondaOperations) RunConda(ctx context.Context, args []string, stdin io.Reader) (*CommandExecution, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("conda command requires at least one argument")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Execute raw conda command: conda <args...>
	return o.executeCommand(ctx, condaPath, args, stdin)
}

// ============================================================================
// Helper Functions
// ============================================================================

// GetStdout returns stdout (for unified interface with RawExecution)
// Exported so it can satisfy ExecutionStreamReader interface from service package
func (e *CommandExecution) GetStdout() io.ReadCloser { return e.Stdout }

// GetStderr returns stderr (for unified interface with RawExecution)
// Exported so it can satisfy ExecutionStreamReader interface from service package
func (e *CommandExecution) GetStderr() io.ReadCloser { return e.Stderr }

// GetRunID returns run ID (for server-side conversion)
func (e *CommandExecution) GetRunID() string { return e.RunID }

// GetStdin returns stdin (for server-side conversion)
func (e *CommandExecution) GetStdin() io.WriteCloser { return e.Stdin }

// GetDone returns done channel (for server-side conversion)
// Exported so it can satisfy ExecutionStreamReader interface from service package
func (e *CommandExecution) GetDone() <-chan error { return e.Done }

// GetExitCode returns exit code channel (for server-side conversion)
// Exported so it can satisfy ExecutionStreamReader interface from service package
func (e *CommandExecution) GetExitCode() <-chan int { return e.ExitCode }

// GetCancel returns cancel function (for server-side conversion)
func (e *CommandExecution) GetCancel() context.CancelFunc { return e.Cancel }

// ParseEnvListOutput parses conda env list output and returns a map of env names to paths
// Includes all environments including base
// This function handles the output format from "conda env list" command
func ParseEnvListOutput(output string) (map[string]string, error) {
	envs := make(map[string]string)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line format: "env_name    /path/to/env" or "*env_name    /path/to/env"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		envName := strings.TrimPrefix(parts[0], "*")
		envPath := parts[len(parts)-1]

		// Validate that envPath looks like a valid path (starts with / or contains path separators)
		// This prevents parsing invalid output like "invalid output format" as an environment
		if !strings.HasPrefix(envPath, "/") && !strings.Contains(envPath, string(filepath.Separator)) {
			continue
		}

		envs[envName] = envPath
	}

	return envs, nil
}
