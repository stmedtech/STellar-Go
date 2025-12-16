package conda

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"stellar/p2p/protocols/compute/service"

	golog "github.com/ipfs/go-log/v2"
)

var managerLogger = golog.Logger("stellar-conda-manager")

// CondaManager manages conda environments using an Executor interface
type CondaManager struct {
	executor  Executor
	condaPath string
}

// Executor is an interface for executing commands (aliased from service package for convenience)
type Executor = service.Executor

// NewCondaManager creates a new CondaManager
func NewCondaManager(executor Executor, condaPath string) *CondaManager {
	return &CondaManager{
		executor:  executor,
		condaPath: condaPath,
	}
}

// ListEnvironments returns all conda environments (excluding base)
func (m *CondaManager) ListEnvironments(ctx context.Context) (map[string]string, error) {
	if m.executor == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if m.condaPath == "" {
		return nil, fmt.Errorf("conda path is empty")
	}

	// Execute conda env list
	exec, err := m.executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: m.condaPath,
		Args:    []string{"env", "list"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute conda env list: %w", err)
	}

	// Read output
	output, err := io.ReadAll(exec.Stdout)
	if err != nil {
		exec.Cancel()
		return nil, fmt.Errorf("failed to read output: %w", err)
	}

	// Wait for completion
	exitCode, doneErr := waitForExecution(exec)
	if doneErr != nil {
		return nil, fmt.Errorf("conda env list failed: %w", doneErr)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("conda env list exited with code %d", exitCode)
	}

	// Parse output
	envs, err := parseEnvListOutput(string(output))
	if err != nil {
		return nil, fmt.Errorf("failed to parse env list output: %w", err)
	}

	return envs, nil
}

// GetEnvironment returns the path of a specific environment
func (m *CondaManager) GetEnvironment(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("environment name is empty")
	}

	envs, err := m.ListEnvironments(ctx)
	if err != nil {
		return "", err
	}

	path, ok := envs[name]
	if !ok {
		return "", fmt.Errorf("environment not found: %s", name)
	}

	return path, nil
}

