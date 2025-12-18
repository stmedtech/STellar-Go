# Conda Architecture - Clean Design (Redesigned)

## Core Principles

1. **Single Source of Truth**: All conda domain logic in `operations.go` (plain Go, no Executor)
2. **Separation of Concerns**: Domain logic, protocol, and integration are separate
3. **DRY**: No duplication between local and remote
4. **Simplicity**: Minimal abstractions, clear responsibilities
5. **Clean Architecture**: Domain logic independent of infrastructure

## Architecture Layers

### 1. Core Protocol Layer (`service/`)
- **RawExecutor**: Executes commands locally via `os/exec`
- **Executor Interface**: Abstraction for command execution
- **Client/Server**: Protocol for remote command execution
- **No conda-specific logic here** - pure protocol layer

### 2. Conda Domain Layer (`conda/operations.go`)
- **CondaOperations**: All conda business logic
- Uses plain Go (`os/exec`) - **no Executor dependency**
- Single file with all operations:
  - **Pre-conda**: Install, CommandPath, GetCondaVersion
  - **Environment**: List, Get, Create, Remove, Update, InstallPackage
- **This is the ONLY place with conda business logic**
- Uses `constant.StellarPath()` for path detection (no hardcoded paths)

### 3. Conda Integration Layer
- **Removed**: CondaExecutor is no longer needed
- All conda operations use explicit `__conda` commands
- No CONDA_ENV environment variable pattern - use `__conda run <command>` instead

### 4. Remote Conda Layer
- **Server** (`server.go`): `__conda` handler routes to CondaOperations via CondaHandler
- **Client**: Uses `client.Run()` directly with `__conda` commands (no wrapper needed)
- **No Executor dependency** - uses protocol directly
- Factory pattern avoids import cycles

### 5. CLI Layer (`cmd/stellar/conda.go`)
- Directly calls CondaOperations
- No Executor needed
- Simple and straightforward

## What We Removed

1. **CondaManager** (`conda/manager.go`): Redundant - used Executor, replaced by CondaOperations
2. **RemoteExecutor** (`remote_executor.go`): Not needed - clients use protocol directly
3. **CondaService** (`conda_service.go`): Not needed - clients can use `client.Run(__conda ...)` directly
4. **CondaExecutor** (`conda_executor.go`): Not needed - use explicit `__conda run <command>` instead of CONDA_ENV pattern
5. **CondaOperationsFactory**: Not needed - direct instantiation via Creator
6. **All Executor-based conda code**: Replaced by plain Go operations

## Data Flow

### Local Execution
```
CLI → CondaOperations → os/exec → conda command
```

### Remote Conda Operations
```
Client → client.Run(__conda ...) → Server → CondaHandler → CondaOperations → os/exec → conda command
```

### Python Execution with Conda
```
Client → client.Run(__conda run-python <env> <code>) → Server → CondaHandler → CondaOperations → os/exec → conda run -n <env> python -c <code>
```

## File Structure

```
service/
  ├── executor.go              # RawExecutor (core protocol)
  ├── types.go                 # Executor interface, RawExecution
  ├── client.go                # Compute client
  ├── server.go                # Compute server + __conda handler
  ├── conda_operations.go      # Factory for server-side (avoids import cycle)
  ├── conda_handler_wrapper.go # Wrapper for server-side handler (avoids import cycle)
  └── conda/
      ├── operations.go        # ALL conda domain logic (plain Go)
      ├── handler.go           # CondaHandler (single source of truth for command handling)
      ├── infrastructure.go    # Path finding, URL generation
      ├── conda.go             # Download helper
      └── conda_service_init.go # Registers factory

cmd/stellar/
  └── conda.go                 # CLI (calls CondaOperations directly)
```

## Key Design Decisions

1. **Plain Go for Domain Logic**: CondaOperations uses `os/exec` directly, no Executor abstraction
2. **Protocol for Remote**: Remote operations use `__conda` protocol, not Executor interface
3. **Factory Pattern**: Server uses factory to avoid import cycles
4. **Explicit Commands**: All conda operations use explicit `__conda` commands (no CONDA_ENV pattern)
5. **Single Source of Truth**: All conda logic in one file (`operations.go`)

## Benefits

1. **Simple**: Clear separation, minimal abstractions
2. **Testable**: Domain logic isolated, easy to test
3. **Maintainable**: Single source of truth, no duplication
4. **Reliable**: Plain Go operations, no complex dependencies
5. **Clean**: Follows clean architecture principles
6. **DRY**: No code duplication between local and remote

## Migration Notes

- **CondaManager**: Deprecated, use CondaOperations directly
- **RemoteExecutor**: Deprecated, use `client.Run(__conda ...)` directly
- **CondaService**: Deprecated, use `client.Run(__conda ...)` directly
- **CondaExecutor**: Deprecated, use `__conda run <command>` instead of CONDA_ENV pattern
- **CondaOperationsFactory**: Deprecated, use CondaOperationsCreator
