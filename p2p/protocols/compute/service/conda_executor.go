package service

import (
	"context"
	"fmt"
)

// CondaExecutor wraps a base Executor and activates conda environments
// when CONDA_ENV is set in the request's environment variables.
type CondaExecutor struct {
	baseExecutor Executor
	condaPath    string
}

// NewCondaExecutor creates a new CondaExecutor that wraps the given base executor.
// condaPath is the path to the conda executable (e.g., "conda" or "/usr/bin/conda").
func NewCondaExecutor(baseExecutor Executor, condaPath string) *CondaExecutor {
	return &CondaExecutor{
		baseExecutor: baseExecutor,
		condaPath:    condaPath,
	}
}

// ExecuteRaw executes a command, optionally within a conda environment.
// If req.Env contains "CONDA_ENV" with a non-empty value, the command is wrapped
// with "conda run -n <env> <command> <args>".
// Otherwise, the command is executed directly via the base executor.
func (e *CondaExecutor) ExecuteRaw(ctx context.Context, req RawExecutionRequest) (*RawExecution, error) {
	// Check if base executor is nil
	if e.baseExecutor == nil {
		return nil, fmt.Errorf("base executor is nil")
	}

	// Check if CONDA_ENV is set and not empty
	condaEnv, hasCondaEnv := req.Env["CONDA_ENV"]
	if !hasCondaEnv || condaEnv == "" {
		// No conda environment specified, delegate directly to base executor
		return e.baseExecutor.ExecuteRaw(ctx, req)
	}

	// Check if conda path is empty
	if e.condaPath == "" {
		return nil, fmt.Errorf("conda path is empty")
	}

	// Wrap command with conda run
	// Command becomes: conda run -n <env> <original_command> <original_args>
	wrappedReq := RawExecutionRequest{
		Command:    e.condaPath,
		Args:       append([]string{"run", "-n", condaEnv, req.Command}, req.Args...),
		Env:        make(map[string]string),
		WorkingDir: req.WorkingDir,
		Stdin:      req.Stdin,
	}

	// Copy all environment variables except CONDA_ENV (which is handled by conda run)
	for k, v := range req.Env {
		if k != "CONDA_ENV" {
			wrappedReq.Env[k] = v
		}
	}

	// Execute via base executor
	return e.baseExecutor.ExecuteRaw(ctx, wrappedReq)
}
