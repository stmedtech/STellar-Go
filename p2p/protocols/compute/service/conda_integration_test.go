package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeServer_RawCommandStillWorks tests that raw commands work independently of conda
func TestComputeServer_RawCommandStillWorks(t *testing.T) {
	p := startComputePair(t) // Uses RawExecutor directly

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		cmd = "echo"
		args = []string{"hello"}
	}

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "raw-command-test",
		Command: cmd,
		Args:    args,
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_CondaCommandWorks tests that __conda run commands work
// __conda run is for raw conda commands like "conda env list"
func TestComputeServer_CondaCommandWorks(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Test with __conda run command (raw conda command)
	// __conda run env list becomes: conda env list
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-command-test",
		Command: "__conda",
		Args:    []string{"run", "env", "list"},
	})
	if err != nil {
		// If conda is not available, skip the test
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)

	// The output should contain environment list, indicating conda was used
	assert.Contains(t, string(out), "base")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_MixedCommands tests handling mix of raw and conda commands
func TestComputeServer_MixedCommands(t *testing.T) {
	requireNonWindows(t)

	// Check if conda operations creator is registered
	if GetCondaOperationsCreator() == nil {
		t.Skip("conda operations creator not registered")
		return
	}

	p := startComputePair(t)

	// First, run a raw command
	var cmd1 string
	var args1 []string
	if runtime.GOOS == "windows" {
		cmd1 = "cmd"
		args1 = []string{"/c", "echo", "raw"}
	} else {
		cmd1 = "echo"
		args1 = []string{"raw"}
	}

	h1, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "raw-test",
		Command: cmd1,
		Args:    args1,
	})
	require.NoError(t, err)
	require.NoError(t, h1.Stdin.Close())
	out1, _ := io.ReadAll(h1.Stdout)
	require.NoError(t, <-h1.Done)

	// Then, run a conda command using __conda run-python
	h2, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "print('conda')"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NoError(t, h2.Stdin.Close())
	out2, _ := io.ReadAll(h2.Stdout)
	require.NoError(t, <-h2.Done)

	// Both should succeed
	assert.Contains(t, string(out1), "raw")
	assert.Contains(t, string(out2), "conda")
}

// TestComputeServer_ExecuteInCondaEnv tests executing commands in conda environment
func TestComputeServer_ExecuteInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Execute Python code in base environment using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-env-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "import sys; print('conda_env:', sys.executable)"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	stderr, err := io.ReadAll(h.Stderr)
	require.NoError(t, err)

	// Should contain conda environment indicator
	assert.Contains(t, string(out), "conda_env:")

	err = <-h.Done
	if err != nil {
		t.Fatalf("execution failed: %v, stderr: %s, stdout: %s", err, string(stderr), string(out))
	}
	exitCode := <-h.ExitCode
	assert.Equal(t, 0, exitCode, "Execution should succeed: stderr=%s, stdout=%s", string(stderr), string(out))
}

// TestComputeServer_InvalidCondaEnv tests handling invalid environment gracefully
func TestComputeServer_InvalidCondaEnv(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Try to execute in non-existent environment using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "invalid-env-test",
		Command: "__conda",
		Args:    []string{"run-python", "nonexistent-env-12345", "print('test')"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)
	stderr, _ := io.ReadAll(h.Stderr)

	err = <-h.Done
	// Should fail with non-zero exit code
	assert.Error(t, err)
	exitCode := <-h.ExitCode
	assert.NotEqual(t, 0, exitCode)

	// Should have error message about environment
	assert.NotEmpty(t, string(stderr))
}

// TestComputeServer_StreamingInCondaEnv tests that streaming works in conda environment
func TestComputeServer_StreamingInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Execute command that produces streaming output using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "streaming-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "import time, sys; [print(f'line {i}') or sys.stdout.flush() for i in range(5)]"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)

	// Should have all lines
	for i := 0; i < 5; i++ {
		assert.Contains(t, string(out), fmt.Sprintf("line %d", i))
	}

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_CancelInCondaEnv tests that cancellation works in conda environment
func TestComputeServer_CancelInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Start a long-running command using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "cancel-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "import time; time.sleep(10)"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	// Cancel immediately
	require.NoError(t, h.Cancel())

	err = <-h.Done
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancel")
}

