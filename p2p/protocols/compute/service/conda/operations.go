package conda

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	assets "stellar"
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

// InstallClient installs the bundled stellar-client Python package into a conda environment
// Extracts the embedded stellar-client directory and installs it using pip
// Returns CommandExecution for streaming installation output
// The installation is permanent (not editable) and temp files are cleaned up after completion
func (o *CondaOperations) InstallClient(ctx context.Context, envName string) (*CommandExecution, error) {
	if envName == "" {
		return nil, fmt.Errorf("environment name is empty")
	}

	condaPath, err := o.getCondaPath()
	if err != nil {
		return nil, err
	}

	// Verify environment exists (non-blocking check)
	if _, err := o.GetEnvironment(ctx, envName); err != nil {
		return nil, fmt.Errorf("environment not found: %s", envName)
	}

	// Get stellar path for temporary extraction
	appDir, err := constant.StellarPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get stellar path: %w", err)
	}

	// Create temporary directory for extraction
	tempDir := filepath.Join(appDir, "temp", fmt.Sprintf("stellar-client-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Extract embedded stellar-client directory
	// The embedded path is "stellar-client", so we extract it to tempDir
	// This will create tempDir/stellar-client/ with all the files
	clientDir := filepath.Join(tempDir, "stellar-client")
	if err := extractEmbeddedClient(clientDir); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to extract stellar-client: %w", err)
	}

	// Verify pyproject.toml exists before installation
	pyprojectPath := filepath.Join(clientDir, "pyproject.toml")
	if _, err := os.Stat(pyprojectPath); os.IsNotExist(err) {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("pyproject.toml not found in extracted package at %s", pyprojectPath)
	}

	// Verify critical __init__.py files exist before building
	initPyPath := filepath.Join(clientDir, "stellar_client", "__init__.py")
	if _, err := os.Stat(initPyPath); os.IsNotExist(err) {
		operationsLogger.Errorf("CRITICAL: __init__.py not found at %s", initPyPath)
		// List what's actually in the stellar_client directory
		stellarClientDir := filepath.Join(clientDir, "stellar_client")
		if entries, listErr := os.ReadDir(stellarClientDir); listErr == nil {
			operationsLogger.Errorf("Contents of %s:", stellarClientDir)
			for _, entry := range entries {
				operationsLogger.Errorf("  - %s (dir: %v)", entry.Name(), entry.IsDir())
			}
		}
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("__init__.py not found in extracted package at %s - package will not work", initPyPath)
	}
	operationsLogger.Infof("Verified __init__.py exists at %s", initPyPath)

	// Install using pip in the conda environment
	// Use conda run to execute pip install in the environment
	// Install without -e flag for permanent installation (not editable)
	// Use python -m pip to ensure we use the correct Python and pip from the conda environment
	// The clientDir already contains the stellar-client directory structure with pyproject.toml
	// Build wheel first, then install it to ensure proper package structure
	operationsLogger.Infof("Building and installing stellar-client from %s into conda environment %s", clientDir, envName)

	// First, build the wheel to ensure proper package structure
	buildExec, err := o.executeCommand(ctx, condaPath, []string{
		"run", "-n", envName, "--no-capture-output",
		"python", "-m", "pip", "install", "--quiet", "build",
	}, nil)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to install build tool: %w", err)
	}

	// Wait for build tool installation
	bridge := NewStreamBridge(buildExec)
	_, _ = bridge.BridgeTo(io.Discard, io.Discard)
	_, buildErr := bridge.Wait()
	if buildErr != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("build tool installation failed: %w", buildErr)
	}

	// Build the wheel
	// Use --wheel flag and ensure all files are included
	wheelExec, err := o.executeCommand(ctx, condaPath, []string{
		"run", "-n", envName, "--no-capture-output",
		"python", "-m", "build", "--wheel", "--outdir", tempDir, clientDir,
	}, nil)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start wheel build: %w", err)
	}

	// Wait for wheel build to complete
	wheelBridge := NewStreamBridge(wheelExec)
	_, _ = wheelBridge.BridgeTo(io.Discard, io.Discard)
	_, wheelErr := wheelBridge.Wait()
	if wheelErr != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("wheel build failed: %w", wheelErr)
	}

	// Find the built wheel
	wheelFiles, err := filepath.Glob(filepath.Join(tempDir, "*.whl"))
	if err != nil || len(wheelFiles) == 0 {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("wheel file not found after build")
	}
	wheelFile := wheelFiles[0]
	operationsLogger.Infof("Built wheel: %s", wheelFile)

	// Install the wheel (this ensures proper package structure)
	// Use --force-reinstall to ensure we install the newly built wheel
	exec, err := o.executeCommand(ctx, condaPath, []string{
		"run", "-n", envName, "--no-capture-output",
		"python", "-m", "pip", "install", "--force-reinstall", wheelFile,
	}, nil)
	if err != nil {
		// Clean up on error
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start pip install: %w", err)
	}

	// Wrap the execution to clean up temp directory after completion and verify installation
	return wrapWithCleanupAndVerify(exec, tempDir, condaPath, envName), nil
}

