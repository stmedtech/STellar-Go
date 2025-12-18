package service

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// condaHandlerWrapper wraps conda operations to provide HandleSubcommand interface
// This avoids import cycles while allowing server to use shared handler pattern
// Used when CondaHandler is not directly available (type assertion fails)
type condaHandlerWrapper struct {
	ops interface {
		ListEnvironments(ctx context.Context) (ExecutionStreamReader, error)
		GetEnvironment(ctx context.Context, name string) (string, error)
		CreateEnvironment(ctx context.Context, name, pythonVersion string) (ExecutionStreamReader, error)
		RemoveEnvironment(ctx context.Context, name string) (ExecutionStreamReader, error)
		UpdateEnvironment(ctx context.Context, name, yamlPath string) (ExecutionStreamReader, error)
		InstallPackage(ctx context.Context, envName, packageName string) (ExecutionStreamReader, error)
		CommandPath(ctx context.Context) (string, error)
		GetCondaVersion(ctx context.Context) (ExecutionStreamReader, error)
		Install(ctx context.Context, pythonVersion string) (ExecutionStreamReader, error)
		RunPython(ctx context.Context, env, code string, stdin io.Reader) (ExecutionStreamReader, error)
		RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (ExecutionStreamReader, error)
		RunConda(ctx context.Context, args []string, stdin io.Reader) (ExecutionStreamReader, error)
	}
}

// HandleSubcommand handles a conda subcommand
// Implements the same interface as conda.CondaHandler for server-side use
func (w *condaHandlerWrapper) HandleSubcommand(ctx context.Context, subcommand string, args []string, stdin io.Reader) (ExecutionStreamReader, error) {
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
		pythonVersion := "3.13" // default
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
func (w *condaHandlerWrapper) createStringOutputExecution(output string) ExecutionStreamReader {
	done := make(chan error, 1)
	exitCode := make(chan int, 1)
	done <- nil
	exitCode <- 0
	close(done)
	close(exitCode)

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
