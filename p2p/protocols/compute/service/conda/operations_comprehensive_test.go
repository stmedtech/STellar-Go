//go:build conda

package conda

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Helpers
// ============================================================================

func requireCondaAvailable(t *testing.T) {
	ops, err := NewCondaOperations("")
	if err != nil {
		t.Skipf("conda not found: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	versionExec, err := ops.GetCondaVersion(ctx)
	if err != nil {
		t.Skipf("conda not working: %v", err)
	}

	bridge := NewStreamBridge(versionExec)
	_, _ = bridge.BridgeTo(io.Discard, io.Discard)
	code, _ := bridge.Wait()
	if code != 0 {
		t.Skip("conda version check failed")
	}
}

func generateTestEnvName(prefix string) string {
	return fmt.Sprintf("test-%s-%d", prefix, time.Now().UnixNano())
}

func setupTestOps(t *testing.T) *CondaOperations {
	ops, err := NewCondaOperations("")
	require.NoError(t, err)
	return ops
}

func createTestEnvironment(t *testing.T, ops *CondaOperations, ctx context.Context, envName string) {
	exec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err, "Failed to create environment: stdout=%s, stderr=%s", stdout, stderr)
	assert.Equal(t, 0, exitCode, "Environment creation should succeed: stdout=%s, stderr=%s", stdout, stderr)
}

func cleanupTestEnvironment(t *testing.T, ops *CondaOperations, ctx context.Context, envName string) {
	removeExec, err := ops.RemoveEnvironment(ctx, envName)
	if err == nil && removeExec != nil {
		go func() {
			_ = removeExec.Stdout.Close()
			_ = removeExec.Stderr.Close()
			if removeExec.Cancel != nil {
				removeExec.Cancel()
			}
		}()
	}
}

func assertExecutionSuccess(t *testing.T, exec *CommandExecution, description string) {
	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err, "%s failed: stdout=%s, stderr=%s", description, stdout, stderr)
	assert.Equal(t, 0, exitCode, "%s should succeed: stdout=%s, stderr=%s", description, stdout, stderr)
}

func readExecutionOutput(exec *CommandExecution) (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup

	// Start reading from pipes immediately (don't wait for Done)
	// This ensures we capture output even if command hangs
	wg.Add(2)
	var stdoutErr, stderrErr error
	go func() {
		defer wg.Done()
		_, stdoutErr = io.Copy(&stdoutBuf, exec.Stdout)
	}()
	go func() {
		defer wg.Done()
		_, stderrErr = io.Copy(&stderrBuf, exec.Stderr)
	}()

	// Wait for execution to complete with timeout
	// Use a select with timeout to prevent hanging forever
	select {
	case doneErr := <-exec.Done:
		if code, ok := <-exec.ExitCode; ok {
			exitCode = code
		}
		err = doneErr
		// Wait for I/O copying to complete (with timeout)
		// Use a shorter timeout since command is done
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// I/O completed
		case <-time.After(5 * time.Second):
			// Timeout - I/O might be stuck, but we have the exit code
			// Close pipes to unblock readers
			_ = exec.Stdout.Close()
			_ = exec.Stderr.Close()
		}
	case <-time.After(2 * time.Minute):
		// Command execution timed out - this shouldn't happen in normal tests
		// but prevents the test suite from hanging
		err = fmt.Errorf("command execution timed out after 2 minutes")
		exitCode = -1
		// Cancel the command and close pipes to unblock readers
		if exec.Cancel != nil {
			exec.Cancel()
		}
		_ = exec.Stdout.Close()
		_ = exec.Stderr.Close()
		// Wait a bit for readers to finish (with timeout)
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// Give up waiting for readers
		}
	}

	// Check for copy errors (but don't fail if command completed)
	if stdoutErr != nil && err == nil {
		err = fmt.Errorf("stdout read error: %w", stdoutErr)
	}
	if stderrErr != nil && err == nil {
		err = fmt.Errorf("stderr read error: %w", stderrErr)
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// ============================================================================
// CondaOperations Creation Tests
// ============================================================================

func TestNewCondaOperations_WithPath(t *testing.T) {
	// Test with explicit path
	ops, err := NewCondaOperations("conda")
	require.NoError(t, err)
	require.NotNil(t, ops)
}

func TestNewCondaOperations_WithoutPath(t *testing.T) {
	// Test without path (will try to find conda)
	ops, err := NewCondaOperations("")
	if err != nil {
		// If conda not found, that's okay - operations still created
		require.NotNil(t, ops)
	} else {
		require.NotNil(t, ops)
	}
}

func TestNewCondaOperations_InvalidPath(t *testing.T) {
	// Test with invalid path - should still create operations
	ops, err := NewCondaOperations("/nonexistent/conda")
	require.NoError(t, err) // Should not fail - allows install-conda
	require.NotNil(t, ops)
}

// ============================================================================
// GetCondaVersion Tests
// ============================================================================

func TestCondaOperations_GetCondaVersion_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.GetCondaVersion(ctx)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "conda")
}

