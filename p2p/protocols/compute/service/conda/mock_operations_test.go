//go:build conda

package conda

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Mock vs Real Conda Operations Comparison Tests
// ============================================================================

// TestMockVsReal_ListEnvironments compares mock and real ListEnvironments
func TestMockVsReal_ListEnvironments(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations
	realExec, err := realOps.ListEnvironments(ctx)
	require.NoError(t, err)
	realStdout, _, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode)
	assert.Contains(t, realStdout, "base")

	// Test mock operations
	mockExec, err := mockOps.ListEnvironments(ctx)
	require.NoError(t, err)
	mockStdout, _, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode)
	assert.Contains(t, mockStdout, "base")

	// Both should succeed with exit code 0
	assert.Equal(t, realExitCode, mockExitCode)
}

// TestMockVsReal_GetEnvironment compares mock and real GetEnvironment
func TestMockVsReal_GetEnvironment(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations - get base environment
	realPath, realErr := realOps.GetEnvironment(ctx, "base")
	require.NoError(t, realErr)
	assert.NotEmpty(t, realPath)

	// Test mock operations - get base environment
	mockPath, mockErr := mockOps.GetEnvironment(ctx, "base")
	require.NoError(t, mockErr)
	assert.NotEmpty(t, mockPath)

	// Both should return a path
	assert.NotEmpty(t, realPath)
	assert.NotEmpty(t, mockPath)
}

// TestMockVsReal_GetEnvironment_NotFound compares error handling
func TestMockVsReal_GetEnvironment_NotFound(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envName := "nonexistent-env-" + time.Now().Format("20060102150405")

	// Test real operations
	_, realErr := realOps.GetEnvironment(ctx, envName)
	assert.Error(t, realErr)
	assert.Contains(t, realErr.Error(), "not found")

	// Test mock operations
	_, mockErr := mockOps.GetEnvironment(ctx, envName)
	assert.Error(t, mockErr)
	assert.Contains(t, mockErr.Error(), "not found")

	// Both should return "not found" error
	_ = realErr // Both errors are checked above
	assert.NotNil(t, mockErr)
}

// TestMockVsReal_GetCondaVersion compares mock and real GetCondaVersion
func TestMockVsReal_GetCondaVersion(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations
	realExec, err := realOps.GetCondaVersion(ctx)
	require.NoError(t, err)
	realStdout, _, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode)
	assert.Contains(t, realStdout, "conda")

	// Test mock operations
	mockExec, err := mockOps.GetCondaVersion(ctx)
	require.NoError(t, err)
	mockStdout, _, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode)
	assert.Contains(t, mockStdout, "conda")

	// Both should succeed
	assert.Equal(t, realExitCode, mockExitCode)
}

// TestMockVsReal_CommandPath compares mock and real CommandPath
func TestMockVsReal_CommandPath(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations
	realPath, realErr := realOps.CommandPath(ctx)
	require.NoError(t, realErr)
	assert.NotEmpty(t, realPath)
	assert.Contains(t, strings.ToLower(realPath), "conda")

	// Test mock operations
	mockPath, mockErr := mockOps.CommandPath(ctx)
	require.NoError(t, mockErr)
	assert.NotEmpty(t, mockPath)
	assert.Contains(t, strings.ToLower(mockPath), "conda")

	// Both should return a path containing "conda"
	assert.NotEmpty(t, realPath)
	assert.NotEmpty(t, mockPath)
}

// TestMockVsReal_RunConda_EnvList compares mock and real RunConda with env list
func TestMockVsReal_RunConda_EnvList(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations
	realExec, err := realOps.RunConda(ctx, []string{"env", "list"}, nil)
	require.NoError(t, err)
	realStdout, _, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode)
	assert.Contains(t, realStdout, "base")

	// Test mock operations
	mockExec, err := mockOps.RunConda(ctx, []string{"env", "list"}, nil)
	require.NoError(t, err)
	mockStdout, _, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode)
	assert.Contains(t, mockStdout, "base")

	// Both should succeed and contain "base"
	assert.Equal(t, realExitCode, mockExitCode)
}

// TestMockVsReal_CreateEnvironment compares mock and real CreateEnvironment
func TestMockVsReal_CreateEnvironment(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	envName := generateTestEnvName("mock-vs-real")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test real operations
	realExec, err := realOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	realStdout, realStderr, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode, "real create failed: stderr=%s, stdout=%s", realStderr, realStdout)

	// Verify real environment was created
	realPath, err := realOps.GetEnvironment(ctx, envName)
	require.NoError(t, err)
	assert.NotEmpty(t, realPath)

	// Test mock operations
	mockExec, err := mockOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	mockStdout, mockStderr, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode, "mock create failed: stderr=%s, stdout=%s", mockStderr, mockStdout)

	// Verify mock environment was created
	mockPath, err := mockOps.GetEnvironment(ctx, envName)
	require.NoError(t, err)
	assert.NotEmpty(t, mockPath)

	// Both should succeed
	assert.Equal(t, realExitCode, mockExitCode)

	// Cleanup real environment
	removeExec, _ := realOps.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// TestMockVsReal_RunPython compares mock and real RunPython
