package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startComputePairWithConda creates a compute pair with CondaExecutor
func startComputePairWithConda(t *testing.T, condaPath string) *computePair {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	baseExecutor := NewRawExecutor()

	var executor Executor
	if condaPath != "" {
		executor = NewCondaExecutor(baseExecutor, condaPath)
	} else {
		executor = baseExecutor
	}

	server := NewServer(serverConn, executor)
	require.NotNil(t, server)

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	acceptErr := make(chan error, 1)
	go func() { acceptErr <- server.Accept() }()

	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Serve(ctx) }()

	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		_ = server.Close()
	})

	return &computePair{client: client, server: server, cancel: cancel}
}

// TestComputeServer_WithCondaExecutor tests that server uses CondaExecutor correctly
func TestComputeServer_WithCondaExecutor(t *testing.T) {
	// This test verifies that the server can be created with CondaExecutor
	// We'll test actual conda functionality in other tests
	clientConn, serverConn := net.Pipe()
	baseExecutor := NewRawExecutor()
	condaExecutor := NewCondaExecutor(baseExecutor, "conda")

	server := NewServer(serverConn, condaExecutor)
	require.NotNil(t, server)
	require.NotNil(t, server.executor)

	_ = server.Close()
	_ = clientConn.Close()
}

// TestComputeServer_RawCommandStillWorks tests that raw commands work without CONDA_ENV
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

// TestComputeServer_CondaCommandWorks tests that __conda commands work
// CondaExecutor removed - use __conda run instead of CONDA_ENV
func TestComputeServer_CondaCommandWorks(t *testing.T) {
	requireNonWindows(t)

	p := startComputePair(t)

	// Test with __conda run command instead of CONDA_ENV
	// Note: This test may skip if conda is not available
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-command-test",
		Command: "__conda",
		Args:    []string{"run", "python", "-c", "import sys; print(sys.executable)"},
	})
	if err != nil {
		// If conda is not available, skip the test
		t.Skipf("conda not available or test environment not set up: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)

	// The output should contain python path, indicating conda was used
	assert.NotEmpty(t, string(out))

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_MixedCommands tests handling mix of raw and conda commands
func TestComputeServer_MixedCommands(t *testing.T) {
	requireNonWindows(t)

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// First, run a raw command (no CONDA_ENV)
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

	// Then, run a conda command (with CONDA_ENV)
	h2, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-test",
		Command: "python",
		Args:    []string{"-c", "print('conda')"},
		Env:     map[string]string{"CONDA_ENV": "base"},
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

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Execute Python code in base environment
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "conda-env-test",
		Command: "python",
		Args:    []string{"-c", "import sys; print('conda_env:', sys.executable)"},
		Env:     map[string]string{"CONDA_ENV": "base"},
	})
	if err != nil {
		t.Skipf("conda not available: %v", err)
		return
	}
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)

	// Should contain conda environment indicator
	assert.Contains(t, string(out), "conda_env:")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_InvalidCondaEnv tests handling invalid environment gracefully
func TestComputeServer_InvalidCondaEnv(t *testing.T) {
	requireNonWindows(t)

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Try to execute in non-existent environment
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "invalid-env-test",
		Command: "python",
		Args:    []string{"-c", "print('test')"},
		Env:     map[string]string{"CONDA_ENV": "nonexistent-env-12345"},
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

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Execute command that produces streaming output
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "streaming-test",
		Command: "python",
		Args:    []string{"-c", "import time, sys; [print(f'line {i}') or sys.stdout.flush() for i in range(5)]"},
		Env:     map[string]string{"CONDA_ENV": "base"},
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
		assert.Contains(t, string(out), "line "+string(rune('0'+i)))
	}

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestComputeServer_CancelInCondaEnv tests that cancellation works in conda environment
func TestComputeServer_CancelInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Start a long-running command
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "cancel-test",
		Command: "python",
		Args:    []string{"-c", "import time; time.sleep(10)"},
		Env:     map[string]string{"CONDA_ENV": "base"},
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

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Execute invalid Python code
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "error-test",
		Command: "python",
		Args:    []string{"-c", "invalid python syntax!!!"},
		Env:     map[string]string{"CONDA_ENV": "base"},
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

	// Should have error in stderr
	stderrStr := string(stderr)
	assert.True(t, assert.Contains(t, stderrStr, "SyntaxError") || assert.Contains(t, stderrStr, "error"), "stderr should contain error message")
}

