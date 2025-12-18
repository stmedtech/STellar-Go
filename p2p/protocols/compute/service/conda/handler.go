package conda

import (
	"context"
	"fmt"
	"io"
	"strings"

	"stellar/p2p/protocols/compute/service"
)

// CondaHandler handles conda subcommands with streaming I/O
// This is a shared handler used by both CLI and server-side execution
// All operations return CommandExecution for raw streaming
type CondaHandler struct {
	ops *CondaOperations
}

// NewCondaHandler creates a new conda handler
func NewCondaHandler(ops *CondaOperations) *CondaHandler {
	return &CondaHandler{ops: ops}
}

// HandleSubcommand handles a conda subcommand with streaming I/O
// Returns executionStreamReader for operations that execute commands
// Returns error for validation errors
// This is the single source of truth for conda command handling
func (h *CondaHandler) HandleSubcommand(ctx context.Context, subcommand string, args []string, stdin io.Reader) (service.ExecutionStreamReader, error) {
	switch subcommand {
	case "list":
		return h.ops.ListEnvironments(ctx)

	case "get":
		if len(args) < 1 {
			return nil, fmt.Errorf("environment name required")
		}
		// GetEnvironment returns a string, not CommandExecution
		// We need to handle this differently - return nil and let caller handle
		path, err := h.ops.GetEnvironment(ctx, args[0])
		if err != nil {
			return nil, err
		}
		// Create a dummy execution that outputs the path
		return h.createStringOutputExecution(path), nil

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
		return h.ops.CreateEnvironment(ctx, envName, pythonVersion)

	case "remove":
		if len(args) < 1 {
			return nil, fmt.Errorf("environment name required")
		}
		return h.ops.RemoveEnvironment(ctx, args[0])

	case "update":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and YAML file required")
		}
		return h.ops.UpdateEnvironment(ctx, args[0], args[1])

	case "install":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and package name required")
		}
		return h.ops.InstallPackage(ctx, args[0], args[1])

	case "path":
		path, err := h.ops.CommandPath(ctx)
		if err != nil {
			return nil, err
		}
		// Create a dummy execution that outputs the path
		return h.createStringOutputExecution(path), nil

	case "version":
		return h.ops.GetCondaVersion(ctx)

	case "install-conda":
		pythonVersion := "py313" // default
		if len(args) > 0 {
			pythonVersion = args[0]
		}
		return h.ops.Install(ctx, pythonVersion)

	case "run-python":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and code required")
		}
		return h.ops.RunPython(ctx, args[0], args[1], stdin)

	case "run-script":
		if len(args) < 2 {
			return nil, fmt.Errorf("environment name and script path required")
		}
		envName := args[0]
		scriptPath := args[1]
		scriptArgs := args[2:]
		return h.ops.RunScript(ctx, envName, scriptPath, scriptArgs, stdin)

	case "run":
		// Raw conda command: __conda run <conda-args...>
		// Execute conda with any arguments provided
		if len(args) == 0 {
			return nil, fmt.Errorf("conda command requires at least one argument")
		}
		return h.ops.RunConda(ctx, args, stdin)

	default:
		return nil, fmt.Errorf("unknown conda subcommand: %s", subcommand)
	}
}

// PrintUsage prints the usage information for conda commands
// This is the single source of truth for conda command documentation
func (h *CondaHandler) PrintUsage() string {
	return `Usage: stellar conda <subcommand> [options]

Conda Installation & Setup:
  install-conda [version] Download and install conda (default: py313)
  path                    Show conda executable path
  version                 Show conda version

Environment Management:
  list                    List all conda environments
  create <name> [--python <version>] Create a new conda environment
  remove <name>           Remove a conda environment
  update <name> <yaml>    Update environment from YAML file
  install <env> <pkg>     Install a package in an environment
  get <name>              Get the path of an environment

Python Execution:
  run-python <env> <code> Execute Python code in an environment
  run-script <env> <script> [args...] Execute a Python script in an environment

Raw Conda Commands:
  run <conda-args...>     Execute raw conda command with any arguments

Options:
  --quiet                 Suppress progress output (for create, remove, update, install, install-conda)

Examples:
  stellar conda install-conda py313
  stellar conda path
  stellar conda version
  stellar conda list
  stellar conda create --python 3.13 myenv
  stellar conda create --python 3.13 myenv --quiet
  stellar conda remove myenv
  stellar conda update myenv environment.yml
  stellar conda install myenv numpy
  stellar conda get myenv
  stellar conda run-python myenv "print('Hello, World!')"
  stellar conda run-script myenv script.py arg1 arg2
  stellar conda run env list
  stellar conda run create -n myenv python=3.9
`
}

// createStringOutputExecution creates a CommandExecution that outputs a string
// Used for operations that return simple values (get, path)
func (h *CondaHandler) createStringOutputExecution(output string) *CommandExecution {
	done := make(chan error, 1)
	exitCode := make(chan int, 1)
	done <- nil
	exitCode <- 0
	close(done)
	close(exitCode)

	return &CommandExecution{
		RunID:    generateCommandRunID(),
		Stdin:    nil,
		Stdout:   io.NopCloser(strings.NewReader(output + "\n")),
		Stderr:   io.NopCloser(strings.NewReader("")),
		Done:     done,
		ExitCode: exitCode,
		Cancel:   func() {},
	}
}
