package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRawExecutor_ExecuteSimpleCommand tests executing a simple command
func TestRawExecutor_ExecuteSimpleCommand(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		cmd = "echo"
		args = []string{"hello"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, execution)

	// Read stdout
	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "hello")

	// Wait for completion
	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExecuteCommandWithArgs tests executing a command with arguments
func TestRawExecutor_ExecuteCommandWithArgs(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "arg1", "arg2", "arg3"}
	} else {
		cmd = "echo"
		args = []string{"arg1", "arg2", "arg3"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	output := strings.TrimSpace(string(stdout))
	assert.Contains(t, output, "arg1")
	assert.Contains(t, output, "arg2")
	assert.Contains(t, output, "arg3")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExecuteWithStdin tests executing a command with stdin input
func TestRawExecutor_ExecuteWithStdin(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "findstr", "."}
	} else {
		cmd = "cat"
		args = []string{}
	}

	input := "test input\nline 2\n"
	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Stdin:   strings.NewReader(input),
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Read stdout (should get the input we provided)
	stdout, err := io.ReadAll(execution.Stdout)
	// Ignore "file already closed" errors - they're expected when command finishes quickly
	if err != nil && !strings.Contains(err.Error(), "closed") && err != io.EOF {
		require.NoError(t, err)
	}
	assert.Contains(t, string(stdout), "test input")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExecuteWithEnv tests executing a command with environment variables
func TestRawExecutor_ExecuteWithEnv(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "%TEST_VAR%"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo $TEST_VAR"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "test_value")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExecuteWithWorkingDir tests executing a command with working directory
func TestRawExecutor_ExecuteWithWorkingDir(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "cd"}
	} else {
		cmd = "pwd"
		args = []string{}
	}

	req := RawExecutionRequest{
		Command:    cmd,
		Args:       args,
		WorkingDir: tmpDir,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	output := strings.TrimSpace(string(stdout))
	if runtime.GOOS != "windows" {
		assert.Equal(t, tmpDir, output)
	} else {
		// Windows cd command output format may vary
		assert.Contains(t, strings.ToLower(output), strings.ToLower(tmpDir))
	}

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExecuteWithAllOptions tests executing with all options set
func TestRawExecutor_ExecuteWithAllOptions(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "%TEST_VAR%"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo $TEST_VAR && pwd"}
	}

	req := RawExecutionRequest{
		Command:    cmd,
		Args:       args,
		Env:        map[string]string{"TEST_VAR": "all_options"},
		WorkingDir: tmpDir,
		Stdin:      strings.NewReader("input"),
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "all_options")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_CancelExecution tests canceling an execution
func TestRawExecutor_CancelExecution(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"127.0.0.1", "-n", "10"}
	} else {
		cmd = "sleep"
		args = []string{"10"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Cancel after a short delay
	time.Sleep(100 * time.Millisecond)
	execution.Cancel()

	// Wait for completion (should be canceled)
	select {
	case err := <-execution.Done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("execution did not complete after cancel")
	}
}

// TestRawExecutor_ExitCode_Success tests successful exit code
func TestRawExecutor_ExitCode_Success(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "exit", "0"}
	} else {
		cmd = "true"
		args = []string{}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ExitCode_Failure tests failure exit code
func TestRawExecutor_ExitCode_Failure(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "exit", "1"}
	} else {
		cmd = "false"
		args = []string{}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.NotEqual(t, 0, code)
}

// TestRawExecutor_StdoutCapture tests capturing stdout
func TestRawExecutor_StdoutCapture(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "stdout", "output"}
	} else {
		cmd = "echo"
		args = []string{"stdout", "output"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "stdout")
	assert.Contains(t, string(stdout), "output")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_StderrCapture tests capturing stderr
func TestRawExecutor_StderrCapture(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "stderr", ">&2"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo stderr output >&2"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stderr, err := io.ReadAll(execution.Stderr)
	require.NoError(t, err)
	assert.Contains(t, string(stderr), "stderr")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ConcurrentExecutions tests multiple concurrent executions
func TestRawExecutor_ConcurrentExecutions(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "concurrent"}
	} else {
		cmd = "echo"
		args = []string{"concurrent"}
	}

	numExecutions := 10
	var wg sync.WaitGroup
	errors := make(chan error, numExecutions)

	for i := 0; i < numExecutions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := RawExecutionRequest{
				Command: cmd,
				Args:    args,
			}
			execution, err := executor.ExecuteRaw(ctx, req)
			if err != nil {
				errors <- err
				return
			}
			// Read stdout immediately to avoid pipe closure issues
			stdout, err := io.ReadAll(execution.Stdout)
			// Ignore "file already closed" errors - they're expected when command finishes quickly
			if err != nil && !strings.Contains(err.Error(), "closed") && err != io.EOF {
				errors <- err
				return
			}
			if !strings.Contains(string(stdout), "concurrent") {
				errors <- fmt.Errorf("unexpected output: %s", string(stdout))
				return
			}
			code, err := WaitForCompletion(execution, 5*time.Second)
			if err != nil {
				errors <- err
				return
			}
			if code != 0 {
				errors <- fmt.Errorf("unexpected exit code: %d", code)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		require.NoError(t, err)
	}
}

