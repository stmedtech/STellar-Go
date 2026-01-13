//go:build !conda

package conda

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// MockCondaOperations is a mock implementation of CondaOperations for testing
// without requiring actual conda installation
type MockCondaOperations struct {
	// Mock data
	Environments map[string]string // env name -> path
	Version      string
	CondaPath    string
}

// NewMockCondaOperations creates a new mock conda operations instance
func NewMockCondaOperations() *MockCondaOperations {
	return &MockCondaOperations{
		Environments: map[string]string{
			"base": "/mock/conda/base",
		},
		Version:   "24.1.0",
		CondaPath: "/mock/conda/bin/conda",
	}
}

// Install mocks conda installation
func (m *MockCondaOperations) Install(ctx context.Context, pythonVersion string) (*CommandExecution, error) {
	return createMockCommandExecution("conda installed", "", 0), nil
}

// CommandPath returns the mock conda path
func (m *MockCondaOperations) CommandPath(ctx context.Context) (string, error) {
	return m.CondaPath, nil
}

// GetCondaVersion returns mock conda version
// Format should match real conda version output (e.g., "conda 24.1.0")
func (m *MockCondaOperations) GetCondaVersion(ctx context.Context) (*CommandExecution, error) {
	// Match real conda version format
	output := fmt.Sprintf("conda %s\n", m.Version)
	return createMockCommandExecution(output, "", 0), nil
}

// ListEnvironments returns mock environment list
func (m *MockCondaOperations) ListEnvironments(ctx context.Context) (*CommandExecution, error) {
	// Format similar to "conda env list" output
	output := "# conda environments:\n#\nbase                  /mock/conda/base\n"
	for name, path := range m.Environments {
		if name != "base" {
			output += fmt.Sprintf("%-20s %s\n", name, path)
		}
	}
	return createMockCommandExecution(output, "", 0), nil
}

// GetEnvironment returns mock environment path
func (m *MockCondaOperations) GetEnvironment(ctx context.Context, name string) (string, error) {
	if path, ok := m.Environments[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("environment not found: %s", name)
}

// CreateEnvironment adds a mock environment
func (m *MockCondaOperations) CreateEnvironment(ctx context.Context, name, pythonVersion string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if pythonVersion == "" {
		return nil, fmt.Errorf("python version is empty")
	}

	// Add to mock environments
	m.Environments[name] = "/mock/conda/envs/" + name

	return createMockCommandExecution("environment created", "", 0), nil
}

// RemoveEnvironment removes a mock environment
func (m *MockCondaOperations) RemoveEnvironment(ctx context.Context, name string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}

	// Accept any non-empty environment name (environments may have been created in previous requests)
	// Only fail if it's obviously invalid (empty or base which shouldn't be removed)
	if name == "base" {
		return nil, fmt.Errorf("cannot remove base environment")
	}

	// Remove from mock if it exists, but don't fail if it doesn't (may have been created in previous request)
	delete(m.Environments, name)
	return createMockCommandExecution("environment removed", "", 0), nil
}

// UpdateEnvironment mocks environment update
func (m *MockCondaOperations) UpdateEnvironment(ctx context.Context, name, yamlPath string) (*CommandExecution, error) {
	if name == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if yamlPath == "" {
		return nil, fmt.Errorf("yaml path is empty")
	}

	if _, ok := m.Environments[name]; !ok {
		return nil, fmt.Errorf("environment not found: %s", name)
	}

	return createMockCommandExecution("environment updated", "", 0), nil
}

// InstallPackage mocks package installation
func (m *MockCondaOperations) InstallPackage(ctx context.Context, envName, packageName string) (*CommandExecution, error) {
	if envName == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if packageName == "" {
		return nil, fmt.Errorf("package name is empty")
	}

	// Accept any non-empty environment name (environments may have been created in previous requests)
	// Only fail if it's obviously invalid (empty)
	// For base or any other environment, assume it exists (created in previous request)

	return createMockCommandExecution("package installed", "", 0), nil
}

// InstallClient mocks stellar-client installation
func (m *MockCondaOperations) InstallClient(ctx context.Context, envName string) (*CommandExecution, error) {
	if envName == "" {
		return nil, fmt.Errorf("environment name is empty")
	}

	// Accept any non-empty environment name (environments may have been created in previous requests)
	// Only fail if it's obviously invalid (empty)
	// For base or any other environment, assume it exists (created in previous request)

	return createMockCommandExecution("stellar-client installed", "", 0), nil
}

