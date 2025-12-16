# Unified Conda Operations Architecture

## Overview

This document outlines the architecture for combining local and remote conda operations using clean architecture principles and design patterns.

## Requirements

1. **Pre-conda Features** (must work both locally and remotely):
   - Install conda (`Install()`)
   - Get conda path (`FindCondaPath()`, `CommandPath()`)
   - Get conda version (`GetCondaVersion()`, `Version()`)
   
   **Note**: `DownloadCondaInstaller()` and deprecated `DownloadUrl()` are excluded from remote API planning as they are internal implementation details for `Install()`.

2. **Environment Management** (must work both locally and remotely):
   - List environments (`ListEnvironments()`)
   - Get environment path (`GetEnvironment()`)
   - Create environment (`CreateEnvironment()`)
   - Remove environment (`RemoveEnvironment()`)
   - Update environment (`UpdateEnvironment()`)
   - Install package (`InstallPackage()`)

3. **Remote Execution**:
   - Use compute protocol for remote operations
   - Since direct `conda` command access requires absolute path, use a custom command prefix
   - Redirect commands to actual conda command remotely

## Architecture Design

### Design Patterns

1. **Strategy Pattern**: Different execution strategies (local vs remote)
2. **Adapter Pattern**: Adapt compute protocol to conda operations
3. **Facade Pattern**: Unified interface for all conda operations
4. **Dependency Injection**: Inject execution strategy

### Component Structure

```
┌─────────────────────────────────────────────────────────┐
│              CondaOperations Interface                   │
│  (Unified interface for all conda operations)          │
└─────────────────────────────────────────────────────────┘
                          ▲
                          │
        ┌─────────────────┴─────────────────┐
        │                                   │
┌───────────────────┐            ┌───────────────────┐
│  LocalCondaOps    │            │  RemoteCondaOps   │
│  (Strategy)       │            │  (Strategy)       │
│                   │            │                   │
│  Uses:            │            │  Uses:           │
│  - CondaManager   │            │  - CondaService   │
│  - RawExecutor    │            │  - Compute Client │
│  - Infrastructure │            │  - Command Router │
└───────────────────┘            └───────────────────┘
```

### Unified Interface

```go
// CondaOperations defines the unified interface for all conda operations
type CondaOperations interface {
    // Pre-conda features
    InstallConda(ctx context.Context, version string) error
    GetCondaPath(ctx context.Context) (string, error)
    GetCondaVersion(ctx context.Context) (string, error)
    
    // Environment management
    ListEnvironments(ctx context.Context) (map[string]string, error)
    GetEnvironment(ctx context.Context, name string) (string, error)
    CreateEnvironment(ctx context.Context, name, pythonVersion string) (string, error)
    RemoveEnvironment(ctx context.Context, name string) error
    UpdateEnvironment(ctx context.Context, name, yamlPath string) error
    InstallPackage(ctx context.Context, envName, packageName string) error
    
    // Python execution
    RunPython(ctx context.Context, envName, code string, stdin io.Reader) (*service.RawExecutionHandle, error)
    RunScript(ctx context.Context, envName, scriptPath string, args []string, stdin io.Reader) (*service.RawExecutionHandle, error)
}
```

### Remote Command Routing

Since direct `conda` command access requires absolute path, we'll use a command routing mechanism:

1. **Command Prefix**: Use a special prefix like `__conda__` or detect conda commands
2. **Server-Side Routing**: Server intercepts conda commands and routes to actual conda executable
3. **Path Resolution**: Server resolves conda path automatically using `FindCondaPath()`

**Example Flow:**
```
Client: Run("conda", ["env", "list"])
  ↓
Server: Detects "conda" command
  ↓
Server: Resolves conda path using FindCondaPath()
  ↓
Server: Executes: /absolute/path/to/conda env list
  ↓
Server: Returns output
```

## Implementation Plan

### Phase 6A: Extend CondaService

**Goal**: Add all pre-conda features and environment management to CondaService

**Tasks**:
1. Extend `CondaService` with pre-conda methods:
   - `InstallConda()` - Remote conda installation
   - `GetCondaPath()` - Get remote conda path
   - `GetCondaVersion()` - Get remote conda version
   
   **Note**: `DownloadCondaInstaller()` is excluded as it's an internal implementation detail. `InstallConda()` will handle the full installation process internally.

2. Add missing environment management methods:
   - `UpdateEnvironment()` - Already exists, verify completeness
   - Ensure all methods match `CondaManager` interface

3. Refactor to remove CONDA_ENV:
   - Remove CONDA_ENV from CondaExecutor
   - Update CondaService methods to use `conda run -n <envName>` directly
   - All methods take explicit `envName` parameter

**Files**:
- `p2p/protocols/compute/service/conda_service.go` (extend)
- `p2p/protocols/compute/service/conda_service_test.go` (extend)
- `p2p/protocols/compute/service/server.go` (add command routing)

### Phase 6C: Create Unified Interface

**Goal**: Create unified interface and implementations

**Tasks**:
1. Define `CondaOperations` interface
2. Implement `LocalCondaOps` using `CondaManager`
3. Implement `RemoteCondaOps` using `CondaService`
4. Create factory function to choose strategy

