package service

import (
	"context"
	"fmt"
	"io"
	"stellar/p2p/protocols/compute/streams"
	"strings"
)

// condaHandlerWrapper wraps conda operations to provide HandleSubcommand interface
// This avoids import cycles while allowing server to use shared handler pattern
// Used when CondaHandler is not directly available (type assertion fails)
type condaHandlerWrapper struct {
	ops interface {
		ListEnvironments(ctx context.Context) (streams.ExecutionStreamReader, error)
		GetEnvironment(ctx context.Context, name string) (string, error)
		CreateEnvironment(ctx context.Context, name, pythonVersion string) (streams.ExecutionStreamReader, error)
		RemoveEnvironment(ctx context.Context, name string) (streams.ExecutionStreamReader, error)
		UpdateEnvironment(ctx context.Context, name, yamlPath string) (streams.ExecutionStreamReader, error)
		InstallPackage(ctx context.Context, envName, packageName string) (streams.ExecutionStreamReader, error)
		CommandPath(ctx context.Context) (string, error)
		GetCondaVersion(ctx context.Context) (streams.ExecutionStreamReader, error)
		Install(ctx context.Context, pythonVersion string) (streams.ExecutionStreamReader, error)
		RunPython(ctx context.Context, env, code string, stdin io.Reader) (streams.ExecutionStreamReader, error)
		RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error)
		RunConda(ctx context.Context, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error)
	}
}

// HandleSubcommand handles a conda subcommand
// Implements the same interface as conda.CondaHandler for server-side use
func (w *condaHandlerWrapper) HandleSubcommand(ctx context.Context, subcommand string, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error) {
	switch subcommand {
	case "list":
		return w.ops.ListEnvironments(ctx)

	case "get":
		if len(args) < 1 {
			return nil, fmt.Errorf("environment name required")
		}
		path, err := w.ops.GetEnvironment(ctx, args[0])
		if err != nil {
			return nil, err
		}
		// Create a simple execution that outputs the path
		return w.createStringOutputExecution(path), nil

	case "create":
		if len(args) < 1 {
			return nil, fmt.Errorf("environment name required")
		}
		envName := args[0]
		pythonVersion := "3.9" // default - use stable version that's widely available
		// Parse --python flag if present
		for i, arg := range args {
			if arg == "--python" && i+1 < len(args) {
				pythonVersion = args[i+1]
				break
			}
		}
		return w.ops.CreateEnvironment(ctx, envName, pythonVersion)

	case "remove":
		if len(args) < 1 {
			return nil, fmt.Errorf("environment name required")
		}
		return w.ops.RemoveEnvironment(ctx, args[0])

	case "update":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and YAML file required")
		}
		return w.ops.UpdateEnvironment(ctx, args[0], args[1])

	case "install":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and package name required")
		}
		return w.ops.InstallPackage(ctx, args[0], args[1])

	case "path":
		path, err := w.ops.CommandPath(ctx)
		if err != nil {
			return nil, err
		}
		return w.createStringOutputExecution(path), nil

	case "version":
		return w.ops.GetCondaVersion(ctx)

	case "install-conda":
		pythonVersion := "py313" // default
		if len(args) > 0 {
			pythonVersion = args[0]
		}
		return w.ops.Install(ctx, pythonVersion)

	case "run-python":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and code required")
		}
		return w.ops.RunPython(ctx, args[0], args[1], stdin)

	case "run-script":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and script path required")
		}
		envName := args[0]
		scriptPath := args[1]
		scriptArgs := args[2:]
		return w.ops.RunScript(ctx, envName, scriptPath, scriptArgs, stdin)

	case "run":
		// Raw conda command: __conda run <conda-args...>
		if len(args) == 0 {
			return nil, fmt.Errorf("conda command requires at least one argument")
		}
		return w.ops.RunConda(ctx, args, stdin)

	default:
		return nil, fmt.Errorf("unknown conda subcommand: %s", subcommand)
	}
}

// createStringOutputExecution creates an execution that outputs a string
// Note: Channels are kept open (not closed) so multiple readers can get the values
// This prevents race conditions where monitorExecution and handleCondaOperation
// both try to read from the same channels
func (w *condaHandlerWrapper) createStringOutputExecution(output string) streams.ExecutionStreamReader {
	done := make(chan error, 1)
	exitCode := make(chan int, 1)
	done <- nil
	exitCode <- 0
	// Don't close channels - keep them open so multiple readers can get the values
	// The channels are buffered, so the values are available to all readers

	return &stringOutputExecution{
		output:   output + "\n",
		done:     done,
		exitCode: exitCode,
	}
}

// stringOutputExecution is a simple execution that outputs a string
type stringOutputExecution struct {
	output   string
	done     <-chan error
	exitCode <-chan int
}

func (e *stringOutputExecution) GetStdout() io.ReadCloser {
	return io.NopCloser(strings.NewReader(e.output))
}

func (e *stringOutputExecution) GetStderr() io.ReadCloser {
	return io.NopCloser(strings.NewReader(""))
}

func (e *stringOutputExecution) GetDone() <-chan error {
	return e.done
}

func (e *stringOutputExecution) GetExitCode() <-chan int {
	return e.exitCode
}