// CreateEnvironment creates a new conda environment
func (m *CondaManager) CreateEnvironment(ctx context.Context, name, pythonVersion string) (string, error) {
	if m.executor == nil {
		return "", fmt.Errorf("executor is nil")
	}
	if m.condaPath == "" {
		return "", fmt.Errorf("conda path is empty")
	}
	if name == "" {
		return "", fmt.Errorf("environment name is empty")
	}
	if pythonVersion == "" {
		return "", fmt.Errorf("python version is empty")
	}

	// Check if environment already exists
	if envPath, err := m.GetEnvironment(ctx, name); err == nil {
		// Environment already exists, return existing path
		return envPath, nil
	}

	// Create environment
	exec, err := m.executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: m.condaPath,
		Args:    []string{"create", "--name", name, fmt.Sprintf("python=%s", pythonVersion), "-y"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute conda create: %w", err)
	}

	// Read stdout and stderr concurrently to prevent blocking
	// Conda may write to stderr, and if stderr isn't drained, the process can hang
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	// Start reading both streams concurrently
	go func() {
		output, err := io.ReadAll(exec.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(exec.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion (this ensures the process finishes)
	exitCode, doneErr := waitForExecution(exec)

	// Collect stdout and stderr output concurrently
	// We need to drain both streams to prevent the process from hanging
	// Always read both streams, even if there's an error, to get diagnostic information
	var stdout []byte
	var stderr []byte

	// Collect stdout
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		// If stdout read failed, still try to drain stderr (don't assign, just drain)
		select {
		case stderr = <-stderrCh:
		case <-stderrErrCh:
		}
		// If we have an error from execution, include it
		if doneErr != nil {
			if len(stderr) > 0 {
				return "", fmt.Errorf("conda create failed: %w\nstdout read error: %v\nstderr: %s", doneErr, err, string(stderr))
			}
			return "", fmt.Errorf("conda create failed: %w\nstdout read error: %v", doneErr, err)
		}
		if exitCode != 0 {
			if len(stderr) > 0 {
				return "", fmt.Errorf("conda create exited with code %d (stdout read error: %v)\nstderr: %s", exitCode, err, string(stderr))
			}
			return "", fmt.Errorf("conda create exited with code %d (stdout read error: %v)", exitCode, err)
		}
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}

	// Collect stderr output (may be empty, that's ok)
	// We must drain stderr even if we don't use it, to prevent process hang
	select {
	case stderr = <-stderrCh:
	case err := <-stderrErrCh:
		// If stderr read failed, we still have stdout
		if doneErr != nil {
			if len(stdout) > 0 {
				return "", fmt.Errorf("conda create failed: %w\nstdout: %s\nstderr read error: %v", doneErr, string(stdout), err)
			}
			return "", fmt.Errorf("conda create failed: %w\nstderr read error: %v", doneErr, err)
		}
		if exitCode != 0 {
			if len(stdout) > 0 {
				return "", fmt.Errorf("conda create exited with code %d\nstdout: %s\nstderr read error: %v", exitCode, string(stdout), err)
			}
			return "", fmt.Errorf("conda create exited with code %d (stderr read error: %v)", exitCode, err)
		}
		// Non-fatal if stderr read fails but exit code is 0
		// Set empty stderr to avoid unused variable warning
		stderr = []byte{}
	}

	// If there was an execution error (not just non-zero exit), return it with output
	if doneErr != nil {
		exec.Cancel()
		if len(stdout) > 0 && len(stderr) > 0 {
			return "", fmt.Errorf("conda create failed: %w\nstdout: %s\nstderr: %s", doneErr, string(stdout), string(stderr))
		} else if len(stderr) > 0 {
			return "", fmt.Errorf("conda create failed: %w\nstderr: %s", doneErr, string(stderr))
		} else if len(stdout) > 0 {
			return "", fmt.Errorf("conda create failed: %w\nstdout: %s", doneErr, string(stdout))
		}
		return "", fmt.Errorf("conda create failed: %w", doneErr)
	}

	if exitCode != 0 {
		// Log both stdout and stderr for debugging (always log, even at info level for visibility)
		managerLogger.Warnf("conda create failed with exit code %d for environment %s", exitCode, name)
		if len(stdout) > 0 {
			managerLogger.Infof("conda create stdout: %s", string(stdout))
		}
		if len(stderr) > 0 {
			managerLogger.Infof("conda create stderr: %s", string(stderr))
		}

		// Check if environment already exists (race condition: another process created it)
		// This can happen in concurrent scenarios
		combinedOutput := strings.ToLower(string(stdout) + "\n" + string(stderr))
		if strings.Contains(combinedOutput, "already exists") ||
			strings.Contains(combinedOutput, "prefix already exists") ||
			strings.Contains(combinedOutput, "environment location") {
			// Environment exists - verify and return it
			if envPath, err := m.GetEnvironment(ctx, name); err == nil {
				managerLogger.Infof("Environment %s already exists (race condition), returning existing path", name)
				return envPath, nil
			}
		}

		// Check for Terms of Service error - this is a configuration issue, not a test failure
		if strings.Contains(combinedOutput, "terms of service") ||
			strings.Contains(combinedOutput, "tos") ||
			strings.Contains(combinedOutput, "condatossnoninteractiveerror") {
			managerLogger.Errorf("Conda Terms of Service not accepted. Please run: conda tos accept --override-channels --channel https://repo.anaconda.com/pkgs/main && conda tos accept --override-channels --channel https://repo.anaconda.com/pkgs/r")
		}

		// Always include both stdout and stderr in error message for debugging
		if len(stdout) > 0 && len(stderr) > 0 {
			return "", fmt.Errorf("conda create exited with code %d\nstdout: %s\nstderr: %s", exitCode, string(stdout), string(stderr))
		} else if len(stderr) > 0 {
			return "", fmt.Errorf("conda create exited with code %d\nstderr: %s", exitCode, string(stderr))
		} else if len(stdout) > 0 {
			return "", fmt.Errorf("conda create exited with code %d\nstdout: %s", exitCode, string(stdout))
		}
		return "", fmt.Errorf("conda create exited with code %d", exitCode)
	}

	// stderr is drained but not used in success path (intentionally)
	_ = stderr

	// Use stdout for verification (stderr may contain warnings but stdout has the key messages)
	output := stdout

	// Verify creation by checking for activation message or success indicators
	// Conda may output the activation message to stdout or stderr
	outputStr := string(output)
	combinedOutput := outputStr
	if len(stderr) > 0 {
		combinedOutput = outputStr + "\n" + string(stderr)
	}

	// Check for various success indicators
	successPatterns := []string{
		fmt.Sprintf("conda activate %s", regexp.QuoteMeta(name)),
		"To activate this environment",
		"environment location",
		"# To activate this environment",
	}

	found := false
	for _, pattern := range successPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(combinedOutput) {
			found = true
			break
		}
	}

	// If no success pattern found, check if environment actually exists
	// (sometimes conda doesn't output the activation message but still succeeds)
	if !found {
		if _, err := m.GetEnvironment(ctx, name); err != nil {
			// Environment doesn't exist, creation failed
			return "", fmt.Errorf("environment creation failed: activation message not found and environment does not exist. stdout: %s, stderr: %s", outputStr, string(stderr))
		}
		// Environment exists, creation succeeded despite missing message
		found = true
	}

	if !found {
		return "", fmt.Errorf("environment creation failed: no success indicators found. stdout: %s, stderr: %s", outputStr, string(stderr))
	}

	// Get the environment path
	envPath, err := m.GetEnvironment(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get environment path after creation: %w", err)
	}

	return envPath, nil
}

// RemoveEnvironment removes a conda environment
func (m *CondaManager) RemoveEnvironment(ctx context.Context, name string) error {
	if m.executor == nil {
		return fmt.Errorf("executor is nil")
	}
	if m.condaPath == "" {
		return fmt.Errorf("conda path is empty")
	}
	if name == "" {
		return fmt.Errorf("environment name is empty")
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Check if environment exists
	if _, err := m.GetEnvironment(ctx, name); err != nil {
		return fmt.Errorf("environment not found: %s", name)
	}

	// Remove environment
	exec, err := m.executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: m.condaPath,
		Args:    []string{"env", "remove", "--name", name, "-y"},
	})
	if err != nil {
		return fmt.Errorf("failed to execute conda env remove: %w", err)
	}

	// Read stdout and stderr concurrently to prevent blocking
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(exec.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(exec.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	exitCode, doneErr := waitForExecution(exec)

	// If there's an execution error (like context cancellation), return it immediately
	// Don't try to verify removal if the command was cancelled
	if doneErr != nil {
		// Still try to drain stdout/stderr for logging, but return the error
		var stdout []byte
		var stderr []byte
		select {
		case stdout = <-stdoutCh:
		default:
		}
		select {
		case stderr = <-stderrCh:
		default:
		}
		if len(stdout) > 0 || len(stderr) > 0 {
			managerLogger.Warnf("conda env remove cancelled or failed: %v\nstdout: %s\nstderr: %s", doneErr, string(stdout), string(stderr))
		}
		exec.Cancel()
		return fmt.Errorf("conda env remove failed: %w", doneErr)
	}

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		select {
		case <-stderrCh:
		case <-stderrErrCh:
		}
		if exitCode != 0 {
			return fmt.Errorf("conda env remove exited with code %d (stdout read error: %v)", exitCode, err)
		}
		return fmt.Errorf("failed to read stdout: %w", err)
	}

	// Collect stderr
	var stderr []byte
	select {
	case stderr = <-stderrCh:
	case err := <-stderrErrCh:
		if exitCode != 0 {
			return fmt.Errorf("conda env remove exited with code %d (stderr read error: %v)", exitCode, err)
		}
		stderr = []byte{}
	}

	if exitCode != 0 {
		errMsg := string(stderr)
		if errMsg == "" {
			errMsg = string(stdout)
		}
		// Log both stdout and stderr for debugging
		managerLogger.Warnf("conda env remove failed with exit code %d", exitCode)
		managerLogger.Debugf("conda env remove stdout: %s", string(stdout))
		managerLogger.Debugf("conda env remove stderr: %s", string(stderr))
		// Include both in error message if available
		if len(stdout) > 0 && len(stderr) > 0 && string(stdout) != string(stderr) {
			return fmt.Errorf("conda env remove exited with code %d\nstdout: %s\nstderr: %s", exitCode, string(stdout), string(stderr))
		}
		return fmt.Errorf("conda env remove exited with code %d: %s", exitCode, errMsg)
	}

	output := stdout
	_ = stderr

	// Verify removal by checking for completion indicators
	// Conda may output different messages depending on version
	outputStr := strings.ToLower(string(output))
	successPatterns := []string{
		"executing transaction: done",
		"executing transaction",
		"removed",
		"deleted",
		"environment removed",
	}

	found := false
	for _, pattern := range successPatterns {
		if strings.Contains(outputStr, pattern) {
			found = true
			break
		}
	}

	// If no pattern found, verify by checking that environment no longer exists
	if !found {
		if _, err := m.GetEnvironment(ctx, name); err == nil {
			// Environment still exists, removal failed
			managerLogger.Warnf("Environment %s still exists after removal attempt. Output: %s", name, string(output))
			return fmt.Errorf("environment deletion failed: environment still exists. Output: %s", string(output))
		}
		// Environment doesn't exist, removal succeeded despite missing message
		managerLogger.Infof("Environment %s removed successfully (no completion message found, but environment no longer exists)", name)
		found = true
	}

	if !found {
		return fmt.Errorf("environment deletion failed: no success indicators found. Output: %s", string(output))
	}

	return nil
}

// UpdateEnvironment updates a conda environment from a YAML file
func (m *CondaManager) UpdateEnvironment(ctx context.Context, name, yamlPath string) error {
	if m.executor == nil {
		return fmt.Errorf("executor is nil")
	}
	if m.condaPath == "" {
		return fmt.Errorf("conda path is empty")
	}
	if name == "" {
		return fmt.Errorf("environment name is empty")
	}
	if yamlPath == "" {
		return fmt.Errorf("YAML file path is empty")
	}

	// Check if environment exists
	if _, err := m.GetEnvironment(ctx, name); err != nil {
		return fmt.Errorf("environment not found: %s", name)
	}

	// Update environment
	exec, err := m.executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: m.condaPath,
		Args:    []string{"env", "update", "--name", name, "--file", yamlPath},
	})
	if err != nil {
		return fmt.Errorf("failed to execute conda env update: %w", err)
	}

	// Read stdout and stderr concurrently to prevent blocking
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(exec.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(exec.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	exitCode, doneErr := waitForExecution(exec)
	if doneErr != nil {
		exec.Cancel()
		return fmt.Errorf("conda env update failed: %w", doneErr)
	}

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		select {
		case <-stderrCh:
		case <-stderrErrCh:
		}
		if exitCode != 0 {
			return fmt.Errorf("conda env update exited with code %d (stdout read error: %v)", exitCode, err)
		}
		return fmt.Errorf("failed to read stdout: %w", err)
	}

	// Collect stderr
	var stderr []byte
	select {
	case stderr = <-stderrCh:
	case err := <-stderrErrCh:
		if exitCode != 0 {
			return fmt.Errorf("conda env update exited with code %d (stderr read error: %v)", exitCode, err)
		}
		stderr = []byte{}
	}

	if exitCode != 0 {
		errMsg := string(stderr)
		if errMsg == "" {
			errMsg = string(stdout)
		}
		// Log both stdout and stderr for debugging
		managerLogger.Warnf("conda env update failed with exit code %d", exitCode)
		managerLogger.Debugf("conda env update stdout: %s", string(stdout))
		managerLogger.Debugf("conda env update stderr: %s", string(stderr))
		// Include both in error message if available
		if len(stdout) > 0 && len(stderr) > 0 && string(stdout) != string(stderr) {
			return fmt.Errorf("conda env update exited with code %d\nstdout: %s\nstderr: %s", exitCode, string(stdout), string(stderr))
		}
		return fmt.Errorf("conda env update exited with code %d: %s", exitCode, errMsg)
	}

	output := stdout
	_ = stderr

	// Verify update by checking for activation message
	re, err := regexp.Compile(fmt.Sprintf("conda activate %s", regexp.QuoteMeta(name)))
	if err != nil {
		return fmt.Errorf("failed to compile regex: %w", err)
	}

	if !re.MatchString(string(output)) {
		return fmt.Errorf("environment update failed: activation message not found")
	}

	return nil
}