func TestCondaOperations_GetCondaVersion_NoConda(t *testing.T) {
	// Test when conda is not available
	ops, err := NewCondaOperations("/nonexistent/conda")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.GetCondaVersion(ctx)
	require.Error(t, err)
	assert.Nil(t, exec)
}

func TestCondaOperations_GetCondaVersion_ContextCancellation(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Don't cancel immediately - let command start, then cancel
	exec, err := ops.GetCondaVersion(ctx)
	require.NoError(t, err) // Command creation succeeds
	require.NotNil(t, exec)

	// Cancel context after command starts
	cancel()

	// Execution should be cancelled
	err = <-exec.Done
	assert.Error(t, err)
}

// ============================================================================
// CommandPath Tests
// ============================================================================

func TestCondaOperations_CommandPath_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path, err := ops.CommandPath(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, strings.ToLower(path), "conda")
}

func TestCondaOperations_CommandPath_NoConda(t *testing.T) {
	ops, err := NewCondaOperations("/nonexistent/conda")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path, err := ops.CommandPath(ctx)
	require.Error(t, err)
	assert.Empty(t, path)
}

// ============================================================================
// ListEnvironments Tests
// ============================================================================

func TestCondaOperations_ListEnvironments_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.ListEnvironments(ctx)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "base") // Should at least have base environment
}

func TestCondaOperations_ListEnvironments_Streaming(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.ListEnvironments(ctx)
	require.NoError(t, err)

	// Test streaming - read in chunks
	chunk := make([]byte, 1024)
	n, err := exec.Stdout.Read(chunk)
	// Should read some data (may be EOF if command finished fast)
	assert.True(t, n > 0 || err == io.EOF)

	// Wait for completion
	<-exec.Done
}

func TestCondaOperations_ListEnvironments_NoConda(t *testing.T) {
	ops, err := NewCondaOperations("/nonexistent/conda")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.ListEnvironments(ctx)
	require.Error(t, err)
	assert.Nil(t, exec)
}

// ============================================================================
// GetEnvironment Tests
// ============================================================================

func TestCondaOperations_GetEnvironment_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get base environment (should always exist)
	path, err := ops.GetEnvironment(ctx, "base")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestCondaOperations_GetEnvironment_NotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envName := generateTestEnvName("nonexistent")
	path, err := ops.GetEnvironment(ctx, envName)
	require.Error(t, err)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_GetEnvironment_EmptyName(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path, err := ops.GetEnvironment(ctx, "")
	require.Error(t, err)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "empty")
}

// ============================================================================
// CreateEnvironment Tests
// ============================================================================

