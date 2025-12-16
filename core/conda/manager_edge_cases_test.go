package conda

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"stellar/p2p/protocols/compute/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCondaManager_CreateEnvironment_InvalidPythonVersion tests handling invalid Python versions
func TestCondaManager_CreateEnvironment_InvalidPythonVersion(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: ListEnvironments (check if exists)
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Second call: CreateEnvironment fails with invalid version
			return createMockExecution("", "error: invalid python version '99.99'", 1, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	_, err := manager.CreateEnvironment(ctx, "testenv", "99.99")
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "exit status 1") ||
			strings.Contains(err.Error(), "exited with code 1") ||
			strings.Contains(err.Error(), "invalid python version"),
		"Error should indicate failure: %s", err.Error())
}

// TestCondaManager_CreateEnvironment_NetworkTimeout tests handling network timeouts
func TestCondaManager_CreateEnvironment_NetworkTimeout(t *testing.T) {
	if !ShouldRunCondaTests() {
		t.Skip("Skipping TestCondaManager_CreateEnvironment_NetworkTimeout - requires conda. Run in Docker or set CONDATEST_ENABLED=true")
	}

	realExec := service.NewRawExecutor()
	manager := NewCondaManager(realExec, "conda")
	// Use very short timeout to simulate network issues
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	envName := fmt.Sprintf("test-timeout-env-%d", time.Now().UnixNano())
	_, err := manager.CreateEnvironment(ctx, envName, "3.9")

	// Should fail with timeout
	assert.Error(t, err)
	assert.True(t, ctx.Err() == context.DeadlineExceeded || strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline"))
}

// TestCondaManager_CreateEnvironment_ConcurrentSameName tests concurrent creation with same name
func TestCondaManager_CreateEnvironment_ConcurrentSameName(t *testing.T) {
	if !ShouldRunCondaTests() {
		t.Skip("Skipping TestCondaManager_CreateEnvironment_ConcurrentSameName - requires conda. Run in Docker or set CONDATEST_ENABLED=true")
	}

	InitTestLogging(t)
	realExec := service.NewRawExecutor()
	manager := NewCondaManager(realExec, "conda")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	envName := fmt.Sprintf("test-concurrent-env-%d", time.Now().UnixNano())

	// Try to create the same environment concurrently
	done := make(chan error, 2)
	go func() {
		_, err := manager.CreateEnvironment(ctx, envName, "3.9")
		done <- err
	}()
	go func() {
		_, err := manager.CreateEnvironment(ctx, envName, "3.9")
		done <- err
	}()

	// Both should either succeed (one creates, one finds existing) or one fails
	err1 := <-done
	err2 := <-done

	// Log errors for debugging
	if err1 != nil {
		t.Logf("First concurrent creation failed: %v", err1)
	}
	if err2 != nil {
		t.Logf("Second concurrent creation failed: %v", err2)
	}

	// At least one should succeed (either created or found existing)
	successCount := 0
	if err1 == nil {
		successCount++
		t.Logf("First concurrent creation succeeded")
	}
	if err2 == nil {
		successCount++
		t.Logf("Second concurrent creation succeeded")
	}

	// If both failed, check if it's because the environment already exists
	// (race condition: both might try to create, one succeeds, other fails with "already exists")
	if successCount == 0 {
		err1Str := ""
		err2Str := ""
		if err1 != nil {
			err1Str = err1.Error()
		}
		if err2 != nil {
			err2Str = err2.Error()
		}
		// Check if errors indicate environment already exists (which is actually success)
		if strings.Contains(err1Str, "already exists") || strings.Contains(err2Str, "already exists") {
			t.Logf("One or both operations detected existing environment (acceptable race condition)")
			successCount = 1 // Treat as success
		} else {
			// Both failed for other reasons - this is a real failure
			t.Errorf("Both concurrent creations failed:\n  err1: %v\n  err2: %v", err1, err2)
		}
	}

	assert.GreaterOrEqual(t, successCount, 1, "At least one concurrent creation should succeed")

	// Clean up
	if err1 == nil || err2 == nil {
		_ = manager.RemoveEnvironment(context.Background(), envName)
	}
}

// TestCondaManager_RemoveEnvironment_AlreadyRemoved tests removing an already-removed environment
func TestCondaManager_RemoveEnvironment_AlreadyRemoved(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: GetEnvironment (check if exists) - returns not found
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Should not reach here, but if it does, return error
			return createMockExecution("", "error: environment not found", 1, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	err := manager.RemoveEnvironment(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestCondaManager_InstallPackage_EmptyPackageName tests handling empty package name
func TestCondaManager_InstallPackage_EmptyPackageName(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}
	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	err := manager.InstallPackage(ctx, "myenv", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package name is empty")
}

// TestCondaManager_ListEnvironments_WithSpecialChars tests handling special characters in env names
func TestCondaManager_ListEnvironments_WithSpecialChars(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\nenv-123                /opt/conda/envs/env-123\nenv_456                /opt/conda/envs/env_456\nenv.789                /opt/conda/envs/env.789\n"

	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			return createMockExecution(output, "", 0, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	envs, err := manager.ListEnvironments(ctx)
	require.NoError(t, err)
	assert.Contains(t, envs, "env-123")
	assert.Contains(t, envs, "env_456")
	assert.Contains(t, envs, "env.789")
}

// TestCondaManager_CreateEnvironment_WithVeryLongName tests handling very long environment names
func TestCondaManager_CreateEnvironment_WithVeryLongName(t *testing.T) {
	longName := strings.Repeat("a", 300) // Very long name

	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: GetEnvironment (check if exists)
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Second call: CreateEnvironment - may fail due to name length
			return createMockExecution("", "error: environment name too long", 1, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	_, err := manager.CreateEnvironment(ctx, longName, "3.9")
	// May fail due to name length restrictions
	assert.Error(t, err)
}

// TestCondaManager_GetEnvironment_EmptyName_EdgeCase tests handling empty environment name (edge case)
// Note: Main test exists in manager_test.go, this is for additional edge case coverage
func TestCondaManager_GetEnvironment_EmptyName_EdgeCase(t *testing.T) {
	manager := NewCondaManager(nil, "conda")
	ctx := context.Background()

	_, err := manager.GetEnvironment(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestCondaManager_UpdateEnvironment_EmptyYAMLPath tests handling empty YAML path
func TestCondaManager_UpdateEnvironment_EmptyYAMLPath(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}
	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	err := manager.UpdateEnvironment(ctx, "myenv", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "YAML file path is empty")
}

// TestCondaManager_InstallPackage_WithVersionSpec tests installing package with version specifier
func TestCondaManager_InstallPackage_WithVersionSpec(t *testing.T) {
	callCount := 0
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			callCount++
			if callCount == 1 {
				output := "# conda environments:\n#\nbase                  /opt/conda\nmyenv                 /opt/conda/envs/myenv\n"
				return createMockExecution(output, "", 0, nil), nil
			} else {
				// Verify that package name with version is passed correctly
				args := strings.Join(req.Args, " ")
				assert.Contains(t, args, "requests==2.31.0")
				return createMockExecution("Successfully installed requests-2.31.0", "", 0, nil), nil
			}
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	err := manager.InstallPackage(ctx, "myenv", "requests==2.31.0")
	assert.NoError(t, err)
}

// TestCondaManager_ListEnvironments_WithActiveMarker tests parsing environments with active marker
func TestCondaManager_ListEnvironments_WithActiveMarker(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\n*myenv                /opt/conda/envs/myenv\nother                 /opt/conda/envs/other\n"

	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			return createMockExecution(output, "", 0, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	envs, err := manager.ListEnvironments(ctx)
	require.NoError(t, err)
	// Should parse myenv without the asterisk
	assert.Contains(t, envs, "myenv")
	assert.Contains(t, envs, "other")
	assert.NotContains(t, envs, "*myenv")
}

// TestCondaManager_CreateEnvironment_ContextCancellation tests handling context cancellation
func TestCondaManager_CreateEnvironment_ContextCancellation(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: GetEnvironment (check if exists)
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Second call: CreateEnvironment - simulate long-running operation
			exec := createMockExecution("", "", 0, nil)
			// Cancel the context to simulate cancellation
			go func() {
				<-ctx.Done()
				exec.Cancel()
			}()
			return exec, nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	_, err := manager.CreateEnvironment(ctx, "testenv", "3.9")
	assert.Error(t, err)
	assert.True(t, ctx.Err() == context.Canceled || strings.Contains(err.Error(), "cancel"))
}

// TestCondaManager_RemoveEnvironment_ContextCancellation tests handling context cancellation during removal
func TestCondaManager_RemoveEnvironment_ContextCancellation(t *testing.T) {
	callCount := 0
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			callCount++
			if callCount == 1 {
				output := "# conda environments:\n#\nbase                  /opt/conda\nmyenv                 /opt/conda/envs/myenv\n"
				return createMockExecution(output, "", 0, nil), nil
			} else {
				// Simulate cancellation
				exec := createMockExecution("", "", 0, nil)
				go func() {
					<-ctx.Done()
					exec.Cancel()
				}()
				return exec, nil
			}
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.RemoveEnvironment(ctx, "myenv")
	assert.Error(t, err)
}

// TestCondaManager_CreateEnvironment_WithSpacesInName tests handling spaces in environment name
func TestCondaManager_CreateEnvironment_WithSpacesInName(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: GetEnvironment (check if exists)
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Second call: CreateEnvironment - may fail with spaces
			return createMockExecution("", "error: invalid environment name", 1, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	_, err := manager.CreateEnvironment(ctx, "my env", "3.9")
	// May fail due to spaces in name
	assert.Error(t, err)
}

// TestCondaManager_ListEnvironments_StreamingInterrupted tests handling interrupted streaming
func TestCondaManager_ListEnvironments_StreamingInterrupted(t *testing.T) {
	if !ShouldRunCondaTests() {
		t.Skip("Skipping TestCondaManager_ListEnvironments_StreamingInterrupted - requires conda. Run in Docker or set CONDATEST_ENABLED=true")
	}

	realExec := service.NewRawExecutor()
	manager := NewCondaManager(realExec, "conda")
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately to interrupt
	cancel()

	_, err := manager.ListEnvironments(ctx)
	assert.Error(t, err)
}

// TestCondaManager_CreateEnvironment_WithUnicodeName tests handling unicode characters in name
func TestCondaManager_CreateEnvironment_WithUnicodeName(t *testing.T) {
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			// First call: GetEnvironment (check if exists)
			if strings.Contains(req.Command, "env") && strings.Contains(strings.Join(req.Args, " "), "list") {
				return createMockExecution("# conda environments:\n#\nbase                  /opt/conda\n", "", 0, nil), nil
			}
			// Second call: CreateEnvironment - may fail with unicode
			return createMockExecution("", "error: invalid environment name", 1, nil), nil
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	_, err := manager.CreateEnvironment(ctx, "测试环境", "3.9")
	// May fail due to unicode characters
	assert.Error(t, err)
}

// TestCondaManager_GetEnvironment_AfterRemoval tests getting environment after it's been removed
func TestCondaManager_GetEnvironment_AfterRemoval(t *testing.T) {
	callCount := 0
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			callCount++
			if callCount == 1 {
				// First call: ListEnvironments - env exists
				output := "# conda environments:\n#\nbase                  /opt/conda\nmyenv                 /opt/conda/envs/myenv\n"
				return createMockExecution(output, "", 0, nil), nil
			} else if callCount == 2 {
				// Second call: RemoveEnvironment - succeeds
				return createMockExecution("Executing transaction: done", "", 0, nil), nil
			} else {
				// Third call: GetEnvironment after removal - env not found
				output := "# conda environments:\n#\nbase                  /opt/conda\n"
				return createMockExecution(output, "", 0, nil), nil
			}
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	// Remove environment
	err := manager.RemoveEnvironment(ctx, "myenv")
	require.NoError(t, err)

	// Try to get it - should fail
	_, err = manager.GetEnvironment(ctx, "myenv")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestCondaManager_InstallPackage_WithExtraArgs tests that extra args in package name are handled
func TestCondaManager_InstallPackage_WithExtraArgs(t *testing.T) {
	callCount := 0
	mockExec := &mockExecutor{
		executeFunc: func(ctx context.Context, req service.RawExecutionRequest) (*service.RawExecution, error) {
			callCount++
			if callCount == 1 {
				output := "# conda environments:\n#\nbase                  /opt/conda\nmyenv                 /opt/conda/envs/myenv\n"
				return createMockExecution(output, "", 0, nil), nil
			} else {
				// Verify package name with extra args is passed
				args := strings.Join(req.Args, " ")
				// Should contain the full package specification
				assert.Contains(t, args, "requests")
				return createMockExecution("Successfully installed requests", "", 0, nil), nil
			}
		},
	}

	manager := NewCondaManager(mockExec, "conda")
	ctx := context.Background()

	err := manager.InstallPackage(ctx, "myenv", "requests[security]")
	assert.NoError(t, err)
}