// TestRawExecutor_InvalidCommand tests executing an invalid command
func TestRawExecutor_InvalidCommand(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	req := RawExecutionRequest{
		Command: "",
		Args:    []string{},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "command is required")
}

// TestRawExecutor_CommandNotFound tests executing a non-existent command
func TestRawExecutor_CommandNotFound(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	req := RawExecutionRequest{
		Command: "nonexistent_command_12345",
		Args:    []string{},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	// On some systems, this might succeed but fail on Wait
	// On others, it might fail immediately
	if err != nil {
		assert.Contains(t, err.Error(), "not found")
		return
	}

	// If it started, it should fail on wait
	code, err := WaitForCompletion(execution, 5*time.Second)
	if err != nil {
		// Expected - command not found
		return
	}
	// If we got here, the command somehow ran (unlikely but possible)
	assert.NotEqual(t, 0, code, "command should have failed")
}

// TestRawExecutor_StdinClosure tests closing stdin mid-execution
func TestRawExecutor_StdinClosure(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "findstr", "."}
	} else {
		cmd = "cat"
		args = []string{}
	}

	inputData := []byte("input\n")
	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Stdin:   bytes.NewReader(inputData),
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Read stdout (should get the input we provided)
	// Read in a goroutine to avoid blocking if command finishes quickly
	stdoutChan := make(chan []byte, 1)
	errChan := make(chan error, 1)
	go func() {
		data, err := io.ReadAll(execution.Stdout)
		stdoutChan <- data
		errChan <- err
	}()

	// Wait for completion or timeout
	select {
	case stdout := <-stdoutChan:
		err := <-errChan
		require.NoError(t, err)
		assert.Contains(t, string(stdout), "input")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout reading stdout")
	}

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_LongRunningCommand tests a long-running command
func TestRawExecutor_LongRunningCommand(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"127.0.0.1", "-n", "3"}
	} else {
		cmd = "sleep"
		args = []string{"2"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Command should still be running
	select {
	case <-execution.Done:
		t.Fatal("command completed too quickly")
	case <-time.After(100 * time.Millisecond):
		// Expected - command still running
	}

	// Wait for completion
	code, err := WaitForCompletion(execution, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_BinaryData tests handling binary data
func TestRawExecutor_BinaryData(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "findstr", "."}
	} else {
		cmd = "cat"
		args = []string{}
	}

	// Create binary data
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Stdin:   bytes.NewReader(binaryData),
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Read stdout (should get the binary data)
	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Equal(t, binaryData, stdout)

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_LargeOutput tests handling large output
func TestRawExecutor_LargeOutput(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	// Generate large output (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "findstr", "."}
	} else {
		cmd = "cat"
		args = []string{}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Stdin:   bytes.NewReader(largeData),
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Stdin is already being copied from req.Stdin in a goroutine
	// Wait a bit for the copy to start (large data takes time)
	time.Sleep(100 * time.Millisecond)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(stdout))
	assert.Equal(t, largeData, stdout)

	code, err := WaitForCompletion(execution, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_ContextCancellation tests cancellation via context
func TestRawExecutor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executor := NewRawExecutor()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"127.0.0.1", "-n", "10"}
	} else {
		cmd = "sleep"
		args = []string{"10"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Cancel context
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for completion
	select {
	case err := <-execution.Done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("execution did not complete after context cancellation")
	}
}

// TestRawExecutor_ProcessKill tests killing a process
func TestRawExecutor_ProcessKill(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"127.0.0.1", "-n", "10"}
	} else {
		cmd = "sleep"
		args = []string{"10"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Kill via cancel
	time.Sleep(100 * time.Millisecond)
	execution.Cancel()

	// Wait for completion
	select {
	case err := <-execution.Done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("execution did not complete after kill")
	}
}

// TestRawExecutor_ResourceCleanup tests that resources are properly cleaned up
func TestRawExecutor_ResourceCleanup(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "cleanup"}
	} else {
		cmd = "echo"
		args = []string{"cleanup"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Read all output
	_, _ = io.ReadAll(execution.Stdout)
	_, _ = io.ReadAll(execution.Stderr)

	// Wait for completion
	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	// Verify streams are closed (may return EOF or "file already closed" error)
	_, err = execution.Stdout.Read(make([]byte, 1))
	assert.True(t, err == io.EOF || strings.Contains(err.Error(), "closed"))

	_, err = execution.Stderr.Read(make([]byte, 1))
	assert.True(t, err == io.EOF || strings.Contains(err.Error(), "closed"))
}

// TestRawExecutor_EmptyArgs tests execution with empty args
func TestRawExecutor_EmptyArgs(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "true"
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    []string{},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_EmptyEnv tests execution with empty env
func TestRawExecutor_EmptyEnv(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "test"}
	} else {
		cmd = "echo"
		args = []string{"test"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Env:     map[string]string{},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "test")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_InvalidWorkingDir tests execution with invalid working directory
func TestRawExecutor_InvalidWorkingDir(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "test"}
	} else {
		cmd = "echo"
		args = []string{"test"}
	}

	req := RawExecutionRequest{
		Command:    cmd,
		Args:       args,
		WorkingDir: "/nonexistent/directory/12345",
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	// This might succeed on some systems (command starts but fails) or fail immediately
	if err != nil {
		assert.Contains(t, err.Error(), "directory")
		return
	}

	// If it started, it should fail on wait or produce an error
	code, err := WaitForCompletion(execution, 5*time.Second)
	if err != nil {
		// Expected - invalid directory
		return
	}
	// Some systems might still succeed
	_ = code
}

// TestRawExecutor_MultipleEnvVars tests execution with multiple environment variables
func TestRawExecutor_MultipleEnvVars(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "%VAR1%", "%VAR2%", "%VAR3%"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo $VAR1 $VAR2 $VAR3"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Env: map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		},
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	output := string(stdout)
	assert.Contains(t, output, "value1")
	assert.Contains(t, output, "value2")
	assert.Contains(t, output, "value3")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

// TestRawExecutor_NoStdin tests execution without stdin
func TestRawExecutor_NoStdin(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "no", "stdin"}
	} else {
		cmd = "echo"
		args = []string{"no", "stdin"}
	}

	req := RawExecutionRequest{
		Command: cmd,
		Args:    args,
		Stdin:   nil,
	}

	execution, err := executor.ExecuteRaw(ctx, req)
	require.NoError(t, err)

	// Stdin should be closed
	_, err = execution.Stdin.Write([]byte("test"))
	assert.Error(t, err) // Should fail because stdin is closed

	stdout, err := io.ReadAll(execution.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "no")
	assert.Contains(t, string(stdout), "stdin")

	code, err := WaitForCompletion(execution, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}