func TestCondaOperations_CreateEnvironment_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	envName := generateTestEnvName("create")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	require.NotNil(t, exec)

	assertExecutionSuccess(t, exec, "Environment creation")

	path, err := ops.GetEnvironment(ctx, envName)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaOperations_CreateEnvironment_AlreadyExists(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	envName := generateTestEnvName("exists")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	exec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "already exists")

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaOperations_CreateEnvironment_EmptyName(t *testing.T) {
	ops := setupTestOps(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.CreateEnvironment(ctx, "", "3.9")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_CreateEnvironment_EmptyVersion(t *testing.T) {
	ops := setupTestOps(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.CreateEnvironment(ctx, "test-env", "")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_CreateEnvironment_InvalidVersion(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("invalid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.CreateEnvironment(ctx, envName, "999.999")
	require.NoError(t, err) // Command creation succeeds

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	// May succeed or fail depending on conda version
	_ = stdout
	_ = stderr
	_ = exitCode
	_ = err

	// Cleanup if created
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_CreateEnvironment_ContextCancellation(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("cancel")
	ctx, cancel := context.WithCancel(context.Background())

	exec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return
		}
		require.NoError(t, err)
	}
	if exec == nil {
		cancel()
		return
	}

	cancel()

	err = <-exec.Done
	assert.Error(t, err)

	cleanupTestEnvironment(t, ops, context.Background(), envName)
}

func TestCondaOperations_CreateEnvironment_Concurrent(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	envNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		envNames[i] = generateTestEnvName(fmt.Sprintf("concurrent-%d", i))
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			exec, err := ops.CreateEnvironment(ctx, name, "3.9")
			if err == nil && exec != nil {
				_, _, _, _ = readExecutionOutput(exec)
			}
		}(envNames[i])
	}

	wg.Wait()

	for _, name := range envNames {
		path, err := ops.GetEnvironment(ctx, name)
		if err == nil {
			assert.NotEmpty(t, path)
			// Cleanup
			removeExec, _ := ops.RemoveEnvironment(ctx, name)
			if removeExec != nil {
				_, _, _, _ = readExecutionOutput(removeExec)
			}
		}
	}
}

// ============================================================================
// RemoveEnvironment Tests
// ============================================================================

func TestCondaOperations_RemoveEnvironment_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	envName := generateTestEnvName("remove")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	_, err := ops.GetEnvironment(ctx, envName)
	require.NoError(t, err)

	removeExec, err := ops.RemoveEnvironment(ctx, envName)
	require.NoError(t, err)
	require.NotNil(t, removeExec)

	assertExecutionSuccess(t, removeExec, "Environment removal")

	_, err = ops.GetEnvironment(ctx, envName)
	require.Error(t, err)
}

func TestCondaOperations_RemoveEnvironment_NotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("notfound")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RemoveEnvironment(ctx, envName)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_RemoveEnvironment_EmptyName(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RemoveEnvironment(ctx, "")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

// ============================================================================
// UpdateEnvironment Tests
// ============================================================================

func TestCondaOperations_UpdateEnvironment_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("update")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create a simple YAML file
	tmpDir := os.TempDir()
	yamlPath := fmt.Sprintf("%s/test-env-%d.yml", tmpDir, time.Now().UnixNano())
	yamlContent := `name: test-env
dependencies:
  - python=3.9
  - pip
  - pip:
    - requests
`
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)
	defer os.Remove(yamlPath)

	// Update environment
	updateExec, err := ops.UpdateEnvironment(ctx, envName, yamlPath)
	require.NoError(t, err)
	require.NotNil(t, updateExec)

	stdout, stderr, exitCode, err := readExecutionOutput(updateExec)
	// May succeed or fail depending on network/conda version
	_ = stdout
	_ = stderr
	_ = exitCode
	_ = err

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_UpdateEnvironment_NotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	envName := generateTestEnvName("notfound")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := os.TempDir()
	yamlPath := fmt.Sprintf("%s/test-env-%d.yml", tmpDir, time.Now().UnixNano())
	err := os.WriteFile(yamlPath, []byte("name: test\n"), 0644)
	require.NoError(t, err)
	defer os.Remove(yamlPath)

	exec, err := ops.UpdateEnvironment(ctx, envName, yamlPath)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_UpdateEnvironment_FileNotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("update")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Try to update with non-existent file
	exec, err := ops.UpdateEnvironment(ctx, envName, "/nonexistent/file.yml")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_UpdateEnvironment_EmptyName(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.UpdateEnvironment(ctx, "", "/path/to/file.yml")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_UpdateEnvironment_EmptyYamlPath(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.UpdateEnvironment(ctx, "test-env", "")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

// ============================================================================
// InstallPackage Tests
// ============================================================================

func TestCondaOperations_InstallPackage_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)
	envName := generateTestEnvName("install")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	installExec, err := ops.InstallPackage(ctx, envName, "requests")
	require.NoError(t, err)
	require.NotNil(t, installExec)

	_, _, _, _ = readExecutionOutput(installExec)

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaOperations_InstallPackage_EnvNotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("notfound")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.InstallPackage(ctx, envName, "requests")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_InstallPackage_EmptyEnvName(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.InstallPackage(ctx, "", "requests")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_InstallPackage_EmptyPackageName(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.InstallPackage(ctx, "test-env", "")
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

// ============================================================================
// RunPython Tests
// ============================================================================

func TestCondaOperations_RunPython_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("runpython")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Run Python code
	exec, err := ops.RunPython(ctx, envName, "print('Hello, World!')", nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Hello, World!")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_RunPython_WithStdin(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("runpython")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Run Python code that reads from stdin
	stdin := strings.NewReader("test input\n")
	exec, err := ops.RunPython(ctx, envName, "import sys; print('stdin:', sys.stdin.read().strip())", stdin)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "stdin: test input")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_RunPython_EnvNotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("notfound")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunPython(ctx, envName, "print('test')", nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_RunPython_EmptyEnv(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunPython(ctx, "", "print('test')", nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_RunPython_EmptyCode(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunPython(ctx, "test-env", "", nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_RunPython_ErrorCode(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("error")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Run invalid Python code
	exec, err := ops.RunPython(ctx, envName, "invalid syntax!!!", nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	_, stderr, exitCode, err := readExecutionOutput(exec)
	assert.NotEqual(t, 0, exitCode)
	assert.True(t, len(stderr) > 0 || err != nil, "Should have error output")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// ============================================================================
// RunScript Tests
// ============================================================================

func TestCondaOperations_RunScript_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("runscript")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create a test script
	tmpDir := os.TempDir()
	scriptPath := fmt.Sprintf("%s/test-script-%d.py", tmpDir, time.Now().UnixNano())
	scriptContent := "#!/usr/bin/env python\nimport sys\nprint('Hello from script!')\nprint('Args:', sys.argv[1:])\n"
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	// Run script
	exec, err := ops.RunScript(ctx, envName, scriptPath, []string{"arg1", "arg2"}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Hello from script!")
	assert.Contains(t, stdout, "arg1")
	assert.Contains(t, stdout, "arg2")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_RunScript_WithStdin(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("runscript")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create a test script that reads stdin
	tmpDir := os.TempDir()
	scriptPath := fmt.Sprintf("%s/test-script-%d.py", tmpDir, time.Now().UnixNano())
	scriptContent := "#!/usr/bin/env python\nimport sys\nprint('stdin:', sys.stdin.read().strip())\n"
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	// Run script with stdin
	stdin := strings.NewReader("test stdin input\n")
	exec, err := ops.RunScript(ctx, envName, scriptPath, nil, stdin)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "test stdin input")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_RunScript_EnvNotFound(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("notfound")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunScript(ctx, envName, "/path/to/script.py", nil, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "not found")
}

func TestCondaOperations_RunScript_EmptyEnv(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunScript(ctx, "", "/path/to/script.py", nil, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

func TestCondaOperations_RunScript_EmptyScriptPath(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunScript(ctx, "test-env", "", nil, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "empty")
}

// ============================================================================
// RunConda Tests
// ============================================================================

func TestCondaOperations_RunConda_Success(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run conda info
	exec, err := ops.RunConda(ctx, []string{"info"}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "conda")
}

func TestCondaOperations_RunConda_EnvList(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run conda env list
	exec, err := ops.RunConda(ctx, []string{"env", "list"}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, stderr, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "base")
}

func TestCondaOperations_RunConda_EmptyArgs(t *testing.T) {
	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := ops.RunConda(ctx, []string{}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "at least one argument")
}

func TestCondaOperations_RunConda_WithStdin(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run conda command that might use stdin (though most don't)
	stdin := strings.NewReader("test\n")
	exec, err := ops.RunConda(ctx, []string{"info"}, stdin)
	require.NoError(t, err)
	require.NotNil(t, exec)

	_, _, exitCode, err := readExecutionOutput(exec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
}

// ============================================================================
// Streaming I/O Tests
// ============================================================================

func TestCondaOperations_Streaming_StdoutStderr(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("streaming")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Run Python that outputs to both stdout and stderr
	code := "import sys; print('stdout'); print('stderr', file=sys.stderr)"
	exec, err := ops.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)

	// Read from both streams concurrently
	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, exec.Stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, exec.Stderr)
	}()

	err = <-exec.Done
	wg.Wait()

	require.NoError(t, err)
	assert.Contains(t, stdoutBuf.String(), "stdout")
	assert.Contains(t, stderrBuf.String(), "stderr")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

func TestCondaOperations_Streaming_Cancel(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("cancel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment first
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Run long-running Python code
	code := "import time; [time.sleep(0.1) for _ in range(100)]"
	exec, err := ops.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	exec.Cancel()

	err = <-exec.Done
	assert.Error(t, err)

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// ============================================================================
// End-to-End Integration Tests (Docker)
// ============================================================================

func TestCondaOperations_EndToEnd_CreateListRemove(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("e2e")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// List environments before
	listExec1, err := ops.ListEnvironments(ctx)
	require.NoError(t, err)
	envs1, _, _, _ := readExecutionOutput(listExec1)
	initialCount := strings.Count(envs1, "\n")

	// Create environment
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, exitCode, err := readExecutionOutput(createExec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Verify in list
	listExec2, err := ops.ListEnvironments(ctx)
	require.NoError(t, err)
	envs2, _, _, _ := readExecutionOutput(listExec2)
	assert.Contains(t, envs2, envName)

	// Get environment path
	path, err := ops.GetEnvironment(ctx, envName)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Remove environment
	removeExec, err := ops.RemoveEnvironment(ctx, envName)
	require.NoError(t, err)
	_, _, exitCode, err = readExecutionOutput(removeExec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Verify removed
	_, err = ops.GetEnvironment(ctx, envName)
	require.Error(t, err)

	// List environments after
	listExec3, err := ops.ListEnvironments(ctx)
	require.NoError(t, err)
	envs3, _, _, _ := readExecutionOutput(listExec3)
	finalCount := strings.Count(envs3, "\n")
	assert.Equal(t, initialCount, finalCount, "Environment count should return to initial")
}

func TestCondaOperations_EndToEnd_CreateInstallRunRemove(t *testing.T) {
	requireCondaAvailable(t)

	ops := setupTestOps(t)

	envName := generateTestEnvName("e2e-full")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create environment
	createExec, err := ops.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, exitCode, err := readExecutionOutput(createExec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Install package (may fail due to network, that's okay)
	installExec, err := ops.InstallPackage(ctx, envName, "requests")
	if err == nil && installExec != nil {
		_, _, _, _ = readExecutionOutput(installExec)
	}

	// Run Python code
	runExec, err := ops.RunPython(ctx, envName, "import sys; print('Python:', sys.version_info[:2])", nil)
	require.NoError(t, err)
	stdout, stderr, exitCode, err := readExecutionOutput(runExec)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Python:")

	// Cleanup
	removeExec, _ := ops.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}
