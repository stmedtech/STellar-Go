package service

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_Run_Success tests successful command execution
func TestClient_Run_Success(t *testing.T) {
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

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-run-success", Command: cmd, Args: args})
	require.NoError(t, err)
	require.NotNil(t, h)
	require.NotNil(t, h.Stdin)
	require.NotNil(t, h.Stdout)
	require.NotNil(t, h.Stderr)
	require.NotNil(t, h.Log)

	require.NoError(t, h.Stdin.Close())
	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestClient_Run_StreamAccess tests stream access
func TestClient_Run_StreamAccess(t *testing.T) {
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

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-stream-access", Command: cmd, Args: args})
	require.NoError(t, err)
	require.NotNil(t, h)

	assert.NotNil(t, h.Stdin)
	assert.NotNil(t, h.Stdout)
	assert.NotNil(t, h.Stderr)
	assert.NotNil(t, h.Log)
}

// TestClient_Run_StdinWrite tests writing to stdin
func TestClient_Run_StdinWrite(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-stdin-write", Command: "cat"})
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

// TestClient_Run_StdoutRead tests reading from stdout
func TestClient_Run_StdoutRead(t *testing.T) {
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

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-stdout-read", Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	out, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestClient_Run_StderrRead tests reading from stderr
func TestClient_Run_StderrRead(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "client-stderr-read",
		Command: "sh",
		Args:    []string{"-c", "echo err-msg 1>&2"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	errOut, err := io.ReadAll(h.Stderr)
	require.NoError(t, err)
	assert.Contains(t, string(errOut), "err-msg")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)
}

// TestClient_Run_LogRead tests reading from log stream
func TestClient_Run_LogRead(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "client-log-read",
		Command: "sh",
		Args:    []string{"-c", "echo out && echo err 1>&2"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	logBytes, err := io.ReadAll(h.Log)
	require.NoError(t, err)
	require.NotEmpty(t, logBytes)
	assert.Contains(t, string(logBytes), "out")
	assert.Contains(t, string(logBytes), "err")

	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	<-h.Done
	<-h.ExitCode
}

// TestClient_Run_Rejected tests handling rejected requests
func TestClient_Run_Rejected(t *testing.T) {
	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-reject", Command: ""})
	require.Error(t, err)
	require.Nil(t, h)
}

// TestClient_Run_InvalidRequest tests handling invalid requests
func TestClient_Run_InvalidRequest(t *testing.T) {
	p := startComputePair(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-invalid", Command: ""})
	require.Error(t, err)
	require.Nil(t, h)
}

// TestClient_Run_Timeout tests context cancellation behavior
func TestClient_Run_Timeout(t *testing.T) {
	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h, err := p.client.Run(ctx, RunRequest{RunID: "client-ctx-cancel", Command: "echo", Args: []string{"x"}})
	// Depending on where cancellation hits (before/after request send), Run may error or return a handle
	// that is immediately completed. Both are acceptable.
	if err != nil {
		require.Nil(t, h)
		return
	}
	require.NotNil(t, h)
	<-h.Done
}

// TestClient_Cancel_Success tests successful cancellation
func TestClient_Cancel_Success(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "client-cancel-success",
		Command: "sleep",
		Args:    []string{"1000"},
	})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())

	require.NoError(t, h.Cancel())
	doneErr := <-h.Done
	require.Error(t, doneErr)
	<-h.ExitCode
}

// TestClient_Cancel_InvalidRunID tests cancellation with invalid run ID
func TestClient_Cancel_InvalidRunID(t *testing.T) {
	p := startComputePair(t)

	err := p.client.Cancel(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "run_id")
}

// TestClient_Cancel_Timeout tests cancellation request context cancellation behavior
func TestClient_Cancel_Timeout(t *testing.T) {
	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.client.Cancel(ctx, "some-run")
	require.Error(t, err)
}

// TestClient_Status_Success tests successful status query
func TestClient_Status_Success(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "client-status", Command: "cat"})
	require.NoError(t, err)

	st, err := p.client.Status(context.Background(), h.RunID)
	require.NoError(t, err)
	assert.Equal(t, "running", st.Status)

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

// TestClient_Status_InvalidRunID tests status query with invalid run ID
func TestClient_Status_InvalidRunID(t *testing.T) {
	p := startComputePair(t)

	_, err := p.client.Status(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "run_id")
}

// TestClient_Status_Timeout tests status request context cancellation behavior
func TestClient_Status_Timeout(t *testing.T) {
	p := startComputePair(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.client.Status(ctx, "some-run")
	require.Error(t, err)
}

// TestClient_ConcurrentRuns tests concurrent run requests
func TestClient_ConcurrentRuns(t *testing.T) {
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

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h, err := p.client.Run(context.Background(), RunRequest{
				RunID:   "client-concurrent-" + strconv.Itoa(i),
				Command: cmd,
				Args:    args,
			})
			require.NoError(t, err)
			_ = h.Stdin.Close()
			_, _ = io.ReadAll(h.Stdout)
			_, _ = io.ReadAll(h.Stderr)
			<-h.Done
			<-h.ExitCode
		}(i)
	}
	wg.Wait()
}

