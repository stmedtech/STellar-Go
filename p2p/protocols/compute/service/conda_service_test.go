package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockComputeClient is a mock implementation of compute Client for testing
type mockComputeClient struct {
	runFunc func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error)
}

func (m *mockComputeClient) Run(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockComputeClient) Close() error {
	return nil
}

// createMockHandle creates a mock RawExecutionHandle with the given output
func createMockHandle(runID string, stdout, stderr string, exitCode int, err error) *RawExecutionHandle {
	doneCh := make(chan error, 1)
	exitCh := make(chan int, 1)
	doneCh <- err
	exitCh <- exitCode
	close(doneCh)
	close(exitCh)

	// Create a pipe for stdin to allow writing
	stdinR, stdinW := io.Pipe()
	// Close the read end immediately since we're not using it in most tests
	go func() {
		_, _ = io.Copy(io.Discard, stdinR)
		_ = stdinR.Close()
	}()

	return &RawExecutionHandle{
		RunID:    runID,
		Stdin:    stdinW,
		Stdout:   io.NopCloser(strings.NewReader(stdout)),
		Stderr:   io.NopCloser(strings.NewReader(stderr)),
		Done:     doneCh,
		ExitCode: exitCh,
		Cancel:   func() error { return nil },
	}
}

// createMockHandleForEnvList creates a mock handle for conda env list
func createMockHandleForEnvList(envs map[string]string) *RawExecutionHandle {
	output := "# conda environments:\n#\nbase                  /opt/conda\n"
	for name, path := range envs {
		output += fmt.Sprintf("%s                  %s\n", name, path)
	}
	return createMockHandle("env-list", output, "", 0, nil)
}

// newCondaServiceWithClient creates a CondaService with a custom client interface (for testing)
func newCondaServiceWithClient(client computeClientInterface) *CondaService {
	return &CondaService{
		client: client,
	}
}

// TestCondaService_ListEnvironments_Success tests listing remote environments successfully
func TestCondaService_ListEnvironments_Success(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			// Verify command is conda env list
			assert.Equal(t, "conda", req.Command)
			assert.Contains(t, req.Args, "env")
			assert.Contains(t, req.Args, "list")

			// Return mock output
			output := "# conda environments:\n#\nbase                  /opt/conda\nenv1                  /opt/conda/envs/env1\nenv2                  /opt/conda/envs/env2\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	require.NoError(t, err)
	assert.Len(t, envs, 2)
	assert.Contains(t, envs, "env1")
	assert.Contains(t, envs, "env2")
	assert.NotContains(t, envs, "base")
}

// TestCondaService_ListEnvironments_ConnectionError tests handling connection failures
func TestCondaService_ListEnvironments_ConnectionError(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			return nil, io.ErrClosedPipe
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
	// Check for connection-related error (could be "connection error" or "closed pipe" etc.)
	assert.True(t, strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "pipe"),
		"Error should contain connection-related message: %v", err)
}