// wrapWithCleanupAndVerify wraps a CommandExecution to clean up a directory after completion and verify installation
func wrapWithCleanupAndVerify(execution *CommandExecution, cleanupDir string, condaPath string, envName string) *CommandExecution {
	// Create new channels that will be closed after cleanup
	done := make(chan error, 1)
	exitCode := make(chan int, 1)

	// Monitor the original execution and clean up when done
	go func() {
		// Wait for original execution to complete
		originalErr := <-execution.Done
		originalCode := <-execution.ExitCode

		// If installation succeeded, verify it's actually installed
		if originalErr == nil && originalCode == 0 {
			// Verify installation by trying to import the package
			verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer verifyCancel()

			// Write verification script to a temporary file to avoid conda's newline limitation
			verifyScriptFile := filepath.Join(cleanupDir, "verify_install.py")
			verifyScript := `import sys
import os
import importlib.util

# First, check if package can be imported
try:
    import stellar_client
    print("✓ import stellar_client succeeded")
    if hasattr(stellar_client, '__file__') and stellar_client.__file__:
        print(f"  Package location: {stellar_client.__file__}")
        print(f"  Package path: {stellar_client.__path__}")
    else:
        print(f"  WARNING: Package is namespace package (no __file__)")
        print(f"  Package path: {stellar_client.__path__}")
except Exception as e:
    print(f"✗ import stellar_client failed: {e}")
    import traceback
    traceback.print_exc()
    sys.exit(1)

# Check if __init__.py exists and can be read
try:
    if hasattr(stellar_client, '__file__') and stellar_client.__file__:
        init_path = os.path.join(os.path.dirname(stellar_client.__file__), "__init__.py")
        if os.path.exists(init_path):
            print(f"✓ __init__.py found at: {init_path}")
            with open(init_path, 'r') as f:
                content = f.read()
                if 'from_env' in content:
                    print("✓ __init__.py contains 'from_env'")
                else:
                    print("✗ __init__.py does NOT contain 'from_env'")
                    print(f"  First 500 chars: {content[:500]}")
        else:
            print(f"✗ __init__.py NOT found at: {init_path}")
            print(f"  Directory contents: {os.listdir(os.path.dirname(stellar_client.__file__))}")
    else:
        # Namespace package - check all paths
        print("Checking namespace package paths for __init__.py...")
        found_init = False
        for pkg_path in stellar_client.__path__:
            init_path = os.path.join(pkg_path, "__init__.py")
            if os.path.exists(init_path):
                print(f"✓ __init__.py found at: {init_path}")
                found_init = True
                with open(init_path, 'r') as f:
                    content = f.read()
                    if 'from_env' in content:
                        print("✓ __init__.py contains 'from_env'")
                    else:
                        print("✗ __init__.py does NOT contain 'from_env'")
                        print(f"  First 500 chars: {content[:500]}")
                break
            else:
                print(f"  Checking: {pkg_path}")
                if os.path.exists(pkg_path):
                    print(f"    Directory contents: {os.listdir(pkg_path)}")
        if not found_init:
            print("✗ __init__.py NOT found in any package path")
except Exception as e:
    print(f"✗ Error checking __init__.py: {e}")
    import traceback
    traceback.print_exc()

# Now try the actual import that users need
try:
    from stellar_client import from_env, StellarClient
    print("✓ from stellar_client import from_env, StellarClient succeeded")
    print("✓ stellar_client imports verified successfully")
except Exception as e:
    print(f"✗ from stellar_client import from_env, StellarClient failed: {e}")
    print(f"  Available attributes in stellar_client: {[x for x in dir(stellar_client) if not x.startswith('_')]}")
    import traceback
    traceback.print_exc()
    sys.exit(1)
`
			// Write script to file
			if err := os.WriteFile(verifyScriptFile, []byte(verifyScript), 0644); err != nil {
				operationsLogger.Errorf("Failed to write verification script: %v", err)
				originalErr = fmt.Errorf("failed to write verification script: %w", err)
				originalCode = 1
			} else {
				// Execute the script file
				verifyCmd := exec.CommandContext(verifyCtx, condaPath, "run", "-n", envName, "--no-capture-output",
					"python", verifyScriptFile)
				verifyOutput, verifyErr := verifyCmd.CombinedOutput()
				if verifyErr != nil {
					operationsLogger.Errorf("Installation completed but verification FAILED: %v\nOutput: %s", verifyErr, string(verifyOutput))
					// FAIL the installation if verification fails - this is critical
					originalErr = fmt.Errorf("package installed but imports failed - installation is NOT usable: %v\nVerification output: %s", verifyErr, string(verifyOutput))
					originalCode = 1
				} else {
					operationsLogger.Infof("stellar-client successfully installed and verified in environment %s", envName)
				}
			}
		} else {
			operationsLogger.Warnf("Installation failed with error: %v (exit code: %d)", originalErr, originalCode)
		}

		// Clean up temp directory
		if err := os.RemoveAll(cleanupDir); err != nil {
			operationsLogger.Warnf("Failed to clean up temp directory %s: %v", cleanupDir, err)
		} else {
			operationsLogger.Infof("Cleaned up temp directory: %s", cleanupDir)
		}

		// Forward the results
		done <- originalErr
		exitCode <- originalCode
		close(done)
		close(exitCode)
	}()

	// Return a new CommandExecution that wraps the original
	return &CommandExecution{
		RunID:    execution.RunID,
		Stdin:    execution.Stdin,
		Stdout:   execution.Stdout,
		Stderr:   execution.Stderr,
		Done:     done,
		ExitCode: exitCode,
		Cancel: func() {
			// Cancel original execution
			if execution.Cancel != nil {
				execution.Cancel()
			}
			// Clean up temp directory on cancel
			os.RemoveAll(cleanupDir)
		},
	}
}

