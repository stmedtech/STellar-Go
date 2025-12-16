package service

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase-5 integration tests: deterministic by design.
//
// Rules:
// - No sleeps/timeouts inside test code.
// - Only `go test -timeout ...` is used as the global safety net.
// - Tests must be event-driven: completion is signaled via control-plane status events.

type computePair struct {
	client *Client
	server *Server
	cancel context.CancelFunc
}

func startComputePair(t *testing.T) *computePair {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	executor := NewRawExecutor()

	server := NewServer(serverConn, executor)
	require.NotNil(t, server)

	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	acceptErr := make(chan error, 1)
	go func() { acceptErr <- server.Accept() }()

	// net.Pipe is synchronous: Connect() will block until Accept() reads hello.
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

func requireNonWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("not supported on windows")
		}
	}

type logEntry struct {
	RunID string `json:"run_id"`
	Type  string `json:"type"`
	Data  string `json:"data"`
	Time  string `json:"time"`
}

func parseLogEntries(t *testing.T, b []byte) []logEntry {
	t.Helper()
	s := bufio.NewScanner(strings.NewReader(string(b)))
	entries := make([]logEntry, 0)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var e logEntry
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		require.NotEmpty(t, e.RunID)
		require.NotEmpty(t, e.Type)
		// Data may be empty (e.g., empty output); don't require it.
		require.NotEmpty(t, e.Time)
		_, err := time.Parse(time.RFC3339Nano, e.Time)
		require.NoError(t, err)
		entries = append(entries, e)
	}
	require.NoError(t, s.Err())
	return entries
}

func TestIntegration_Run_EchoStdout(t *testing.T) {
	p := startComputePair(t)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		cmd = "echo"
		args = []string{"hello"}
	}

	h, err := p.client.Run(context.Background(), RunRequest{Command: cmd, Args: args})
	require.NoError(t, err)
	require.NotNil(t, h)
	_ = h.Stdin.Close()

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	// Deterministic completion (event-driven)
	err = <-h.Done
	require.NoError(t, err)
	code := <-h.ExitCode
	assert.Equal(t, 0, code)
}

func TestIntegration_Run_WithArgs(t *testing.T) {
	p := startComputePair(t)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "a", "b", "c"}
	} else {
		cmd = "echo"
		args = []string{"a", "b", "c"}
	}

	h, err := p.client.Run(context.Background(), RunRequest{Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "a")
	assert.Contains(t, s, "b")
	assert.Contains(t, s, "c")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_WithEnvAndWorkingDir(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		Command:    "sh",
		Args:       []string{"-c", "echo $FOO && pwd"},
		Env:        map[string]string{"FOO": "bar"},
		WorkingDir: "/tmp",
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "bar")
	assert.Contains(t, s, "/tmp")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_NonZeroExit(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	doneErr := <-h.Done
	require.Error(t, doneErr)
	code := <-h.ExitCode
	assert.Equal(t, 42, code)
}

func TestIntegration_Run_EmptyOutput(t *testing.T) {
	p := startComputePair(t)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "exit", "0"}
	} else {
		cmd = "true"
		args = nil
	}

	h, err := p.client.Run(context.Background(), RunRequest{Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	require.Len(t, out, 0)

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_UnicodeRoundTrip(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{Command: "cat"})
	require.NoError(t, err)

	in := "Hello 世界\n"
	_, err = h.Stdin.Write([]byte(in))
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Equal(t, in, string(out))

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_BinaryRoundTrip(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{Command: "cat"})
	require.NoError(t, err)

	in := []byte{0x00, 0x01, 0x02, 0xFF, 0x0A}
	_, err = h.Stdin.Write(in)
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Equal(t, in, out)

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_DuplicateRunIDRejected(t *testing.T) {
	p := startComputePair(t)

	runID := "dup-run"

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "x"}
	} else {
		cmd = "echo"
		args = []string{"x"}
	}

	h1, err := p.client.Run(context.Background(), RunRequest{RunID: runID, Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h1.Stdin.Close())

	h2, err := p.client.Run(context.Background(), RunRequest{RunID: runID, Command: cmd, Args: args})
	require.Error(t, err)
	require.Nil(t, h2)
}

func TestIntegration_LogStream_EmitsJSONLines(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	runID := "log-json"
	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   runID,
		Command: "sh",
		Args:    []string{"-c", "echo out && echo err 1>&2"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	// Log stream should close deterministically after stdout/stderr forwarding completes.
	logBytes, err := io.ReadAll(h.Log)
	require.NoError(t, err)
	require.NotEmpty(t, logBytes)
	entries := parseLogEntries(t, logBytes)
	// Validate schema + that we got both stdout and stderr entries with the right run id.
	seenStdout := false
	seenStderr := false
	for _, e := range entries {
		assert.Equal(t, runID, e.RunID)
		if e.Type == "stdout" {
			seenStdout = true
			assert.Contains(t, e.Data, "out")
		}
		if e.Type == "stderr" {
			seenStderr = true
			assert.Contains(t, e.Data, "err")
		}
	}
	assert.True(t, seenStdout, "expected at least one stdout log entry")
	assert.True(t, seenStderr, "expected at least one stderr log entry")

	// Ensure command completion is signaled.
	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	<-h.Done
	<-h.ExitCode
}

func TestIntegration_Run_EmptyCommandRejected(t *testing.T) {
	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "empty-cmd", Command: ""})
	require.Error(t, err)
	require.Nil(t, h)
	assert.Contains(t, strings.ToLower(err.Error()), "command")
}

