package service

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// computeClientInterface defines the interface needed by CondaService
type computeClientInterface interface {
	Run(ctx context.Context, req RunRequest) (*RawExecutionHandle, error)
	Close() error
}

// CondaService provides high-level conda operations over compute protocol
type CondaService struct {
	client computeClientInterface
}

// NewCondaService creates a new CondaService
func NewCondaService(client *Client) *CondaService {
	if client == nil {
		return &CondaService{client: nil}
	}
	return &CondaService{
		client: client,
	}
}

// ListEnvironments lists all conda environments on remote device
func (s *CondaService) ListEnvironments(ctx context.Context) (map[string]string, error) {
	if s.client == nil {
		return nil, fmt.Errorf("client is nil")
	}

	// Execute conda env list remotely
	handle, err := s.client.Run(ctx, RunRequest{
		Command: "conda",
		Args:    []string{"env", "list"},
	})
	if err != nil {
		// Check for connection errors
		if err == io.ErrClosedPipe || err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("connection error: %w", err)
		}
		return nil, fmt.Errorf("failed to execute conda env list: %w", err)
	}

	if handle == nil {
		return nil, fmt.Errorf("handle is nil")
	}

	// Read output in background
	outputCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		output, err := io.ReadAll(handle.Stdout)
		if err != nil {
			errCh <- err
			return
		}
		outputCh <- output
	}()

	// Wait for completion
	select {
	case err := <-handle.Done:
		if err != nil {
			handle.Cancel()
			return nil, fmt.Errorf("conda env list failed: %w", err)
		}
		// Read output
		var output []byte
		select {
		case output = <-outputCh:
		case err := <-errCh:
			handle.Cancel()
			return nil, fmt.Errorf("failed to read output: %w", err)
		}
		// Get exit code
		exitCode := <-handle.ExitCode
		if exitCode != 0 {
			stderr, _ := io.ReadAll(handle.Stderr)
			return nil, fmt.Errorf("conda env list exited with code %d: %s", exitCode, string(stderr))
		}
		// Parse output
		envs, err := parseEnvListOutput(string(output))
		if err != nil {
			return nil, fmt.Errorf("failed to parse env list output: %w", err)
		}
		return envs, nil
	case <-ctx.Done():
		handle.Cancel()
		return nil, fmt.Errorf("timeout waiting for conda env list: %w", ctx.Err())
	case err := <-errCh:
		handle.Cancel()
		return nil, fmt.Errorf("failed to read output: %w", err)
	}
}

// GetEnvironment returns the path of a specific environment
func (s *CondaService) GetEnvironment(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("environment name is empty")
	}

	envs, err := s.ListEnvironments(ctx)
	if err != nil {
		return "", err
	}

	if path, ok := envs[name]; ok {
		return path, nil
	}

	return "", fmt.Errorf("environment not found: %s", name)
}