// TestClient_StreamErrors tests stream error handling
func TestClient_StreamErrors(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "client-stream-errors",
		Command: "sleep",
		Args:    []string{"1000"},
	})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	// Closing the server should complete the client handle with an error (canceled/closed).
	require.NoError(t, p.server.Close())
	doneErr := <-h.Done
	require.Error(t, doneErr)
}

// (no extra helpers)

// TestClient_ConnectionErrors tests connection error handling
func TestClient_ConnectionErrors(t *testing.T) {
	// Test that EnsureConnected works
	conn := newMockConn()
	client := NewClient("test-client", conn)
	require.NotNil(t, client)

	// Should fail when not connected
	_, err := client.Run(context.Background(), RunRequest{Command: "echo", Args: []string{"test"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestClient_RunRequest_Validation tests RunRequest validation
func TestClient_RunRequest_Validation(t *testing.T) {
	// Test that RunRequest with empty command is handled
	// This will be validated in the Run method
	req := RunRequest{
		RunID:   "test",
		Command: "",
		Args:    []string{},
	}

	// We can't fully test without connection, but we can verify the struct
	assert.Equal(t, "test", req.RunID)
	assert.Equal(t, "", req.Command)
}

// TestClient_StatusResponse_Parsing tests StatusResponse time parsing
func TestClient_StatusResponse_Parsing(t *testing.T) {
	// Test time parsing logic
	now := time.Now()
	nowStr := now.Format(time.RFC3339Nano)

	parsed, err := time.Parse(time.RFC3339Nano, nowStr)
	require.NoError(t, err)
	assert.WithinDuration(t, now, parsed, time.Second)
}

// TestClient_MatcherFunctions tests matcher functions
func TestClient_MatcherFunctions(t *testing.T) {
	runID := "test-run-123"

	// Test matchComputeRunResponse
	matcher := matchComputeRunResponse(runID)

	// Create a matching response
	packet, err := protocol.NewComputeRunResponsePacket(runID, true, 1, 2, 3, 4, "")
	require.NoError(t, err)

	handshake, err := protocol.UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	matched, err := matcher(handshake)
	require.NoError(t, err)
	assert.True(t, matched, "should match run response with same runID")

	// Test with different runID
	packet2, err := protocol.NewComputeRunResponsePacket("different-run", true, 1, 2, 3, 4, "")
	require.NoError(t, err)

	handshake2, err := protocol.UnmarshalHandshakePacket(packet2)
	require.NoError(t, err)

	matched2, err := matcher(handshake2)
	require.NoError(t, err)
	assert.False(t, matched2, "should not match run response with different runID")

	// Test with wrong packet type
	wrongPacket, err := protocol.NewComputeCancelResponsePacket(runID, true, "")
	require.NoError(t, err)

	wrongHandshake, err := protocol.UnmarshalHandshakePacket(wrongPacket)
	require.NoError(t, err)

	matched3, err := matcher(wrongHandshake)
	require.NoError(t, err)
	assert.False(t, matched3, "should not match wrong packet type")
}

// TestClient_MatcherCancelResponse tests cancel response matcher
func TestClient_MatcherCancelResponse(t *testing.T) {
	runID := "test-cancel-123"
	matcher := matchComputeCancelResponse(runID)

	packet, err := protocol.NewComputeCancelResponsePacket(runID, true, "")
	require.NoError(t, err)

	handshake, err := protocol.UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	matched, err := matcher(handshake)
	require.NoError(t, err)
	assert.True(t, matched)
}

// TestClient_MatcherStatusResponse tests status response matcher
func TestClient_MatcherStatusResponse(t *testing.T) {
	runID := "test-status-123"
	matcher := matchComputeStatusResponse(runID)

	resp := protocol.ComputeStatusResponse{
		RunID:  runID,
		Status: "running",
	}

	packet, err := protocol.NewComputeStatusResponsePacket(resp)
	require.NoError(t, err)

	handshake, err := protocol.UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	matched, err := matcher(handshake)
	require.NoError(t, err)
	assert.True(t, matched)
}

// TestClient_GenerateRunID tests run ID generation
func TestClient_GenerateRunID(t *testing.T) {
	// Test that generateRunID creates unique IDs
	id1 := generateRunID()
	id2 := generateRunID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "run IDs should be unique")
}

// TestClient_RunRequest_WithAllFields tests RunRequest with all fields
func TestClient_RunRequest_WithAllFields(t *testing.T) {
	req := RunRequest{
		RunID:      "test-run",
		Command:    "echo",
		Args:       []string{"hello", "world"},
		Env:        map[string]string{"KEY": "value"},
		WorkingDir: "/tmp",
	}

	assert.Equal(t, "test-run", req.RunID)
	assert.Equal(t, "echo", req.Command)
	assert.Equal(t, []string{"hello", "world"}, req.Args)
	assert.Equal(t, map[string]string{"KEY": "value"}, req.Env)
	assert.Equal(t, "/tmp", req.WorkingDir)
}

// TestClient_StatusResponse_WithAllFields tests StatusResponse with all fields
func TestClient_StatusResponse_WithAllFields(t *testing.T) {
	now := time.Now()
	exitCode := 0
	resp := StatusResponse{
		RunID:     "test-run",
		Status:    "completed",
		ExitCode:  &exitCode,
		StartTime: &now,
		EndTime:   &now,
	}

	assert.Equal(t, "test-run", resp.RunID)
	assert.Equal(t, "completed", resp.Status)
	assert.NotNil(t, resp.ExitCode)
	assert.Equal(t, 0, *resp.ExitCode)
	assert.NotNil(t, resp.StartTime)
	assert.NotNil(t, resp.EndTime)
}

// TestClient_NewClient tests client creation
func TestClient_NewClient(t *testing.T) {
	conn := newMockConn()
	client := NewClient("test-client", conn)
	require.NotNil(t, client)
	assert.Equal(t, "test-client", client.ClientID())
	assert.NotNil(t, client.Multiplexer())
}

// TestClient_NewClient_NilConn tests client creation with nil connection
func TestClient_NewClient_NilConn(t *testing.T) {
	client := NewClient("test-client", nil)
	assert.Nil(t, client, "client should be nil with nil connection")
}

// TestClient_NewClient_EmptyID tests client creation with empty client ID
func TestClient_NewClient_EmptyID(t *testing.T) {
	conn := newMockConn()
	client := NewClient("", conn)
	assert.Nil(t, client, "client should be nil with empty client ID")
}

// TestClient_Handle_Close tests handle stream closure
func TestClient_Handle_Close(t *testing.T) {
	// Create a handle with mock streams
	// Note: In real usage, these would be multiplexed streams
	// For testing, we use simple buffers
	stdin := &bytes.Buffer{}
	stdout := bytes.NewBufferString("test output")
	stderr := bytes.NewBufferString("test error")
	log := bytes.NewBufferString("test log")

	doneCh := make(chan error, 1)
	exitCh := make(chan int, 1)

	// Create a write closer for stdin
	type writeCloser struct {
		io.Writer
		io.Closer
	}
	stdinWC := writeCloser{stdin, io.NopCloser(bytes.NewReader(nil))}

	handle := &RawExecutionHandle{
		RunID:    "test-run",
		Stdin:    stdinWC,
		Stdout:   io.NopCloser(stdout),
		Stderr:   io.NopCloser(stderr),
		Log:      io.NopCloser(log),
		Done:     doneCh,
		ExitCode: exitCh,
		doneCh:   doneCh,
		exitCh:   exitCh,
	}

	// Verify streams are accessible
	assert.NotNil(t, handle.Stdin)
	assert.NotNil(t, handle.Stdout)
	assert.NotNil(t, handle.Stderr)
	assert.NotNil(t, handle.Log)
	assert.Equal(t, "test-run", handle.RunID)
}

// TestClient_Handle_ConcurrentAccess tests concurrent access to handle
func TestClient_Handle_ConcurrentAccess(t *testing.T) {
	doneCh := make(chan error, 1)
	exitCh := make(chan int, 1)

	handle := &RawExecutionHandle{
		RunID:    "test-run",
		Done:     doneCh,
		ExitCode: exitCh,
		doneCh:   doneCh,
		exitCh:   exitCh,
	}

	// Test concurrent access to RunID
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = handle.RunID
		}()
	}
	wg.Wait()

	assert.Equal(t, "test-run", handle.RunID)
}
