package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"

	"stellar/p2p/protocols/common/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConn is a simple in-memory connection for testing
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
	mu       sync.Mutex
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  &bytes.Buffer{},
		writeBuf: &bytes.Buffer{},
	}
}

func (m *mockConn) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(p)
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// setupTestServer creates a server with a mock connection
func setupTestServer(t *testing.T) (*Server, *mockConn, func()) {
	conn := newMockConn()
	executor := NewRawExecutor()
	server := NewServer(conn, executor)
	require.NotNil(t, server)

	cleanup := func() {
		server.Close()
		conn.Close()
	}

	return server, conn, cleanup
}

// TestServer_HandleRun_Success tests successful command execution
func TestServer_HandleRun_Success(t *testing.T) {
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

	h, err := p.client.Run(context.Background(), RunRequest{RunID: "server-run-success", Command: cmd, Args: args})
	require.NoError(t, err)
	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)
	_, _ = io.ReadAll(h.Stderr)
	require.NoError(t, <-h.Done)
	<-h.ExitCode
}

// TestServer_HandleRun_InvalidRequest tests handling invalid requests
func TestServer_HandleRun_InvalidRequest(t *testing.T) {
	executor := NewRawExecutor()
	conn := newMockConn()
	server := NewServer(conn, executor)
	require.NotNil(t, server)

	// Test with invalid JSON - this will fail during unmarshaling
	invalidPacket := &protocol.HandshakePacket{
		Type:    protocol.HandshakeTypeComputeRun,
		Payload: json.RawMessage(`invalid json`),
	}

	ctx := context.Background()
	err := server.handleRun(ctx, invalidPacket)
	// In this service, handlers write an error response back on the control stream and return nil
	// so the server loop can continue serving.
	assert.NoError(t, err)
}

// TestServer_HandleRun_EmptyCommand tests handling empty command
func TestServer_HandleRun_EmptyCommand(t *testing.T) {
	p := startComputePair(t)
	h, err := p.client.Run(context.Background(), RunRequest{RunID: "server-empty-cmd", Command: ""})
	require.Error(t, err)
	require.Nil(t, h)
}

// TestServer_HandleRun_DuplicateRunID tests handling duplicate run IDs
func TestServer_HandleRun_DuplicateRunID(t *testing.T) {
	p := startComputePair(t)
	runID := "server-dup"

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
	_ = h1.Stdin.Close()

	h2, err := p.client.Run(context.Background(), RunRequest{RunID: runID, Command: cmd, Args: args})
	require.Error(t, err)
	require.Nil(t, h2)
}

// TestServer_HandleCancel_Success tests successful cancellation
func TestServer_HandleCancel_Success(t *testing.T) {
	p := startComputePair(t)
	requireNonWindows(t)

	h, err := p.client.Run(context.Background(), RunRequest{
		RunID:   "server-cancel-success",
		Command: "sleep",
		Args:    []string{"1000"},
	})
	require.NoError(t, err)
	_ = h.Stdin.Close()

	require.NoError(t, h.Cancel())
	doneErr := <-h.Done
	require.Error(t, doneErr)
	<-h.ExitCode
}

// TestServer_HandleCancel_InvalidRunID tests cancellation with invalid run ID
func TestServer_HandleCancel_InvalidRunID(t *testing.T) {
	p := startComputePair(t)
	err := p.client.Cancel(context.Background(), "no-such-run")
	require.Error(t, err)
}

// TestServer_HandleCancel_EmptyRunID tests cancellation with empty run ID
func TestServer_HandleCancel_EmptyRunID(t *testing.T) {
	p := startComputePair(t)
	err := p.client.Cancel(context.Background(), "")
	require.Error(t, err)
}

// TestServer_HandleStatus_InvalidRunID tests status query with invalid run ID
func TestServer_HandleStatus_InvalidRunID(t *testing.T) {
	p := startComputePair(t)
	_, err := p.client.Status(context.Background(), "no-such-run")
	require.Error(t, err)
}

// TestServer_HandleStatus_EmptyRunID tests status query with empty run ID
func TestServer_HandleStatus_EmptyRunID(t *testing.T) {
	p := startComputePair(t)
	_, err := p.client.Status(context.Background(), "")
	require.Error(t, err)
}

// TestServer_RunCleanup tests run cleanup
func TestServer_RunCleanup(t *testing.T) {
	executor := NewRawExecutor()
	conn := newMockConn()
	server := NewServer(conn, executor)
	require.NotNil(t, server)

	runID := "test-cleanup"
	server.runsMu.Lock()
	server.runs[runID] = &RunInfo{
		RunID:  runID,
		Status: RunStatusRunning,
	}
	server.runsMu.Unlock()

	server.cleanupRun(runID)

	server.runsMu.RLock()
	_, exists := server.runs[runID]
	server.runsMu.RUnlock()

	assert.False(t, exists, "run should be cleaned up")
}

// TestServer_ConcurrentRuns tests concurrent run handling
func TestServer_ConcurrentRuns(t *testing.T) {
	executor := NewRawExecutor()
	conn := newMockConn()
	server := NewServer(conn, executor)
	require.NotNil(t, server)

	// Test that runs map is thread-safe
	var wg sync.WaitGroup
	numRuns := 10

	for i := 0; i < numRuns; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runID := fmt.Sprintf("run-%d", id)
			server.runsMu.Lock()
			server.runs[runID] = &RunInfo{
				RunID:  runID,
				Status: RunStatusRunning,
			}
			server.runsMu.Unlock()

			// Verify it was added
			server.runsMu.RLock()
			_, exists := server.runs[runID]
			server.runsMu.RUnlock()
			assert.True(t, exists, "run should exist")
		}(i)
	}

	wg.Wait()

	server.runsMu.RLock()
	assert.Equal(t, numRuns, len(server.runs), "all runs should be stored")
	server.runsMu.RUnlock()
}

// NOTE: The remaining previously-skipped tests were redundant with the comprehensive Phase 5
// integration suite and have been intentionally removed to avoid duplicate coverage.