// CreateEnvironment creates a conda environment on remote device
func (s *CondaService) CreateEnvironment(ctx context.Context, name, pythonVersion string) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("client is nil")
	}
	if name == "" {
		return "", fmt.Errorf("environment name is empty")
	}
	if pythonVersion == "" {
		return "", fmt.Errorf("python version is empty")
	}

	// Check if environment already exists
	if envPath, err := s.GetEnvironment(ctx, name); err == nil {
		// Environment already exists, return existing path
		return envPath, nil
	}

	// Create environment
	handle, err := s.client.Run(ctx, RunRequest{
		Command: "conda",
		Args:    []string{"create", "--name", name, fmt.Sprintf("python=%s", pythonVersion), "-y"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute conda create: %w", err)
	}

	// Read stdout and stderr concurrently
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(handle.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(handle.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	select {
	case err := <-handle.Done:
		if err != nil {
			handle.Cancel()
			return "", fmt.Errorf("conda create failed: %w", err)
		}
	case <-ctx.Done():
		handle.Cancel()
		return "", fmt.Errorf("timeout waiting for conda create: %w", ctx.Err())
	}

	exitCode := <-handle.ExitCode

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		if exitCode != 0 {
			select {
			case stderr := <-stderrCh:
				return "", fmt.Errorf("conda create exited with code %d (stdout read error: %v)\nstderr: %s", exitCode, err, string(stderr))
			default:
			}
			return "", fmt.Errorf("conda create exited with code %d (stdout read error: %v)", exitCode, err)
		}
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}

	// Collect stderr
	var stderr []byte
	select {
	case stderr = <-stderrCh:
	case err := <-stderrErrCh:
		if exitCode != 0 {
			return "", fmt.Errorf("conda create exited with code %d\nstdout: %s\nstderr read error: %v", exitCode, string(stdout), err)
		}
		stderr = []byte{}
	}

	if exitCode != 0 {
		errMsg := string(stderr)
		if errMsg == "" {
			errMsg = string(stdout)
		}
		return "", fmt.Errorf("conda create exited with code %d: %s", exitCode, errMsg)
	}

	// Verify creation by checking if environment exists
	envPath, err := s.GetEnvironment(ctx, name)
	if err != nil {
		return "", fmt.Errorf("environment creation failed: environment does not exist after creation: %w", err)
	}

	return envPath, nil
}

// RemoveEnvironment removes a conda environment on remote device
func (s *CondaService) RemoveEnvironment(ctx context.Context, name string) error {
	if s.client == nil {
		return fmt.Errorf("client is nil")
	}
	if name == "" {
		return fmt.Errorf("environment name is empty")
	}

	// Check if environment exists
	if _, err := s.GetEnvironment(ctx, name); err != nil {
		return fmt.Errorf("environment not found: %s", name)
	}

	// Remove environment
	handle, err := s.client.Run(ctx, RunRequest{
		Command: "conda",
		Args:    []string{"env", "remove", "--name", name, "-y"},
	})
	if err != nil {
		return fmt.Errorf("failed to execute conda env remove: %w", err)
	}

	// Read stdout and stderr concurrently
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(handle.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(handle.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	select {
	case err := <-handle.Done:
		if err != nil {
			handle.Cancel()
			return fmt.Errorf("conda env remove failed: %w", err)
		}
	case <-ctx.Done():
		handle.Cancel()
		return fmt.Errorf("timeout waiting for conda env remove: %w", ctx.Err())
	}

	exitCode := <-handle.ExitCode

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		if exitCode != 0 {
			select {
			case stderr := <-stderrCh:
				return fmt.Errorf("conda env remove exited with code %d (stdout read error: %v)\nstderr: %s", exitCode, err, string(stderr))
			default:
			}
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
			return fmt.Errorf("conda env remove exited with code %d\nstdout: %s\nstderr read error: %v", exitCode, string(stdout), err)
		}
		stderr = []byte{}
	}

	if exitCode != 0 {
		errMsg := string(stderr)
		if errMsg == "" {
			errMsg = string(stdout)
		}
		return fmt.Errorf("conda env remove exited with code %d: %s", exitCode, errMsg)
	}

	// Verify removal by checking if environment no longer exists
	_, err = s.GetEnvironment(ctx, name)
	if err == nil {
		return fmt.Errorf("environment removal failed: environment still exists")
	}

	return nil
}

// RunPython executes Python code in a conda environment
func (s *CondaService) RunPython(ctx context.Context, env string, code string, stdin io.Reader) (*RawExecutionHandle, error) {
	if s.client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if code == "" {
		return nil, fmt.Errorf("code is empty")
	}

	// Check if environment exists
	if _, err := s.GetEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("environment not found: %s", env)
	}

	// Execute Python code with CONDA_ENV set
	envVars := map[string]string{
		"CONDA_ENV": env,
	}

	handle, err := s.client.Run(ctx, RunRequest{
		Command: "python",
		Args:    []string{"-c", code},
		Env:     envVars,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python code: %w", err)
	}

	// If stdin is provided, copy it to handle.Stdin in a goroutine
	if stdin != nil {
		if handle.Stdin != nil {
			go func() {
				defer handle.Stdin.Close()
				_, _ = io.Copy(handle.Stdin, stdin)
			}()
		}
	} else {
		// Close stdin if not provided
		if handle.Stdin != nil {
			handle.Stdin.Close()
		}
	}

	return handle, nil
}

// RunScript executes a Python script in a conda environment
func (s *CondaService) RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (*RawExecutionHandle, error) {
	if s.client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if scriptPath == "" {
		return nil, fmt.Errorf("script path is empty")
	}

	// Check if environment exists
	if _, err := s.GetEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("environment not found: %s", env)
	}

	// Execute Python script with CONDA_ENV set
	envVars := map[string]string{
		"CONDA_ENV": env,
	}

	scriptArgs := append([]string{scriptPath}, args...)

	handle, err := s.client.Run(ctx, RunRequest{
		Command: "python",
		Args:    scriptArgs,
		Env:     envVars,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python script: %w", err)
	}

	// If stdin is provided, copy it to handle.Stdin in a goroutine
	if stdin != nil {
		if handle.Stdin != nil {
			go func() {
				defer handle.Stdin.Close()
				_, _ = io.Copy(handle.Stdin, stdin)
			}()
		}
	} else {
		// Close stdin if not provided
		if handle.Stdin != nil {
			handle.Stdin.Close()
		}
	}

	return handle, nil
}

// InstallPackage installs a package in a conda environment
func (s *CondaService) InstallPackage(ctx context.Context, env, packageName string) error {
	if s.client == nil {
		return fmt.Errorf("client is nil")
	}
	if env == "" {
		return fmt.Errorf("environment name is empty")
	}
	if packageName == "" {
		return fmt.Errorf("package name is empty")
	}

	// Check if environment exists
	if _, err := s.GetEnvironment(ctx, env); err != nil {
		return fmt.Errorf("environment not found: %s", env)
	}

	// Install package using conda run with pip install
	envVars := map[string]string{
		"CONDA_ENV": env,
	}

	handle, err := s.client.Run(ctx, RunRequest{
		Command: "pip",
		Args:    []string{"install", packageName},
		Env:     envVars,
	})
	if err != nil {
		return fmt.Errorf("failed to execute pip install: %w", err)
	}

	// Read stdout and stderr concurrently
	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		output, err := io.ReadAll(handle.Stdout)
		if err != nil {
			stdoutErrCh <- err
			return
		}
		stdoutCh <- output
	}()

	go func() {
		output, err := io.ReadAll(handle.Stderr)
		if err != nil {
			stderrErrCh <- err
			return
		}
		stderrCh <- output
	}()

	// Wait for completion
	select {
	case err := <-handle.Done:
		if err != nil {
			handle.Cancel()
			// Check if it's "already satisfied" which is actually success
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
			return fmt.Errorf("pip install failed: %w", err)
		}
	case <-ctx.Done():
		handle.Cancel()
		return fmt.Errorf("timeout waiting for pip install: %w", ctx.Err())
	}

	exitCode := <-handle.ExitCode

	// Collect stdout
	var stdout []byte
	select {
	case stdout = <-stdoutCh:
	case err := <-stdoutErrCh:
		if exitCode != 0 {
			select {
			case stderr := <-stderrCh:
				return fmt.Errorf("pip install exited with code %d (stdout read error: %v)\nstderr: %s", exitCode, err, string(stderr))
			default:
			}
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
			return fmt.Errorf("pip install exited with code %d\nstdout: %s\nstderr read error: %v", exitCode, string(stdout), err)
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
		return fmt.Errorf("pip install exited with code %d: %s", exitCode, errMsg)
	}

	return nil
}

// parseEnvListOutput parses conda env list output
func parseEnvListOutput(output string) (map[string]string, error) {
	envs := make(map[string]string)

	re, err := regexp.Compile(`(\S+)\s{3,}[* ]*(\S+)`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex: %w", err)
	}

	matches := re.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			name := strings.TrimSpace(match[1])
			path := strings.TrimSpace(match[2])
			// Strip asterisk from name if present (active environment marker)
			name = strings.TrimSuffix(name, "*")
			name = strings.TrimSpace(name)
			// Ignore base environment
			if name != "" && name != "base" && path != "" {
				envs[name] = path
			}
		}
	}

	return envs, nil
}