// TestComputeServer_CondaErrorPropagation tests that conda errors propagate correctly
func TestComputeServer_CondaErrorPropagation(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Execute invalid Python code using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "error-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "invalid python syntax!!!"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)
	stderr, _ := io.ReadAll(h.Stderr)

	err = <-h.Done
	assert.Error(t, err)
	exitCode := <-h.ExitCode
	assert.NotEqual(t, 0, exitCode)

	// Should have error in stderr (Python syntax error)
	// Python syntax errors can appear in different formats depending on Python version
	stderrStr := string(stderr)
	hasError := assert.Contains(t, stderrStr, "SyntaxError") ||
		assert.Contains(t, stderrStr, "error") ||
		assert.Contains(t, stderrStr, "invalid") ||
		assert.Contains(t, stderrStr, "Traceback")
	assert.True(t, hasError, "stderr should contain error message, got: %s", stderrStr)
}

// TestComputeServer_CommandErrorInCondaEnv tests that command errors in conda env propagate
func TestComputeServer_CommandErrorInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Execute command that fails using __conda run-python
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "cmd-error-test",
		Command: "__conda",
		Args:    []string{"run-python", "base", "import sys; sys.exit(42)"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)

	err = <-h.Done
	// May or may not have error, but exit code should be 42
	// Note: conda run may modify exit codes in some cases, so we check for non-zero
	exitCode := <-h.ExitCode
	// Exit code should be non-zero (42 or potentially modified by conda)
	assert.NotEqual(t, 0, exitCode, "command should fail with non-zero exit code")
}

// TestComputeServer_NetworkError tests that network errors are handled correctly
func TestComputeServer_NetworkError(t *testing.T) {
	// This test verifies that network errors (connection drops) are handled
	// We'll simulate by closing the connection
	clientConn, serverConn := net.Pipe()
	executor := NewRawExecutor()

	server := NewServer(serverConn, executor)
	require.NotNil(t, server)

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	acceptErr := make(chan error, 1)
	go func() { acceptErr <- server.Accept() }()

	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	// Close connection abruptly
	_ = clientConn.Close()
	_ = serverConn.Close()

	// Server should handle the close gracefully
	_ = server.Close()
}

// TestComputeServer_ConcurrentCondaCommands tests handling concurrent conda commands
func TestComputeServer_ConcurrentCondaCommands(t *testing.T) {
	requireNonWindows(t)

	// Check if conda operations creator is registered
	if GetCondaOperationsCreator() == nil {
		t.Skip("conda operations creator not registered")
	}

	p := startComputePair(t)

	// Run multiple conda commands concurrently with unique run IDs
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(idx int) {
			h, err := p.client.Run(context.Background(), RunRequest{
				RunID:   fmt.Sprintf("concurrent-test-%d", idx),
				Command: "__conda",
				Args:    []string{"run-python", "base", "print('test')"},
			})
			if err != nil {
				done <- err
				return
			}
			require.NoError(t, h.Stdin.Close())
			_, _ = io.ReadAll(h.Stdout)
			done <- <-h.Done
		}(i)
	}

	// All should succeed with unique run IDs
	successCount := 0
	for i := 0; i < 3; i++ {
		err := <-done
		if err == nil {
			successCount++
		}
	}
	// All concurrent commands should succeed when conda is available
	assert.Equal(t, 3, successCount, "All concurrent commands should succeed")
}

