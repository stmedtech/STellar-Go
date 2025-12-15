package service

import (
	"context"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitExecution(t *testing.T, exec *RawExecution) (exitCode int, doneErr error) {
	t.Helper()
	require.NotNil(t, exec)
	doneErr = <-exec.Done
	exitCode = <-exec.ExitCode
	return exitCode, doneErr
}

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

	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{Command: cmd, Args: args})
	require.NoError(t, err)

	out, err := io.ReadAll(exec.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	code, doneErr := waitExecution(t, exec)
	require.NoError(t, doneErr)
	assert.Equal(t, 0, code)
}

func TestRawExecutor_ExecuteCommandWithArgs(t *testing.T) {
	executor := NewRawExecutor()
	ctx := context.Background()

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "a", "b", "c"}
	} else {
		cmd = "echo"
		args = []string{"a", "b", "c"}
	}

	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{Command: cmd, Args: args})
	require.NoError(t, err)

	out, err := io.ReadAll(exec.Stdout)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "a")
	assert.Contains(t, s, "b")
	assert.Contains(t, s, "c")

	code, doneErr := waitExecution(t, exec)
	require.NoError(t, doneErr)
	assert.Equal(t, 0, code)
}

func TestRawExecutor_ExecuteWithStdin_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdin round-trip test uses `cat` for determinism")
	}

	executor := NewRawExecutor()
	ctx := context.Background()

	in := "abc\n"
	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{
		Command: "cat",
		Stdin:   strings.NewReader(in),
	})
	require.NoError(t, err)

	out, err := io.ReadAll(exec.Stdout)
	require.NoError(t, err)
	assert.Equal(t, in, string(out))

	code, doneErr := waitExecution(t, exec)
	require.NoError(t, doneErr)
	assert.Equal(t, 0, code)
}

func TestRawExecutor_EnvAndWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env/working dir test uses sh")
	}

	executor := NewRawExecutor()
	ctx := context.Background()

	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{
		Command:    "sh",
		Args:       []string{"-c", "echo $FOO && pwd"},
		Env:        map[string]string{"FOO": "bar"},
		WorkingDir: "/tmp",
	})
	require.NoError(t, err)

	out, err := io.ReadAll(exec.Stdout)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "bar")
	assert.Contains(t, s, "/tmp")

	code, doneErr := waitExecution(t, exec)
	require.NoError(t, doneErr)
	assert.Equal(t, 0, code)
}

func TestRawExecutor_NonZeroExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-zero exit test uses sh")
	}

	executor := NewRawExecutor()
	ctx := context.Background()

	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	})
	require.NoError(t, err)

	_, _ = io.ReadAll(exec.Stdout)
	code, doneErr := waitExecution(t, exec)
	require.Error(t, doneErr)
	assert.Equal(t, 42, code)
}

func TestRawExecutor_Cancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cancel test uses sh")
	}

	executor := NewRawExecutor()
	ctx := context.Background()

	exec, err := executor.ExecuteRaw(ctx, RawExecutionRequest{
		Command: "sleep",
		Args:    []string{"1000"},
	})
	require.NoError(t, err)

	exec.Cancel()
	code, doneErr := waitExecution(t, exec)
	require.Error(t, doneErr)
	assert.NotEqual(t, 0, code)
}
