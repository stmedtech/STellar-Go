//go:build conda

package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"stellar/p2p/protocols/compute/streams"
)

// ============================================================================
// Server Conda Operation Tests
// ============================================================================

func requireCondaForServer(t *testing.T) {
	// Since this file has //go:build conda, tests here only run with -tags conda
	// We still need to verify conda is actually available

	// Conda initialization happens via:
	// 1. If running full suite (go test ./p2p/protocols/compute/...), conda package is imported and init() runs
	// 2. If running service tests alone, conda_test_helper_test.go (separate test package) imports conda and calls Initialize()
	// Check if creator is registered
	if GetCondaOperationsCreator() == nil {
		t.Fatal("conda operations creator not registered. Run: go test ./p2p/protocols/compute/... (full suite) or ensure conda_test_helper_test.go is compiled")
	}

	// Try to run a simple __conda command to verify conda is available
	// We can't import conda package due to import cycle, so we test via server
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

	// Try to run __conda version to verify conda works
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-check",
		Command: "__conda",
		Args:    []string{"version"},
	})
	if err != nil {
		t.Fatalf("Failed to run __conda version (conda should be available in Docker): %v", err)
	}

	require.NoError(t, h.Stdin.Close())
	_, _ = io.ReadAll(h.Stdout)
	err = <-h.Done
	if err != nil {
		t.Fatalf("Conda version command failed (conda should work in Docker): %v", err)
	}

	_ = client.Close()
	_ = server.Close()
}

func generateTestEnvName(prefix string) string {
	return fmt.Sprintf("test-%s-%d", prefix, time.Now().UnixNano())
}

func readServerExecutionOutput(exec streams.ExecutionStreamReader) (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, exec.GetStdout())
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, exec.GetStderr())
	}()

	doneErr := <-exec.GetDone()
	if code, ok := <-exec.GetExitCode(); ok {
		exitCode = code
	}

	wg.Wait()
	return stdoutBuf.String(), stderrBuf.String(), exitCode, doneErr
}

func TestServer_HandleCondaOperation_List(t *testing.T) {
	requireCondaForServer(t)

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

	// Send __conda list command
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-list-test",
		Command: "__conda",
		Args:    []string{"list"},
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "base")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_Get(t *testing.T) {
	requireCondaForServer(t)

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

	// Send __conda get command
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-get-test",
		Command: "__conda",
		Args:    []string{"get", "base"},
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(stdout)))

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_Create(t *testing.T) {
	requireCondaForServer(t)

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

	envName := generateTestEnvName("server-create")

	// Send __conda create command
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-create-test",
		Command: "__conda",
		Args:    []string{"create", envName, "--python", "3.9"},
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	_, err = io.ReadAll(h.Stdout)
	require.NoError(t, err)

	err = <-h.Done
	require.NoError(t, err)
	assert.Equal(t, 0, <-h.ExitCode)

	// Cleanup
	removeH, _ := client.Run(ctx, RunRequest{
		RunID:   "conda-remove-test",
		Command: "__conda",
		Args:    []string{"remove", envName},
	})
	if removeH != nil {
		require.NoError(t, removeH.Stdin.Close())
		_, _ = io.ReadAll(removeH.Stdout)
		<-removeH.Done
	}

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_RunPython(t *testing.T) {
	requireCondaForServer(t)

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

	envName := generateTestEnvName("server-runpython")

	// Create environment first
	createH, err := client.Run(ctx, RunRequest{
		RunID:   "conda-create-env",
		Command: "__conda",
		Args:    []string{"create", envName, "--python", "3.9"},
	})
	require.NoError(t, err)
	require.NoError(t, createH.Stdin.Close())
	_, _ = io.ReadAll(createH.Stdout)
	<-createH.Done

	// Run Python code
	runH, err := client.Run(ctx, RunRequest{
		RunID:   "conda-run-python",
		Command: "__conda",
		Args:    []string{"run-python", envName, "print('Hello from server!')"},
	})
	require.NoError(t, err)
	require.NotNil(t, runH)

	require.NoError(t, runH.Stdin.Close())
	stdout, err := io.ReadAll(runH.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "Hello from server!")

	require.NoError(t, <-runH.Done)
	assert.Equal(t, 0, <-runH.ExitCode)

	// Cleanup
	removeH, _ := client.Run(ctx, RunRequest{
		RunID:   "conda-remove-env",
		Command: "__conda",
		Args:    []string{"remove", envName},
	})
	if removeH != nil {
		require.NoError(t, removeH.Stdin.Close())
		_, _ = io.ReadAll(removeH.Stdout)
		<-removeH.Done
	}

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_Run(t *testing.T) {
	requireCondaForServer(t)

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

	// Send __conda run command
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-run-test",
		Command: "__conda",
		Args:    []string{"run", "env", "list"},
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stdout, err := io.ReadAll(h.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "base")

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_InvalidSubcommand(t *testing.T) {
	// This test requires conda operations creator to be registered
	// In Docker, this should be registered when running full test suite
	// Since this file has //go:build conda, tests here only run with -tags conda

	// In Docker, creator should be registered when running full suite
	if GetCondaOperationsCreator() == nil {
		t.Fatal("conda operations creator not registered. In Docker, run: go test ./p2p/protocols/compute/... (full suite)")
	}

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

	// Send invalid __conda command
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-invalid-test",
		Command: "__conda",
		Args:    []string{"unknown-command"},
	})
	require.NoError(t, err) // Run succeeds, execution fails
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())
	stderr, err := io.ReadAll(h.Stderr)
	require.NoError(t, err)
	assert.Contains(t, string(stderr), "unknown")

	err = <-h.Done
	assert.Error(t, err)
	exitCode := <-h.ExitCode
	assert.NotEqual(t, 0, exitCode)

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_EmptySubcommand(t *testing.T) {
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

	// Send __conda without subcommand - should be rejected immediately
	_, err := client.Run(ctx, RunRequest{
		RunID:   "conda-empty-test",
		Command: "__conda",
		Args:    []string{},
	})
	// Should be rejected with error about missing subcommand
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subcommand")

	_ = client.Close()
	_ = server.Close()
}

func TestServer_HandleCondaOperation_Streaming(t *testing.T) {
	requireCondaForServer(t)

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

	// Send __conda version (should stream output)
	h, err := client.Run(ctx, RunRequest{
		RunID:   "conda-version-test",
		Command: "__conda",
		Args:    []string{"version"},
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	require.NoError(t, h.Stdin.Close())

	// Read in chunks to test streaming
	chunk := make([]byte, 1024)
	n, err := h.Stdout.Read(chunk)
	assert.True(t, n > 0 || err == io.EOF)

	// Read rest
	_, _ = io.ReadAll(h.Stdout)

	require.NoError(t, <-h.Done)
	assert.Equal(t, 0, <-h.ExitCode)

	_ = client.Close()
	_ = server.Close()
}