// TestComputeServer_CondaPathNotFound tests handling when conda operations creator is not registered
// In Docker, this should not happen, but we test the error handling
func TestComputeServer_CondaPathNotFound(t *testing.T) {
	// This test verifies error handling when creator is not registered
	if GetCondaOperationsCreator() != nil {
		// In Docker with creator registered, test that __conda commands work
		p := startComputePair(t)

		// Try to run __conda list - should work in Docker
		h, err := p.client.Run(context.Background(), RunRequest{
			RunID:   "conda-list-test",
			Command: "__conda",
			Args:    []string{"list"},
		})
		require.NoError(t, err)
		require.NotNil(t, h)

		require.NoError(t, h.Stdin.Close())
		stdout, _ := io.ReadAll(h.Stdout)
		err = <-h.Done
		require.NoError(t, err)                    // Should succeed in Docker
		assert.Contains(t, string(stdout), "base") // Should list environments
		return
	}

	// If creator not registered, test error handling
	clientConn, serverConn := net.Pipe()
	executor := NewRawExecutor()

	server := NewServer(serverConn, executor)
	require.NotNil(t, server)

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	acceptErr := make(chan error, 1)
	go func() { acceptErr <- server.Accept() }()

	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	// Try to run __conda command - should fail because creator not registered
	// When creator is nil, sendRunError is called, which sends an error response
	// This means client.Run should return an error immediately
	h, err := client.Run(context.Background(), RunRequest{
		RunID:   "conda-not-found-test",
		Command: "__conda",
		Args:    []string{"list"},
	})
	// When creator is not registered, sendRunError is called, so Run() should return an error
	if err != nil {
		// Expected: error should mention conda
		assert.Contains(t, err.Error(), "conda")
		assert.Nil(t, h)
	} else {
		// If Run() succeeded, it means creator is registered (shouldn't happen in this branch)
		// In this case, the execution might succeed or fail depending on conda availability
		// But since we're testing the error path, we should skip if creator is registered
		if GetCondaOperationsCreator() != nil {
			t.Skip("conda operations creator is registered (test requires it to be unregistered)")
			return
		}
		// If creator is not registered but Run() succeeded, check execution fails
		require.NotNil(t, h)
		require.NoError(t, h.Stdin.Close())
		stderr, _ := io.ReadAll(h.Stderr)
		err = <-h.Done
		assert.Error(t, err)                        // Should fail because creator not registered
		assert.Contains(t, string(stderr), "conda") // Error should mention conda
	}

	_ = client.Close()
	_ = server.Close()
}