// extractEmbeddedClient extracts the embedded stellar-client directory to the target path
func extractEmbeddedClient(targetDir string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Track extracted files for debugging
	var extractedFiles []string
	var missingInitFiles []string

	// Walk the embedded stellar-client directory
	// First, try to directly read __init__.py files to verify they're embedded
	initFiles := []string{
		"stellar-client/stellar_client/__init__.py",
		"stellar-client/stellar_client/models/__init__.py",
		"stellar-client/stellar_client/resources/__init__.py",
		"stellar-client/stellar_client/utils/__init__.py",
	}
	for _, initFile := range initFiles {
		if data, err := assets.Assets.ReadFile(initFile); err == nil {
			operationsLogger.Infof("Found embedded file: %s (size: %d bytes)", initFile, len(data))
		} else {
			operationsLogger.Warnf("Missing embedded file: %s (error: %v)", initFile, err)
		}
	}

	err := fs.WalkDir(assets.Assets, "stellar-client", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from stellar-client root
		relPath, err := filepath.Rel("stellar-client", path)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		// Skip the root directory entry itself (relPath == ".")
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			// Create directory
			return os.MkdirAll(targetPath, 0755)
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Read file from embedded FS
		data, err := assets.Assets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Write file to target
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		extractedFiles = append(extractedFiles, relPath)

		// Check if this is an __init__.py file
		if filepath.Base(relPath) == "__init__.py" {
			operationsLogger.Infof("Extracted __init__.py: %s", relPath)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Verify critical files were extracted
	criticalFiles := []string{
		"stellar_client/__init__.py",
		"stellar_client/models/__init__.py",
		"stellar_client/resources/__init__.py",
		"stellar_client/utils/__init__.py",
		"pyproject.toml",
	}

	for _, critical := range criticalFiles {
		found := false
		for _, extracted := range extractedFiles {
			if extracted == critical {
				found = true
				break
			}
		}
		if !found {
			missingInitFiles = append(missingInitFiles, critical)
			operationsLogger.Warnf("Critical file not found in embedded assets: %s", critical)
		}
	}

	if len(missingInitFiles) > 0 {
		operationsLogger.Warnf("Missing critical files in extraction: %v", missingInitFiles)
		operationsLogger.Infof("Extracted files: %v", extractedFiles)
	}

	return nil
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