func TestMockVsReal_RunPython(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	envName := generateTestEnvName("mock-vs-real-python")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create real environment
	createExec, err := realOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create mock environment
	mockCreateExec, err := mockOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(mockCreateExec)

	code := "print('Hello, World!')"

	// Test real operations
	realExec, err := realOps.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)
	realStdout, realStderr, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode, "real run-python failed: stderr=%s", realStderr)
	assert.Contains(t, realStdout, "Hello, World!")

	// Test mock operations
	mockExec, err := mockOps.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)
	mockStdout, mockStderr, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode, "mock run-python failed: stderr=%s", mockStderr)
	assert.Contains(t, mockStdout, "Hello, World!")

	// Both should succeed
	assert.Equal(t, realExitCode, mockExitCode)

	// Cleanup
	removeExec, _ := realOps.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// TestMockVsReal_RunPython_WithStdin compares mock and real RunPython with stdin
func TestMockVsReal_RunPython_WithStdin(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	envName := generateTestEnvName("mock-vs-real-stdin")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create real environment
	createExec, err := realOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create mock environment
	mockCreateExec, err := mockOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(mockCreateExec)

	code := "import sys; print('stdin:', sys.stdin.read().strip())"
	stdin := strings.NewReader("test input\n")

	// Test real operations
	realExec, err := realOps.RunPython(ctx, envName, code, stdin)
	require.NoError(t, err)
	realStdout, realStderr, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode, "real run-python with stdin failed: stderr=%s", realStderr)
	assert.Contains(t, realStdout, "test input")

	// Test mock operations
	mockStdin := strings.NewReader("test input\n")
	mockExec, err := mockOps.RunPython(ctx, envName, code, mockStdin)
	require.NoError(t, err)
	mockStdout, mockStderr, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode, "mock run-python with stdin failed: stderr=%s", mockStderr)
	// Mock should also handle stdin (though it may not process it the same way)
	assert.NotEmpty(t, mockStdout)

	// Both should succeed
	assert.Equal(t, realExitCode, mockExitCode)

	// Cleanup
	removeExec, _ := realOps.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// TestMockVsReal_RunPython_ErrorCode compares error handling
func TestMockVsReal_RunPython_ErrorCode(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	envName := generateTestEnvName("mock-vs-real-error")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create real environment
	createExec, err := realOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create mock environment
	mockCreateExec, err := mockOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(mockCreateExec)

	code := "import sys; sys.exit(42)"

	// Test real operations
	realExec, err := realOps.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)
	_, realStderr, realExitCode, _ := readExecutionOutput(realExec)
	// Real may or may not have error, but exit code should be non-zero
	assert.NotEqual(t, 0, realExitCode, "real should fail with non-zero exit code: stderr=%s", realStderr)

	// Test mock operations
	mockExec, err := mockOps.RunPython(ctx, envName, code, nil)
	require.NoError(t, err)
	_, mockStderr, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.NotEqual(t, 0, mockExitCode, "mock should fail with non-zero exit code: stderr=%s", mockStderr)

	// Both should have non-zero exit code
	assert.NotEqual(t, 0, realExitCode)
	assert.NotEqual(t, 0, mockExitCode)

	// Cleanup
	removeExec, _ := realOps.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// TestMockVsReal_RunScript compares mock and real RunScript
func TestMockVsReal_RunScript(t *testing.T) {
	realOps, err := NewCondaOperations("")
	require.NoError(t, err)

	mockOps := NewMockCondaOperations()

	envName := generateTestEnvName("mock-vs-real-script")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create real environment
	createExec, err := realOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(createExec)

	// Create mock environment
	mockCreateExec, err := mockOps.CreateEnvironment(ctx, envName, "3.9")
	require.NoError(t, err)
	_, _, _, _ = readExecutionOutput(mockCreateExec)

	// Create test script
	scriptPath := "/tmp/test-script-mock-vs-real.py"
	scriptContent := "#!/usr/bin/env python\nimport sys\nprint('Script args:', sys.argv[1:])\n"
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	args := []string{"arg1", "arg2"}

	// Test real operations
	realExec, err := realOps.RunScript(ctx, envName, scriptPath, args, nil)
	require.NoError(t, err)
	realStdout, realStderr, realExitCode, realErr := readExecutionOutput(realExec)
	require.NoError(t, realErr)
	assert.Equal(t, 0, realExitCode, "real run-script failed: stderr=%s", realStderr)
	assert.Contains(t, realStdout, "arg1")
	assert.Contains(t, realStdout, "arg2")

	// Test mock operations
	mockExec, err := mockOps.RunScript(ctx, envName, scriptPath, args, nil)
	require.NoError(t, err)
	mockStdout, mockStderr, mockExitCode, mockErr := readExecutionOutput(mockExec)
	require.NoError(t, mockErr)
	assert.Equal(t, 0, mockExitCode, "mock run-script failed: stderr=%s", mockStderr)
	assert.Contains(t, mockStdout, "arg1")
	assert.Contains(t, mockStdout, "arg2")

	// Both should succeed and contain args
	assert.Equal(t, realExitCode, mockExitCode)

	// Cleanup
	removeExec, _ := realOps.RemoveEnvironment(ctx, envName)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec)
	}
}

// Note: readExecutionOutput is defined in operations_comprehensive_test.go
// We reuse it here for consistency
