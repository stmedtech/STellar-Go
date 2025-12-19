package service

import (
	"context"
	"io"
	"stellar/p2p/protocols/compute/streams"
)

// CondaOperationsCreator is a function type that creates CondaOperations for server-side use
// This avoids import cycle: server can use this without importing conda package
type CondaOperationsCreator func(condaPath string) (interface{}, error)

// CondaOperations defines the interface for conda operations
// This interface is used by conda_handler_wrapper to break import cycles
type CondaOperations interface {
	// Pre-conda preparation methods (work remotely via Executor)
	Install(ctx context.Context, pythonVersion string) (streams.ExecutionStreamReader, error)
	CommandPath(ctx context.Context) (string, error)
	GetCondaVersion(ctx context.Context) (streams.ExecutionStreamReader, error)

	// Environment management methods (work remotely via Executor)
	ListEnvironments(ctx context.Context) (streams.ExecutionStreamReader, error)
	GetEnvironment(ctx context.Context, name string) (string, error)
	CreateEnvironment(ctx context.Context, name, pythonVersion string) (streams.ExecutionStreamReader, error)
	RemoveEnvironment(ctx context.Context, name string) (streams.ExecutionStreamReader, error)
	UpdateEnvironment(ctx context.Context, name, yamlPath string) (streams.ExecutionStreamReader, error)
	InstallPackage(ctx context.Context, envName, packageName string) (streams.ExecutionStreamReader, error)

	// Python execution methods (streaming I/O)
	RunPython(ctx context.Context, env, code string, stdin io.Reader) (streams.ExecutionStreamReader, error)
	RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error)

	// Raw conda command execution
	RunConda(ctx context.Context, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error)
}

var condaOperationsCreator CondaOperationsCreator

// SetCondaOperationsCreator allows the conda package to register its creator
func SetCondaOperationsCreator(creator CondaOperationsCreator) {
	condaOperationsCreator = creator
}

// GetCondaOperationsCreator returns the registered creator
func GetCondaOperationsCreator() CondaOperationsCreator {
	return condaOperationsCreator
}