// InstallPackage installs a package in a conda environment
func (m *CondaManager) InstallPackage(ctx context.Context, envName, packageName string) error {
	if m.executor == nil {
		return fmt.Errorf("executor is nil")
	}
	if m.condaPath == "" {
		return fmt.Errorf("conda path is empty")
	}
	if envName == "" {
		return fmt.Errorf("environment name is empty")
	}
	if packageName == "" {
		return fmt.Errorf("package name is empty")
	}

	// Check if environment exists
	if _, err := m.GetEnvironment(ctx, envName); err != nil {
		return fmt.Errorf("environment not found: %s", envName)
	}

	// Install package using conda run
	exec, err := m.executor.ExecuteRaw(ctx, service.RawExecutionRequest{
		Command: m.condaPath,
		Args:    []string{"run", "--name", envName, "pip", "install", packageName},
	})
	if err != nil {
		return fmt.Errorf("failed to execute pip install: %w", err)
	}

	// Read stdout and stderr concurrently to prevent blocking
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(exec.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(exec.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	exitCode, doneErr := waitForExecution(exec)
	if doneErr != nil {
		// Try to get output for checking "already satisfied"
		var stdout []byte
		select {
		case stdout = <-stdoutCh:
		default:
		}
		var stderr []byte
		select {
		case stderr = <-stderrCh:
		default:
		}
		combinedOutput := string(stdout) + "\n" + string(stderr)
		if strings.Contains(combinedOutput, "Requirement already satisfied") {
			return nil
		}
		return fmt.Errorf("pip install failed: %w", doneErr)
	}

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		select {
		case <-stderrCh:
		case <-stderrErrCh:
		}
		if exitCode != 0 {
			return fmt.Errorf("pip install exited with code %d (stdout read error: %v)", exitCode, err)
		}
		return fmt.Errorf("failed to read stdout: %w", err)
	}

	// Collect stderr
	var stderr []byte
	select {
	case stderr = <-stderrCh:
	case err := <-stderrErrCh:
		if exitCode != 0 {
			return fmt.Errorf("pip install exited with code %d (stderr read error: %v)", exitCode, err)
		}
		stderr = []byte{}
	}

	// Combine output for checking "already satisfied"
	combinedOutput := string(stdout) + "\n" + string(stderr)

	if exitCode != 0 {
		// Check if package is already installed
		if strings.Contains(combinedOutput, "Requirement already satisfied") {
			return nil
		}
		errMsg := string(stderr)
		if errMsg == "" {
			errMsg = string(stdout)
		}
		// Log both stdout and stderr for debugging
		managerLogger.Warnf("pip install failed with exit code %d", exitCode)
		managerLogger.Debugf("pip install stdout: %s", string(stdout))
		managerLogger.Debugf("pip install stderr: %s", string(stderr))
		// Include both in error message if available
		if len(stdout) > 0 && len(stderr) > 0 && string(stdout) != string(stderr) {
			return fmt.Errorf("pip install exited with code %d\nstdout: %s\nstderr: %s", exitCode, string(stdout), string(stderr))
		}
		return fmt.Errorf("pip install exited with code %d: %s", exitCode, errMsg)
	}

	return nil
}

// parseEnvListOutput parses the output of "conda env list"
func parseEnvListOutput(output string) (map[string]string, error) {
	envs := make(map[string]string)

	re, err := regexp.Compile(`\S+\s{3,}[* ]*\S+`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex: %w", err)
	}

	envLines := re.FindAllString(output, -1)
	if len(envLines) == 0 {
		return envs, nil // Empty is valid (only base environment)
	}

	for _, envLine := range envLines {
		parts := strings.Fields(envLine)
		if len(parts) >= 2 {
			envName := parts[0]
			envPath := parts[len(parts)-1]

			// Strip asterisk from environment name (indicates active environment)
			envName = strings.TrimPrefix(envName, "*")

			// Skip base environment
			if envName != "base" && envName != "" && envPath != "" {
				envs[envName] = envPath
			}
		}
	}

	return envs, nil
}

// waitForExecution waits for a RawExecution to complete and returns exit code and error
func waitForExecution(exec *service.RawExecution) (int, error) {
	doneErr := <-exec.Done
	exitCode := <-exec.ExitCode
	return exitCode, doneErr
}