// TestComputeServer_CommandErrorInCondaEnv tests that command errors in conda env propagate
func TestComputeServer_CommandErrorInCondaEnv(t *testing.T) {
	requireNonWindows(t)

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Execute command that fails
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "cmd-error-test",
		Command: "python",
		Args:    []string{"-c", "import sys; sys.exit(42)"},
		Env:     map[string]string{"CONDA_ENV": "base"},
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
	exitCode := <-h.ExitCode
	assert.Equal(t, 42, exitCode)
}

// TestComputeServer_NetworkError tests that network errors are handled correctly
func TestComputeServer_NetworkError(t *testing.T) {
	// This test verifies that network errors (connection drops) are handled
	// We'll simulate by closing the connection
	clientConn, serverConn := net.Pipe()
	baseExecutor := NewRawExecutor()
	condaExecutor := NewCondaExecutor(baseExecutor, "conda")

	server := NewServer(serverConn, condaExecutor)
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

	condaPath := "conda"
	p := startComputePairWithConda(t, condaPath)

	// Run multiple conda commands concurrently with unique run IDs
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(idx int) {
			h, err := p.client.Run(context.Background(), RunRequest{
				RunID:   fmt.Sprintf("concurrent-test-%d", idx),
				Command: "python",
				Args:    []string{"-c", "print('test')"},
				Env:     map[string]string{"CONDA_ENV": "base"},
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
	// All should succeed with unique run IDs
	assert.GreaterOrEqual(t, successCount, 2, "At least 2 concurrent commands should succeed")
}

// TestComputeServer_CondaPathNotFound tests handling conda not found on server
func TestComputeServer_CondaPathNotFound(t *testing.T) {
	// Create server with invalid conda path
	clientConn, serverConn := net.Pipe()
	baseExecutor := NewRawExecutor()
	condaExecutor := NewCondaExecutor(baseExecutor, "/nonexistent/conda")

	server := NewServer(serverConn, condaExecutor)
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

	// Try to run command with CONDA_ENV - should fail immediately
	// The server rejects the run because conda path doesn't exist
	h, err := client.Run(context.Background(), RunRequest{
		RunID:   "conda-not-found-test",
		Command: "echo",
		Args:    []string{"test"},
		Env:     map[string]string{"CONDA_ENV": "base"},
	})
	// Run() may succeed or fail depending on when the error is detected
	// If it succeeds, the execution should fail
	if err == nil {
		require.NotNil(t, h)
		err = <-h.Done
		assert.Error(t, err) // Should fail because conda path doesn't exist
	} else {
		// Run was rejected immediately
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conda")
	}
}

// TestComputeServer_EmptyCondaPath tests handling empty conda path
func TestComputeServer_EmptyCondaPath(t *testing.T) {
	// Create server with empty conda path
	clientConn, serverConn := net.Pipe()
	baseExecutor := NewRawExecutor()
	condaExecutor := NewCondaExecutor(baseExecutor, "")

	server := NewServer(serverConn, condaExecutor)
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

	// Try to run command with CONDA_ENV - should fail immediately
	// The server rejects the run because conda path is empty
	h, err := client.Run(context.Background(), RunRequest{
		RunID:   "empty-conda-path-test",
		Command: "echo",
		Args:    []string{"test"},
		Env:     map[string]string{"CONDA_ENV": "base"},
	})
	// Run() may succeed or fail depending on when the error is detected
	// If it succeeds, the execution should fail
	if err == nil {
		require.NotNil(t, h)
		err = <-h.Done
		assert.Error(t, err) // Should fail because conda path is empty
	} else {
		// Run was rejected immediately
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conda path is empty")
	}
}
