package conda

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Handler Creation Tests
// ============================================================================

func TestNewCondaHandler(t *testing.T) {
	// Use mock operations for unit tests (no conda required)
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	require.NotNil(t, handler)
	assert.NotNil(t, handler.ops)
}

// ============================================================================
// Handler Subcommand Tests (Unit Tests - No Conda Required)
// ============================================================================

func TestCondaHandler_HandleSubcommand_Get_EmptyArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "get", []string{}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "required")
}

func TestCondaHandler_HandleSubcommand_Create_EmptyArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "create", []string{}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "required")
}

func TestCondaHandler_HandleSubcommand_Update_InvalidArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "update", []string{"env-name"}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "required")
}

func TestCondaHandler_HandleSubcommand_Install_InvalidArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "install", []string{"env-name"}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "required")
}

func TestCondaHandler_HandleSubcommand_RunPython_InvalidArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "run-python", []string{"env-name"}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "required")
}

func TestCondaHandler_HandleSubcommand_Run_EmptyArgs(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "run", []string{}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "at least one argument")
}

func TestCondaHandler_HandleSubcommand_Unknown(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := handler.HandleSubcommand(ctx, "unknown-command", []string{}, nil)
	require.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "unknown")
}

// ============================================================================
// PrintUsage Tests
// ============================================================================

func TestCondaHandler_PrintUsage(t *testing.T) {
	ops := NewMockCondaOperations()

	handler := NewCondaHandler(ops)
	usage := handler.PrintUsage()

	assert.Contains(t, usage, "Usage: stellar conda")
	assert.Contains(t, usage, "install-conda")
	assert.Contains(t, usage, "list")
	assert.Contains(t, usage, "create")
	assert.Contains(t, usage, "run-python")
	assert.Contains(t, usage, "run-script")
	assert.Contains(t, usage, "run")
}