// TestComputeServer_CondaList tests __conda list command
func TestComputeServer_CondaList(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-list-test",
		Command: "__conda",
		Args:    []string{"list"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "base")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_CondaGet tests __conda get command
func TestComputeServer_CondaGet(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-get-test",
		Command: "__conda",
		Args:    []string{"get", "base"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	stderr, _ := io.ReadAll(h.Stderr)

	err = <-h.Done
	exitCode := <-h.ExitCode

	// If execution failed, check stderr for error details and skip if conda not available
	if err != nil {
		stderrStr := string(stderr)
		if strings.Contains(stderrStr, "conda") || strings.Contains(err.Error(), "conda") {
			t.Skipf("conda not available: %v", err)
			return
		}
		// Otherwise, fail the test with error details
		t.Fatalf("execution failed: %v, stderr: %s, stdout: %s", err, stderrStr, string(stdout))
	}

	assert.NotEmpty(t, strings.TrimSpace(string(stdout)))
	assert.Equal(t, 0, exitCode)
}

// TestComputeServer_CondaPath tests __conda path command
func TestComputeServer_CondaPath(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-path-test",
		Command: "__conda",
		Args:    []string{"path"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	stderr, _ := io.ReadAll(h.Stderr)

	err = <-h.Done
	exitCode := <-h.ExitCode

	// If execution failed, check stderr for error details and skip if conda not available
	if err != nil {
		stderrStr := string(stderr)
		if strings.Contains(stderrStr, "conda") || strings.Contains(err.Error(), "conda") {
			t.Skipf("conda not available: %v", err)
			return
		}
		// Otherwise, fail the test with error details
		t.Fatalf("execution failed: %v, stderr: %s, stdout: %s", err, stderrStr, string(stdout))
	}

	path := strings.TrimSpace(string(stdout))
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "conda")
	assert.Equal(t, 0, exitCode)
}

// TestComputeServer_CondaVersion tests __conda version command
func TestComputeServer_CondaVersion(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-version-test",
		Command: "__conda",
		Args:    []string{"version"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	version := strings.TrimSpace(string(stdout))
	assert.NotEmpty(t, version)
	// Version should contain numbers (e.g., "conda 24.x.x" or just version number)
	assert.Regexp(t, `\d+`, version)

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_CondaRemove tests __conda remove command
func TestComputeServer_CondaRemove(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envName := fmt.Sprintf("test-remove-%d", time.Now().UnixNano())

	// Create environment first
	createH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-create-for-remove",
		Command: "__conda",
		Args:    []string{"create", envName, "--python", "3.9"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NoError(t, createH.Stdin.Close())
	_, _ = io.ReadAll(createH.Stdout)
	// Wait for create to complete (with timeout to prevent hanging)
	select {
	case err := <-createH.Done:
		if err != nil {
			t.Fatalf("Failed to create environment: %v", err)
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("Create environment timed out after 2 minutes")
	}

	// Remove the environment
	removeH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-remove-test",
		Command: "__conda",
		Args:    []string{"remove", envName},
	})
	require.NoError(t, err)
	require.NotNil(t, removeH)

	require.NoError(t, removeH.Stdin.Close())
	stdout, err := io.ReadAll(removeH.Stdout)
	require.NoError(t, err)
	stderr, err := io.ReadAll(removeH.Stderr)
	require.NoError(t, err)

	// Wait for remove to complete (with timeout to prevent hanging)
	select {
	case err := <-removeH.Done:
		if err != nil {
			t.Fatalf("execution failed: %v, stderr: %s, stdout: %s", err, string(stderr), string(stdout))
		}
		exitCode := <-removeH.ExitCode
		assert.Equal(t, 0, exitCode, "Remove should succeed: stderr=%s, stdout=%s", string(stderr), string(stdout))
	case <-time.After(2 * time.Minute):
		t.Fatal("Remove environment timed out after 2 minutes")
	}
}

// TestComputeServer_CondaInstallPackage tests __conda install command
func TestComputeServer_CondaInstallPackage(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envName := fmt.Sprintf("test-install-%d", time.Now().UnixNano())

	// Create environment first
	createH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-create-for-install",
		Command: "__conda",
		Args:    []string{"create", envName, "--python", "3.9"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NoError(t, createH.Stdin.Close())
	_, _ = io.ReadAll(createH.Stdout)
	<-createH.Done

	// Install a package (requests is lightweight)
	installH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-install-test",
		Command: "__conda",
		Args:    []string{"install", envName, "requests"},
	})
	require.NoError(t, err)
	require.NotNil(t, installH)

	require.NoError(t, installH.Stdin.Close())
	_, err = io.ReadAll(installH.Stdout)
	require.NoError(t, err)

	err = <-installH.Done
	require.NoError(t, err)
	assert.Equal(t, 0, <-installH.ExitCode)

	// Cleanup
	removeH, _ := p.client.Run(ctx, RunRequest{
		RunID:   "conda-remove-after-install",
		Command: "__conda",
		Args:    []string{"remove", envName},
	})
	if removeH != nil {
		require.NoError(t, removeH.Stdin.Close())
		_, _ = io.ReadAll(removeH.Stdout)
		<-removeH.Done
	}
}

// TestComputeServer_CondaRunScript tests __conda run-script command
func TestComputeServer_CondaRunScript(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envName := fmt.Sprintf("test-runscript-%d", time.Now().UnixNano())

	// Create environment first
	createH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-create-for-runscript",
		Command: "__conda",
		Args:    []string{"create", envName, "--python", "3.9"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NoError(t, createH.Stdin.Close())
	_, _ = io.ReadAll(createH.Stdout)
	<-createH.Done

	// Create a test script
	scriptPath := "/tmp/test-script-integration.py"
	scriptContent := "#!/usr/bin/env python\nimport sys\nprint('Script args:', sys.argv[1:])\n"
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	// Run the script with arguments
	runH, err := p.client.Run(ctx, RunRequest{
		RunID:   "conda-run-script-test",
		Command: "__conda",
		Args:    []string{"run-script", envName, scriptPath, "arg1", "arg2"},
	})
	require.NoError(t, err)
	require.NotNil(t, runH)

	require.NoError(t, runH.Stdin.Close())
	stdout, err := io.ReadAll(runH.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "arg1")
	assert.Contains(t, string(stdout), "arg2")

	require.NoError(t, <-runH.Done)
	assert.Equal(t, 0, <-runH.ExitCode)

	// Cleanup
	removeH, _ := p.client.Run(ctx, RunRequest{
		RunID:   "conda-remove-after-runscript",
		Command: "__conda",
		Args:    []string{"remove", envName},
	})
	if removeH != nil {
		require.NoError(t, removeH.Stdin.Close())
		_, _ = io.ReadAll(removeH.Stdout)
		<-removeH.Done
	}
}