**Files**:
- `core/conda/operations.go` (new - interface and implementations)
- `core/conda/operations_test.go` (new)

### Phase 6D: Update CLI

**Goal**: Update CLI to use unified interface

**Tasks**:
1. Update `cmd/stellar/conda.go` to use `CondaOperations`
2. Add remote device selection
3. Route commands to local or remote based on context

**Files**:
- `cmd/stellar/conda.go` (refactor)

### Phase 6B: Server Command Routing & Remove CONDA_ENV

**Goal**: Implement automatic conda command routing and remove CONDA_ENV dependency

**Tasks**:
1. Add automatic conda command detection in server (Option 1)
2. Resolve conda path using `FindCondaPath()` on server
3. Route conda commands to absolute path
4. Remove CONDA_ENV from CondaExecutor
5. Update CondaService to use `conda run -n <envName>` directly
6. Update all tests to use explicit envName instead of CONDA_ENV
7. Handle errors gracefully (conda not found)

**Files**:
- `p2p/protocols/compute/service/server.go` (add command routing)
- `p2p/protocols/compute/service/conda_executor.go` (remove CONDA_ENV)
- `p2p/protocols/compute/service/conda_service.go` (update to use conda run directly)
- `p2p/protocols/compute/service/conda_executor_test.go` (update tests)
- `p2p/protocols/compute/service/conda_service_test.go` (update tests)
- `p2p/protocols/compute/service/conda_integration_test.go` (update tests)

## Command Routing Strategy

**Selected: Option 1 - Automatic Detection**

Server automatically detects `conda` commands and resolves path:

```go
// In server.go handleRun()
// Auto-detect conda commands and resolve path
if req.Command == "conda" {
    // Try to resolve conda path
    if condaPath, err := conda.FindCondaPath(); err == nil && condaPath != "" {
        // Replace command with absolute path
        req.Command = condaPath
        logger.Debugf("Resolved conda command to: %s", condaPath)
    } else {
        // If conda not found, let it fail naturally (command not found error)
        logger.Warnf("Conda command requested but conda not found: %v", err)
    }
}
```

**Benefits**:
- Transparent to client
- No special prefix needed
- Works with existing code
- Automatically handles path resolution
- Falls back gracefully if conda not installed

## Environment Activation Strategy

**Removed: CONDA_ENV Environment Variable**

Instead of using `CONDA_ENV` environment variable, all methods explicitly take `envName` as a parameter (like `RunPython` and `RunScript` do).

**Approach**:
1. **For conda commands** (e.g., `conda env list`): Server auto-routes to absolute conda path
2. **For commands in conda environments** (e.g., Python execution): Use `conda run -n <envName> <command>` directly

**Example**:
```go
// Old approach (removed):
envVars := map[string]string{"CONDA_ENV": "myenv"}
client.Run(ctx, RunRequest{Command: "python", Args: ["-c", "print('hello')"], Env: envVars})

// New approach:
// Method signature: RunPython(ctx, envName, code, stdin)
// Internally uses: conda run -n myenv python -c "print('hello')"
```

**Implementation Changes**:
1. Remove `CONDA_ENV` from `CondaExecutor` - it will no longer check for this env var
2. Update `CondaService` methods to use `conda run -n <envName>` directly instead of setting `CONDA_ENV`
3. For direct conda commands, server auto-routes to absolute path
4. For commands needing conda env, use `conda run -n <env>` explicitly

## Testing Strategy

1. **Unit Tests**: Test each operation in isolation
2. **Integration Tests**: Test local vs remote behavior
3. **E2E Tests**: Test full workflow (local and remote)
4. **Mock Tests**: Test error handling and edge cases

## Migration Path

1. **Phase 6A**: Extend CondaService (backward compatible)
2. **Phase 6B**: Create unified interface (parallel to existing)
3. **Phase 6C**: Update CLI gradually (feature flag)
4. **Phase 6D**: Add server routing (backward compatible)

## Key Design Decisions

### 1. Command Routing: Option 1 (Automatic Detection)
- Server automatically detects `conda` commands
- Resolves to absolute path using `FindCondaPath()`
- Transparent to client, no special prefixes needed

### 2. Environment Activation: Explicit Parameters
- **Removed**: `CONDA_ENV` environment variable
- **New**: All methods take explicit `envName` parameter
- Commands in conda environments use `conda run -n <envName> <command>` directly
- Matches the pattern used by `RunPython` and `RunScript`

### 3. Implementation Approach
- **Conda commands** (e.g., `conda env list`): Server auto-routes to absolute path
- **Commands in conda envs** (e.g., Python): Use `conda run -n <envName>` explicitly
- **CondaExecutor**: Will be simplified or removed (no longer needs CONDA_ENV detection)

## Benefits

1. **Unified API**: Same interface for local and remote
2. **Clean Architecture**: Clear separation of concerns
3. **Explicit Parameters**: No hidden environment variables, clearer API
4. **Testability**: Easy to mock and test
5. **Flexibility**: Easy to add new execution strategies
6. **Maintainability**: Single source of truth for operations
7. **Consistency**: All methods follow the same pattern (explicit envName parameter)