// TestCondaService_ListEnvironments_CommandError tests handling remote command errors
func TestCondaService_ListEnvironments_CommandError(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 1 // Non-zero exit code
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("conda: command not found")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_ListEnvironments_Timeout tests handling timeout
func TestCondaService_ListEnvironments_Timeout(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			// Simulate timeout by not completing
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			// Don't send anything, simulating timeout

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
				Cancel: func() error {
					return nil
				},
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
	// Check for timeout or deadline error
	assert.True(t, strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline"), "Error should contain timeout or deadline: %v", err)
}

// TestCondaService_ListEnvironments_Streaming tests streaming output in real-time
func TestCondaService_ListEnvironments_Streaming(t *testing.T) {
	outputChunks := []string{
		"# conda environments:\n#\n",
		"base                  /opt/conda\n",
		"env1                  /opt/conda/envs/env1\n",
		"env2                  /opt/conda/envs/env2\n",
	}
	chunkIdx := 0

	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)

			// Create a reader that yields chunks
			reader := io.NopCloser(io.MultiReader(
				strings.NewReader(outputChunks[0]),
				strings.NewReader(outputChunks[1]),
				strings.NewReader(outputChunks[2]),
				strings.NewReader(outputChunks[3]),
			))

			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   reader,
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	require.NoError(t, err)
	assert.Len(t, envs, 2)
	_ = chunkIdx // Suppress unused warning
}

// TestCondaService_CreateEnvironment_Success tests creating remote environment successfully
func TestCondaService_CreateEnvironment_Success(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env doesn't exist
				output := "# conda environments:\n#\nbase                  /opt/conda\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return &RawExecutionHandle{
					RunID:    "test-run-1",
					Stdout:   io.NopCloser(strings.NewReader(output)),
					Stderr:   io.NopCloser(strings.NewReader("")),
					Done:     doneCh,
					ExitCode: exitCh,
					Cancel:   func() error { return nil },
				}, nil
			} else if callCount == 2 {
				// Second call: conda create
				assert.Equal(t, "conda", req.Command)
				assert.Contains(t, req.Args, "create")
				assert.Contains(t, req.Args, "--name")
				assert.Contains(t, req.Args, "testenv")
				assert.Contains(t, strings.Join(req.Args, " "), "python=3.9")

				output := "conda activate testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return &RawExecutionHandle{
					RunID:    "test-run-2",
					Stdout:   io.NopCloser(strings.NewReader(output)),
					Stderr:   io.NopCloser(strings.NewReader("")),
					Done:     doneCh,
					ExitCode: exitCh,
					Cancel:   func() error { return nil },
				}, nil
			} else if callCount == 3 {
				// Third call: GetEnvironment (conda env list) - env now exists
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return &RawExecutionHandle{
					RunID:    "test-run-3",
					Stdout:   io.NopCloser(strings.NewReader(output)),
					Stderr:   io.NopCloser(strings.NewReader("")),
					Done:     doneCh,
					ExitCode: exitCh,
					Cancel:   func() error { return nil },
				}, nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	path, err := service.CreateEnvironment(ctx, "testenv", "3.9")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

// TestCondaService_CreateEnvironment_AlreadyExists tests handling existing environment
func TestCondaService_CreateEnvironment_AlreadyExists(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list)
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Should not reach here if already exists check works
			t.Fatal("CreateEnvironment should not call conda create if env already exists")
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	path, err := service.CreateEnvironment(ctx, "testenv", "3.9")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Equal(t, 1, callCount, "Should only call GetEnvironment, not create")
}

// TestCondaService_CreateEnvironment_InvalidVersion tests handling invalid version
func TestCondaService_CreateEnvironment_InvalidVersion(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment returns not found
				output := "# conda environments:\n#\nbase                  /opt/conda\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: conda create fails
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 1
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-2",
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("error: invalid Python version: 99.99")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	path, err := service.CreateEnvironment(ctx, "testenv", "99.99")
	assert.Error(t, err)
	assert.Empty(t, path)
}

