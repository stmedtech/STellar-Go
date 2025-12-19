//go:build conda

package conda

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Handler Subcommand Tests (Require Actual Conda)
// ============================================================================

func setupTestHandler(t *testing.T) (*CondaHandler, *CondaOperations) {
	ops, err := NewCondaOperations("")
	require.NoError(t, err)
	return NewCondaHandler(ops), ops
}

func TestCondaHandler_HandleSubcommand_List(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "list", []string{}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "base")
}

func TestCondaHandler_HandleSubcommand_Get(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listExec, err := handler.HandleSubcommand(ctx, "list", []string{}, nil)
	require.NoError(t, err)
	listOut, _, _, _ := readExecutionOutput(listExec.(*CommandExecution))

	lines := strings.Split(listOut, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			envName := strings.TrimPrefix(parts[0], "*")
			exec, err := handler.HandleSubcommand(ctx, "get", []string{envName}, nil)
			if err == nil {
				stdout, _, exitCode, err := readExecutionOutput(exec.(*CommandExecution))
				require.NoError(t, err)
				assert.Equal(t, 0, exitCode)
				assert.NotEmpty(t, strings.TrimSpace(stdout))
				return
			}
		}
	}
	t.Fatalf("No conda environments found. List output: %s", listOut)
}

func TestCondaHandler_HandleSubcommand_Create(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	envName := generateTestEnvName("handler-create")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "create", []string{envName}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	assertExecutionSuccess(t, exec.(*CommandExecution), "Environment creation")

	removeExec, _ := handler.HandleSubcommand(ctx, "remove", []string{envName}, nil)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec.(*CommandExecution))
	}
}

func TestCondaHandler_HandleSubcommand_Create_WithPythonFlag(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	envName := generateTestEnvName("handler-create-python")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "create", []string{envName, "--python", "3.10"}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	assertExecutionSuccess(t, exec.(*CommandExecution), "Environment creation")

	removeExec, _ := handler.HandleSubcommand(ctx, "remove", []string{envName}, nil)
	if removeExec != nil {
		_, _, _, _ = readExecutionOutput(removeExec.(*CommandExecution))
	}
}

func TestCondaHandler_HandleSubcommand_Remove(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-remove")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	removeExec, err := handler.HandleSubcommand(ctx, "remove", []string{envName}, nil)
	require.NoError(t, err)
	require.NotNil(t, removeExec)

	assertExecutionSuccess(t, removeExec.(*CommandExecution), "Environment removal")
}

func TestCondaHandler_HandleSubcommand_Update(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-update")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	yamlPath := "/tmp/test-env.yml"
	yamlContent := "name: test-env\ndependencies:\n  - python=3.9\n"
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)
	defer os.Remove(yamlPath)

	updateExec, err := handler.HandleSubcommand(ctx, "update", []string{envName, yamlPath}, nil)
	require.NoError(t, err)
	require.NotNil(t, updateExec)

	_, _, _, _ = readExecutionOutput(updateExec.(*CommandExecution))

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaHandler_HandleSubcommand_Install(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-install")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	installExec, err := handler.HandleSubcommand(ctx, "install", []string{envName, "requests"}, nil)
	require.NoError(t, err)
	require.NotNil(t, installExec)

	_, _, _, _ = readExecutionOutput(installExec.(*CommandExecution))

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaHandler_HandleSubcommand_Path(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "path", []string{}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.NotEmpty(t, strings.TrimSpace(stdout))
}

func TestCondaHandler_HandleSubcommand_Version(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "version", []string{}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "conda")
}

func TestCondaHandler_HandleSubcommand_RunPython(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-runpython")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	runExec, err := handler.HandleSubcommand(ctx, "run-python", []string{envName, "print('Hello')"}, nil)
	require.NoError(t, err)
	require.NotNil(t, runExec)

	stdout, _, exitCode, err := readExecutionOutput(runExec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "Hello")

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaHandler_HandleSubcommand_RunScript(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-runscript")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	scriptPath := "/tmp/test-script.py"
	scriptContent := "#!/usr/bin/env python\nprint('Script output')\n"
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	runExec, err := handler.HandleSubcommand(ctx, "run-script", []string{envName, scriptPath}, nil)
	require.NoError(t, err)
	require.NotNil(t, runExec)

	stdout, _, exitCode, err := readExecutionOutput(runExec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "Script output")

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaHandler_HandleSubcommand_RunScript_WithArgs(t *testing.T) {
	requireCondaAvailable(t)

	handler, ops := setupTestHandler(t)
	envName := generateTestEnvName("handler-runscript")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createTestEnvironment(t, ops, ctx, envName)

	scriptPath := "/tmp/test-script-args.py"
	scriptContent := "#!/usr/bin/env python\nimport sys\nprint('Args:', sys.argv[1:])\n"
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)
	defer os.Remove(scriptPath)

	runExec, err := handler.HandleSubcommand(ctx, "run-script", []string{envName, scriptPath, "arg1", "arg2"}, nil)
	require.NoError(t, err)
	require.NotNil(t, runExec)

	stdout, _, exitCode, err := readExecutionOutput(runExec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "arg1")
	assert.Contains(t, stdout, "arg2")

	cleanupTestEnvironment(t, ops, ctx, envName)
}

func TestCondaHandler_HandleSubcommand_Run(t *testing.T) {
	requireCondaAvailable(t)

	handler, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "run", []string{"info"}, nil)
	require.NoError(t, err)
	require.NotNil(t, exec)

	stdout, _, exitCode, err := readExecutionOutput(exec.(*CommandExecution))
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "conda")
}