// RunPython mocks Python code execution
func (m *MockCondaOperations) RunPython(ctx context.Context, env, code string, stdin io.Reader) (*CommandExecution, error) {
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if code == "" {
		return nil, fmt.Errorf("code is empty")
	}

	// Check if environment exists
	// For base, always allow. For others, check if it exists in mock.
	// However, if environment name looks like a test environment (contains "test-"),
	// assume it was created in a previous request and allow it
	if env != "base" && !strings.Contains(env, "test-") {
		if _, ok := m.Environments[env]; !ok {
			// Environment not found - return error only for obviously invalid names
			if strings.Contains(env, "nonexistent") || strings.Contains(env, "invalid") {
				return nil, fmt.Errorf("environment not found: %s", env)
			}
			// Otherwise, assume it exists (created in previous request)
		}
	}

	// Handle specific code patterns for tests
	var output string
	var exitCode int

	if strings.Contains(code, "sys.exit(42)") {
		exitCode = 42
		output = ""
	} else if strings.Contains(code, "line") && strings.Contains(code, "range(5)") {
		// Handle streaming test
		output = "line 0\nline 1\nline 2\nline 3\nline 4\n"
		exitCode = 0
	} else if strings.Contains(code, "time.sleep") {
		// For cancellation test - use negative exit code to indicate cancellable
		output = ""
		exitCode = -1 // Special value to make it cancellable
	} else if strings.Contains(code, "invalid python syntax") || strings.Contains(code, "!!!") {
		// Handle invalid Python syntax - return error
		exitCode = 1
		output = ""
		stderr := "SyntaxError: invalid syntax\n"
		return createMockCommandExecution(output, stderr, exitCode), nil
	} else {
		// Default: return code as output
		output = "executed: " + code
		exitCode = 0
	}

	return createMockCommandExecution(output, "", exitCode), nil
}

// RunScript mocks script execution
func (m *MockCondaOperations) RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (*CommandExecution, error) {
	if env == "" {
		return nil, fmt.Errorf("environment name is empty")
	}
	if scriptPath == "" {
		return nil, fmt.Errorf("script path is empty")
	}

	// Check if environment exists
	// For base, always allow. For others, check if it exists in mock.
	// However, if environment name looks like a test environment (contains "test-"),
	// assume it was created in a previous request and allow it
	if env != "base" && !strings.Contains(env, "test-") {
		if _, ok := m.Environments[env]; !ok {
			// Environment not found - return error only for obviously invalid names
			if strings.Contains(env, "nonexistent") || strings.Contains(env, "invalid") {
				return nil, fmt.Errorf("environment not found: %s", env)
			}
			// Otherwise, assume it exists (created in previous request)
		}
	}

	// Include args in output for test (matching Python script format)
	output := "Script args: ["
	if len(args) > 0 {
		argStrs := make([]string, len(args))
		for i, arg := range args {
			argStrs[i] = "'" + arg + "'"
		}
		output += strings.Join(argStrs, ", ")
	}
	output += "]\n"

	return createMockCommandExecution(output, "", 0), nil
}

// RunConda mocks raw conda command execution
func (m *MockCondaOperations) RunConda(ctx context.Context, args []string, stdin io.Reader) (*CommandExecution, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("conda command requires at least one argument")
	}

	// Handle specific commands
	if len(args) >= 2 && args[0] == "env" && args[1] == "list" {
		// Return environment list format
		output := "# conda environments:\n#\nbase                  /mock/conda/base\n"
		for name, path := range m.Environments {
			if name != "base" {
				output += fmt.Sprintf("%-20s %s\n", name, path)
			}
		}
		return createMockCommandExecution(output, "", 0), nil
	}

	// Mock execution based on command
	output := "conda " + strings.Join(args, " ")
	return createMockCommandExecution(output, "", 0), nil
}

// createMockCommandExecution creates a mock CommandExecution
// If exitCode is negative, it indicates the execution should be cancellable
func createMockCommandExecution(stdout, stderr string, exitCode int) *CommandExecution {
	done := make(chan error, 1)
	exitCodeChan := make(chan int, 1)

	// If exitCode is negative, delay completion to allow cancellation
	if exitCode < 0 {
		// For cancellation tests - don't complete immediately
		cancelled := false
		cancelFunc := func() {
			cancelled = true
			done <- fmt.Errorf("command cancelled")
			exitCodeChan <- -1
		}

		// Complete after a short delay, or immediately if cancelled
		go func() {
			time.Sleep(10 * time.Millisecond)
			if !cancelled {
				done <- nil
				exitCodeChan <- 0
			}
		}()

		return &CommandExecution{
			RunID:    generateCommandRunID(),
			Stdin:    nil,
			Stdout:   io.NopCloser(strings.NewReader(stdout)),
			Stderr:   io.NopCloser(strings.NewReader(stderr)),
			Done:     done,
			ExitCode: exitCodeChan,
			Cancel:   cancelFunc,
		}
	}

	// Normal completion
	done <- nil
	exitCodeChan <- exitCode

	return &CommandExecution{
		RunID:    generateCommandRunID(),
		Stdin:    nil,
		Stdout:   io.NopCloser(strings.NewReader(stdout)),
		Stderr:   io.NopCloser(strings.NewReader(stderr)),
		Done:     done,
		ExitCode: exitCodeChan,
		Cancel:   func() {},
	}
}