func TestIntegration_Run_CommandNotFoundRejected(t *testing.T) {
	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "not-found", Command: "definitely-not-a-real-command-xyz"})
	require.Error(t, err)
	require.Nil(t, h)
}

func TestIntegration_Status_InvalidRunID(t *testing.T) {
	p := startComputePair(t)

	_, err := p.client.Status(context.Background(), "no-such-run")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "not found")
}

func TestIntegration_Cancel_InvalidRunID(t *testing.T) {
	p := startComputePair(t)

	err := p.client.Cancel(context.Background(), "no-such-run")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "not found")
}

func TestIntegration_Status_WhileRunning_AndAfterCompletion(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "status-running", Command: "cat"})
	require.NoError(t, err)

	// Keep stdin open so the process remains running.
	st, err := p.client.Status(context.Background(), h.RunID)
	require.NoError(t, err)
	assert.Equal(t, "running", st.Status)

	// Now finish the process.
	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)

	st2, err := p.client.Status(context.Background(), h.RunID)
	require.NoError(t, err)
	assert.Equal(t, "completed", st2.Status)
	require.NotNil(t, st2.ExitCode)
	assert.Equal(t, 0, *st2.ExitCode)
}

func TestIntegration_Cancel_AfterCompletionRejected(t *testing.T) {
	p := startComputePair(t)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "done"}
	} else {
		cmd = "echo"
		args = []string{"done"}
	}

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "cancel-after", Command: cmd, Args: args})
	require.NoError(t, err)
	_ = h.Stdin.Close()
	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	require.NoError(t, <-h.Done)
	<-h.ExitCode

	err = h.Cancel()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "not running")
}

func TestIntegration_StreamClosure_StdoutEOF(t *testing.T) {
	p := startComputePair(t)

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "x"}
	} else {
		cmd = "echo"
		args = []string{"x"}
	}

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "stdout-eof", Command: cmd, Args: args})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	<-h.Done
	<-h.ExitCode

	buf := make([]byte, 1)
	n, rerr := h.Stdout.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, rerr)
}

func TestIntegration_StdoutStderr_InterleavingAndLogs(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "interleave",
		Command: "sh",
		Args:    []string{"-c", "for i in 1 2 3; do echo o$i; echo e$i 1>&2; done"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	logBytes, err := io.ReadAll(h.Log)
	require.NoError(t, err)
	entries := parseLogEntries(t, logBytes)
	require.NotEmpty(t, entries)
	var logStdout, logStderr strings.Builder
	for _, e := range entries {
		if e.Type == "stdout" {
			logStdout.WriteString(e.Data)
		}
		if e.Type == "stderr" {
			logStderr.WriteString(e.Data)
		}
	}
	lsOut := logStdout.String()
	lsErr := logStderr.String()
	assert.Contains(t, lsOut, "o1")
	assert.Contains(t, lsOut, "o2")
	assert.Contains(t, lsOut, "o3")
	assert.Contains(t, lsErr, "e1")
	assert.Contains(t, lsErr, "e2")
	assert.Contains(t, lsErr, "e3")

	out, _ := io.ReadAll(h.Stdout)
	errOut, _ := io.ReadAll(h.Stderr)
	sOut := string(out)
	sErr := string(errOut)
	assert.Contains(t, sOut, "o1")
	assert.Contains(t, sOut, "o2")
	assert.Contains(t, sOut, "o3")
	assert.Contains(t, sErr, "e1")
	assert.Contains(t, sErr, "e2")
	assert.Contains(t, sErr, "e3")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
		}

func TestIntegration_StdinWriteAfterCloseFails(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "stdin-close", Command: "cat"})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	_, werr := h.Stdin.Write([]byte("x"))
	require.Error(t, werr)
}

func TestIntegration_Run_StdinToStdout(t *testing.T) {
	p := startComputePair(t)

	// Use cat on unix; windows variant skipped for determinism.
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{Command: "cat"})
	require.NoError(t, err)

	_, err = h.Stdin.Write([]byte("abc\n"))
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "abc\n")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Run_Stderr(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		Command: "sh",
		Args:    []string{"-c", "echo err-msg 1>&2"},
	})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	errOut, err := io.ReadAll(h.Stderr)
	require.NoError(t, err)
	assert.Contains(t, string(errOut), "err-msg")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

func TestIntegration_Cancel_LongRunning(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		Command: "sh",
		Args:    []string{"-c", "sleep 1000"},
	})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	// Cancel immediately; completion is deterministic via status event.
	require.NoError(t, h.Cancel())

	doneErr := <-h.Done
	require.Error(t, doneErr)
	assert.Contains(t, strings.ToLower(doneErr.Error()), "canceled")
	<-h.ExitCode
}

func TestIntegration_ServerShutdown_CompletesClientHandles(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		Command: "sh",
		Args:    []string{"-c", "sleep 1000"},
	})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	// Closing server should eventually surface as connection closure on client side.
	require.NoError(t, p.server.Close())

	doneErr := <-h.Done
	require.Error(t, doneErr)
	// Could be "canceled" (server actively cancels runs) or "closed" (connection closed first)
	msg := strings.ToLower(doneErr.Error())
	assert.True(t, strings.Contains(msg, "canceled") || strings.Contains(msg, "closed"))
}

func TestIntegration_LargeOutput_Deterministic(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	// 256KiB deterministic output
	h, err := p.client.Run(context.Background(), RunRequest{
		Command: "head",
		Args:    []string{"-c", "262144", "/dev/zero"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	require.Equal(t, 262144, len(out))

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}