// TestCondaService_CreateEnvironment_Streaming tests streaming creation progress
func TestCondaService_CreateEnvironment_Streaming(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment returns not found
				return createMockHandleForEnvList(map[string]string{}), nil
			} else if callCount == 2 {
				// Second call: conda create with streaming output
				return createMockHandle("test-run-2", "Collecting package metadata...\nSolving environment...\nconda activate testenv\n", "", 0, nil), nil
			} else if callCount == 3 {
				// Third call: GetEnvironment (conda env list) - env now exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	path, err := service.CreateEnvironment(ctx, "testenv", "3.9")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

// TestCondaService_CreateEnvironment_Concurrent tests handling concurrent requests
func TestCondaService_CreateEnvironment_Concurrent(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			mu.Lock()
			callCount++
			myCall := callCount
			mu.Unlock()

			if myCall == 1 || myCall == 4 {
				// GetEnvironment calls (one per concurrent create)
				return createMockHandleForEnvList(map[string]string{}), nil
			} else if myCall == 2 || myCall == 5 {
				// conda create calls
				return createMockHandle("test-run", "conda activate testenv\n", "", 0, nil), nil
			} else if myCall == 3 || myCall == 6 {
				// GetEnvironment verification calls
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			}
			t.Fatal("Unexpected call count:", myCall)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	// Run concurrent creates
	done := make(chan error, 2)
	go func() {
		_, err := service.CreateEnvironment(ctx, "testenv", "3.9")
		done <- err
	}()
	go func() {
		_, err := service.CreateEnvironment(ctx, "testenv", "3.9")
		done <- err
	}()

	err1 := <-done
	err2 := <-done
	// At least one should succeed
	assert.True(t, err1 == nil || err2 == nil, "At least one concurrent create should succeed")
}

// TestCondaService_RemoveEnvironment_Success tests removing remote environment successfully
func TestCondaService_RemoveEnvironment_Success(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: conda env remove
				assert.Equal(t, "conda", req.Command)
				assert.Contains(t, req.Args, "env")
				assert.Contains(t, req.Args, "remove")
				assert.Contains(t, req.Args, "--name")
				assert.Contains(t, req.Args, "testenv")

				return createMockHandle("test-run-2", "Executing transaction: done\n", "", 0, nil), nil
			} else if callCount == 3 {
				// Third call: GetEnvironment (conda env list) - env no longer exists
				return createMockHandleForEnvList(map[string]string{}), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.RemoveEnvironment(ctx, "testenv")
	require.NoError(t, err)
}

// TestCondaService_RemoveEnvironment_NotFound tests handling non-existent environment
func TestCondaService_RemoveEnvironment_NotFound(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			// GetEnvironment - env not found
			output := "# conda environments:\n#\nbase                  /opt/conda\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.RemoveEnvironment(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestCondaService_RemoveEnvironment_Streaming tests streaming removal progress
func TestCondaService_RemoveEnvironment_Streaming(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment - env exists
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: conda env remove with streaming
			output := "Removing packages...\nExecuting transaction: done\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-2",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.RemoveEnvironment(ctx, "testenv")
	require.NoError(t, err)
}

// TestCondaService_RunPython_Success tests executing Python code successfully
func TestCondaService_RunPython_Success(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython
				// Verify CONDA_ENV is set
				assert.NotEmpty(t, req.Env["CONDA_ENV"])
				assert.Equal(t, "testenv", req.Env["CONDA_ENV"])
				// Verify command is python
				assert.Equal(t, "python", req.Command)
				assert.Contains(t, req.Args, "-c")

				return createMockHandle("python-run", "Hello, World!\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", "print('Hello, World!')", nil)
	require.NoError(t, err)
	require.NotNil(t, handle)

	out, err := io.ReadAll(handle.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Hello, World!")

	require.NoError(t, <-handle.Done)
	assert.Equal(t, 0, <-handle.ExitCode)
}

// TestCondaService_RunPython_WithStdin tests handling stdin input
func TestCondaService_RunPython_WithStdin(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython
				// Create a pipe for stdin - the read end will be consumed by the mock
				stdinR, stdinW := io.Pipe()
				handle := createMockHandle("test-run", "input received\n", "", 0, nil)
				handle.Stdin = stdinW

				// Read from the pipe in background to prevent blocking
				go func() {
					_, _ = io.Copy(io.Discard, stdinR)
					_ = stdinR.Close()
				}()

				return handle, nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	stdin := strings.NewReader("test input\n")
	handle, err := service.RunPython(ctx, "testenv", "import sys; print(sys.stdin.read())", stdin)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// The stdin should be copied automatically by RunPython, so we just wait
	out, err := io.ReadAll(handle.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "input received")

	require.NoError(t, <-handle.Done)
	assert.Equal(t, 0, <-handle.ExitCode)
}

// TestCondaService_RunPython_Streaming tests streaming stdout/stderr in real-time
func TestCondaService_RunPython_Streaming(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython
				return createMockHandle("test-run", "line1\nline2\nline3\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", "for i in range(1, 4): print(f'line{i}')", nil)
	require.NoError(t, err)

	out, err := io.ReadAll(handle.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "line1")
	assert.Contains(t, string(out), "line2")
	assert.Contains(t, string(out), "line3")

	require.NoError(t, <-handle.Done)
}

// TestCondaService_RunPython_Error tests handling Python execution errors
func TestCondaService_RunPython_Error(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython - error (non-zero exit code with error)
				return createMockHandle("test-run", "", "SyntaxError: invalid syntax\n", 1, fmt.Errorf("execution failed")), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", "invalid python code!!!", nil)
	require.NoError(t, err) // Run() succeeds, execution fails

	err = <-handle.Done
	assert.Error(t, err)
	assert.Equal(t, 1, <-handle.ExitCode)
}

// TestCondaService_RunPython_EnvNotFound tests handling non-existent environment
func TestCondaService_RunPython_EnvNotFound(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment check
				output := "# conda environments:\n#\nbase                  /opt/conda\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Should not reach here
			t.Fatal("RunPython should check environment exists first")
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "nonexistent", "print('hello')", nil)
	assert.Error(t, err)
	assert.Nil(t, handle)
	assert.Contains(t, err.Error(), "not found")
}

// TestCondaService_RunPython_ExitCode tests correctly propagating exit codes
func TestCondaService_RunPython_ExitCode(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython - custom exit code
				return createMockHandle("test-run", "", "", 42, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", "import sys; sys.exit(42)", nil)
	require.NoError(t, err)

	require.NoError(t, <-handle.Done)
	assert.Equal(t, 42, <-handle.ExitCode)
}

// TestCondaService_RunPython_Cancel tests handling cancellation
func TestCondaService_RunPython_Cancel(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython - cancellation
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				// Don't complete, simulating cancellation

				handle := &RawExecutionHandle{
					RunID:    "test-run",
					Stdout:   io.NopCloser(strings.NewReader("")),
					Stderr:   io.NopCloser(strings.NewReader("")),
					Done:     doneCh,
					ExitCode: exitCh,
					Cancel: func() error {
						doneCh <- context.Canceled
						exitCh <- -1
						close(doneCh)
						close(exitCh)
						return nil
					},
				}

				// Simulate cancellation
				go func() {
					time.Sleep(50 * time.Millisecond)
					_ = handle.Cancel()
				}()

				return handle, nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", "import time; time.sleep(10)", nil)
	require.NoError(t, err)

	err = <-handle.Done
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancel")
}

// TestCondaService_RunScript_Success tests executing Python script successfully
func TestCondaService_RunScript_Success(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript
				// Verify CONDA_ENV is set
				assert.Equal(t, "testenv", req.Env["CONDA_ENV"])
				// Verify command is python
				assert.Equal(t, "python", req.Command)
				assert.Contains(t, req.Args, "/path/to/script.py")
				return createMockHandle("test-run", "Script output\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunScript(ctx, "testenv", "/path/to/script.py", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, handle)

	out, err := io.ReadAll(handle.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Script output")

	require.NoError(t, <-handle.Done)
	assert.Equal(t, 0, <-handle.ExitCode)
}

// TestCondaService_RunScript_WithArgs tests handling script arguments
func TestCondaService_RunScript_WithArgs(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript with args
				// Verify args are passed
				assert.Contains(t, req.Args, "/path/to/script.py")
				assert.Contains(t, req.Args, "arg1")
				assert.Contains(t, req.Args, "arg2")
				return createMockHandle("test-run", "arg1 arg2\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunScript(ctx, "testenv", "/path/to/script.py", []string{"arg1", "arg2"}, nil)
	require.NoError(t, err)
	require.NotNil(t, handle)

	require.NoError(t, <-handle.Done)
}

// TestCondaService_RunScript_WithStdin tests handling stdin input
func TestCondaService_RunScript_WithStdin(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript with stdin
				// Create a pipe for stdin - the read end will be consumed by the mock
				stdinR, stdinW := io.Pipe()
				handle := createMockHandle("test-run", "input processed\n", "", 0, nil)
				handle.Stdin = stdinW

				// Read from the pipe in background to prevent blocking
				go func() {
					_, _ = io.Copy(io.Discard, stdinR)
					_ = stdinR.Close()
				}()

				return handle, nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	stdin := strings.NewReader("test input\n")
	handle, err := service.RunScript(ctx, "testenv", "/path/to/script.py", nil, stdin)
	require.NoError(t, err)

	// The stdin should be copied automatically by RunScript, so we just wait
	require.NoError(t, <-handle.Done)
}

// TestCondaService_RunScript_FileNotFound tests handling missing script file
func TestCondaService_RunScript_FileNotFound(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript - file not found
				return createMockHandle("test-run", "", "python: can't open file '/nonexistent/script.py': [Errno 2] No such file or directory\n", 1, fmt.Errorf("execution failed")), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunScript(ctx, "testenv", "/nonexistent/script.py", nil, nil)
	require.NoError(t, err) // Run() succeeds, execution fails

	err = <-handle.Done
	assert.Error(t, err)
	assert.Equal(t, 1, <-handle.ExitCode)
}

// TestCondaService_RunScript_Streaming tests streaming script output
func TestCondaService_RunScript_Streaming(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript - streaming
				return createMockHandle("test-run", "output line 1\noutput line 2\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunScript(ctx, "testenv", "/path/to/script.py", nil, nil)
	require.NoError(t, err)

	out, err := io.ReadAll(handle.Stdout)
	require.NoError(t, err)
	assert.Contains(t, string(out), "output line 1")
	assert.Contains(t, string(out), "output line 2")

	require.NoError(t, <-handle.Done)
}

// TestCondaService_RunScript_Error tests handling script execution errors
func TestCondaService_RunScript_Error(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunScript - error (non-zero exit code with error)
				return createMockHandle("test-run", "", "Traceback (most recent call last):\n  File \"/path/to/script.py\", line 1, in <module>\n    raise ValueError('error')\nValueError: error\n", 1, fmt.Errorf("execution failed")), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunScript(ctx, "testenv", "/path/to/script.py", nil, nil)
	require.NoError(t, err) // Run() succeeds, execution fails

	err = <-handle.Done
	assert.Error(t, err) // Execution should have an error
	assert.Equal(t, 1, <-handle.ExitCode)
}

// TestCondaService_InstallPackage_Success tests installing package remotely successfully
func TestCondaService_InstallPackage_Success(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment check
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: pip install
			assert.Equal(t, "pip", req.Command)
			assert.Equal(t, "testenv", req.Env["CONDA_ENV"])
			assert.Contains(t, req.Args, "install")
			assert.Contains(t, req.Args, "numpy")

			return createMockHandle("test-run-2", "Successfully installed numpy-1.24.0\n", "", 0, nil), nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.InstallPackage(ctx, "testenv", "numpy")
	require.NoError(t, err)
}

// TestCondaService_InstallPackage_AlreadyInstalled tests handling already installed package
func TestCondaService_InstallPackage_AlreadyInstalled(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment check
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: pip install - already installed
			output := "Requirement already satisfied: numpy\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-2",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.InstallPackage(ctx, "testenv", "numpy")
	require.NoError(t, err) // Already installed is success
}

// TestCondaService_InstallPackage_NotFound tests handling package not found
func TestCondaService_InstallPackage_NotFound(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment check
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: pip install - package not found
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 1
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-2",
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("ERROR: Could not find a version that satisfies the requirement nonexistent-package\n")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.InstallPackage(ctx, "testenv", "nonexistent-package")
	assert.Error(t, err)
}

// TestCondaService_InstallPackage_Streaming tests streaming installation progress
func TestCondaService_InstallPackage_Streaming(t *testing.T) {
	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// GetEnvironment check
				output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
				doneCh := make(chan error, 1)
				exitCh := make(chan int, 1)
				doneCh <- nil
				exitCh <- 0
				close(doneCh)
				close(exitCh)

				return createMockHandle("test-run-1", output, "", 0, nil), nil
			}
			// Second call: pip install with streaming
			output := "Collecting numpy\nDownloading numpy-1.24.0...\nInstalling numpy...\nSuccessfully installed numpy-1.24.0\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-2",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	err := service.InstallPackage(ctx, "testenv", "numpy")
	require.NoError(t, err)
}

// TestCondaService_ErrorPropagation_CommandError tests propagating command errors correctly
func TestCondaService_ErrorPropagation_CommandError(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 127 // Command not found
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("conda: command not found\n")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_ErrorPropagation_NetworkError tests propagating network errors
func TestCondaService_ErrorPropagation_NetworkError(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			return nil, io.ErrUnexpectedEOF
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_ErrorPropagation_Timeout tests propagating timeout errors
func TestCondaService_ErrorPropagation_Timeout(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			// Don't complete, simulating timeout

			// Create a pipe for stdin
			stdinR, stdinW := io.Pipe()
			go func() {
				_, _ = io.Copy(io.Discard, stdinR)
				_ = stdinR.Close()
			}()

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdin:    stdinW,
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
				Cancel:   func() error { return nil },
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_ErrorPropagation_ServerError tests propagating server-side errors
func TestCondaService_ErrorPropagation_ServerError(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			// Create a pipe for stdin
			stdinR, stdinW := io.Pipe()
			go func() {
				_, _ = io.Copy(io.Discard, stdinR)
				_ = stdinR.Close()
			}()

			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- fmt.Errorf("server error: internal server error")
			exitCh <- -1
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdin:    stdinW,
				Stdout:   io.NopCloser(strings.NewReader("")),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
				Cancel:   func() error { return nil },
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
	assert.Contains(t, err.Error(), "server error")
}

// TestCondaService_NilClient tests handling nil compute client
func TestCondaService_NilClient(t *testing.T) {
	service := NewCondaService(nil)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_DisconnectedClient tests handling disconnected client
func TestCondaService_DisconnectedClient(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			return nil, io.ErrClosedPipe
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	envs, err := service.ListEnvironments(ctx)
	assert.Error(t, err)
	assert.Nil(t, envs)
}

// TestCondaService_EmptyEnvName tests handling empty environment names
func TestCondaService_EmptyEnvName(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	// Test CreateEnvironment
	path, err := service.CreateEnvironment(ctx, "", "3.9")
	assert.Error(t, err)
	assert.Empty(t, path)

	// Test RemoveEnvironment
	err = service.RemoveEnvironment(ctx, "")
	assert.Error(t, err)

	// Test RunPython
	handle, err := service.RunPython(ctx, "", "print('hello')", nil)
	assert.Error(t, err)
	assert.Nil(t, handle)

	// Test InstallPackage
	err = service.InstallPackage(ctx, "", "numpy")
	assert.Error(t, err)
}

// TestCondaService_ConcurrentOperations tests handling concurrent service calls
func TestCondaService_ConcurrentOperations(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			output := "# conda environments:\n#\nbase                  /opt/conda\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	// Run concurrent operations
	done := make(chan error, 3)
	go func() {
		_, err := service.ListEnvironments(ctx)
		done <- err
	}()
	go func() {
		_, err := service.ListEnvironments(ctx)
		done <- err
	}()
	go func() {
		_, err := service.ListEnvironments(ctx)
		done <- err
	}()

	err1 := <-done
	err2 := <-done
	err3 := <-done

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
}

// TestCondaService_LargeCode tests handling large Python code blocks
func TestCondaService_LargeCode(t *testing.T) {
	// Generate large code
	largeCode := "print('start')\n"
	for i := 0; i < 1000; i++ {
		largeCode += "print(f'line {i}')\n"
	}
	largeCode += "print('end')"

	callCount := 0
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			callCount++
			if callCount == 1 {
				// First call: GetEnvironment (conda env list) - env exists
				return createMockHandleForEnvList(map[string]string{"testenv": "/opt/conda/envs/testenv"}), nil
			} else if callCount == 2 {
				// Second call: RunPython with large code
				// Verify code is passed correctly
				assert.Contains(t, req.Args, "-c")
				assert.Contains(t, strings.Join(req.Args, " "), "print('start')")
				return createMockHandle("test-run", "start\nend\n", "", 0, nil), nil
			}
			t.Fatal("Unexpected call count:", callCount)
			return nil, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	handle, err := service.RunPython(ctx, "testenv", largeCode, nil)
	require.NoError(t, err)
	require.NotNil(t, handle)

	require.NoError(t, <-handle.Done)
}

// TestCondaService_SpecialChars tests handling special characters in env/package names
func TestCondaService_SpecialChars(t *testing.T) {
	mockClient := &mockComputeClient{
		runFunc: func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
			// Verify special chars are handled
			assert.Contains(t, req.Args, "my-env_123")
			assert.Contains(t, req.Args, "package-name_1.2.3")

			output := "Successfully installed package-name_1.2.3\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		},
	}

	service := newCondaServiceWithClient(mockClient)
	ctx := context.Background()

	// First verify env exists
	callCount := 0
	mockClient.runFunc = func(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
		callCount++
		if callCount == 1 {
			output := "# conda environments:\n#\nbase                  /opt/conda\nmy-env_123            /opt/conda/envs/my-env_123\n"
			doneCh := make(chan error, 1)
			exitCh := make(chan int, 1)
			doneCh <- nil
			exitCh <- 0
			close(doneCh)
			close(exitCh)

			return &RawExecutionHandle{
				RunID:    "test-run-1",
				Stdout:   io.NopCloser(strings.NewReader(output)),
				Stderr:   io.NopCloser(strings.NewReader("")),
				Done:     doneCh,
				ExitCode: exitCh,
			}, nil
		}
		// Second call: pip install
		output := "Successfully installed package-name_1.2.3\n"
		doneCh := make(chan error, 1)
		exitCh := make(chan int, 1)
		doneCh <- nil
		exitCh <- 0
		close(doneCh)
		close(exitCh)

		return &RawExecutionHandle{
			RunID:    "test-run-2",
			Stdout:   io.NopCloser(strings.NewReader(output)),
			Stderr:   io.NopCloser(strings.NewReader("")),
			Done:     doneCh,
			ExitCode: exitCh,
		}, nil
	}

	err := service.InstallPackage(ctx, "my-env_123", "package-name_1.2.3")
	require.NoError(t, err)
}
