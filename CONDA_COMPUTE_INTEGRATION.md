# Conda/Python Integration with Compute Protocol

## Overview

This document outlines the design for integrating conda/python environment management and execution with the compute protocol, following clean architecture principles and design patterns. The plan follows **strict TDD principles with mandatory phase gates** - all tests must pass before proceeding to the next phase.

## Principles

1. **TDD First**: Write tests first, then implement minimal code to pass.
2. **Strict Phase Gates**: **ALL tests in a phase must pass before proceeding to the next phase.**
3. **Edge Case Coverage**: Comprehensive test coverage including all failure modes.
4. **Clean Architecture**: Clear separation of concerns, dependency inversion.
5. **No Backward Compatibility**: New implementation replaces legacy conda.go functions.

## Current State Analysis

### Current `core/conda/conda.go` Structure

**Responsibilities:**
1. **Conda Installation & Path Management**
   - `Install()`, `Download()`, `CommandPath()`, `UpdateCondaPath()`
   - Local file system operations

2. **Environment Management**
   - `EnvList()`, `Env()`, `CreateEnv()`, `RemoveEnv()`, `UpdateEnv()`
   - Parses conda CLI output using regex

3. **Command Execution**
   - `RunCommand()` - executes commands in conda environments
   - Uses `exec.Command` directly with `conda run --name <env> <cmd>`
   - Synchronous output capture via `runCommand()` helper

**Issues:**
- ❌ Direct dependency on `os/exec` - not testable in isolation
- ❌ Synchronous execution - blocks until command completes
- ❌ No streaming support - all output captured in memory
- ❌ Tight coupling - conda operations mixed with execution logic
- ❌ No integration with compute protocol
- ❌ Hard to test - relies on actual conda installation

## Design Principles

1. **Separation of Concerns**: Environment management vs command execution
2. **Dependency Inversion**: High-level operations depend on abstractions (Executor interface)
3. **Single Responsibility**: Each component has one clear purpose
4. **Open/Closed**: Extensible without modifying existing code
5. **Interface Segregation**: Small, focused interfaces
6. **Testability**: All components testable in isolation

## Architecture Design

### Layer Structure

```
┌─────────────────────────────────────────────────────────────┐
│  Client Layer (GUI/API)                                     │
│  - CondaService (high-level remote operations)              │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│  Compute Protocol Layer                                      │
│  - compute.Client (raw command execution)                    │
│  - compute.Server (with CondaExecutor)                       │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│  Executor Layer (Abstraction)                                │
│  - Executor interface                                        │
│  - RawExecutor (basic os/exec)                              │
│  - CondaExecutor (conda environment activation)             │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│  Conda Management Layer                                      │
│  - CondaManager (environment CRUD operations)               │
│  - Uses Executor for conda CLI commands                      │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│  Infrastructure Layer                                        │
│  - Path detection, installation, file operations            │
│  - Refactored from current conda.go                          │
└─────────────────────────────────────────────────────────────┘
```

### Component Design

#### 1. CondaExecutor (Server-Side)

**Purpose**: Execute commands within conda environments using the Executor interface.

**Location**: `p2p/protocols/compute/service/conda_executor.go`

```go
// CondaExecutor wraps an Executor and activates conda environment before execution
type CondaExecutor struct {
    baseExecutor Executor
    condaPath    string
}

// NewCondaExecutor creates a CondaExecutor that wraps another executor
func NewCondaExecutor(base Executor, condaPath string) *CondaExecutor

// ExecuteRaw executes a command within a conda environment
// If req.Env contains "CONDA_ENV", activates that environment first
func (e *CondaExecutor) ExecuteRaw(ctx context.Context, req RawExecutionRequest) (*RawExecution, error)
```

**Behavior:**
- Checks `req.Env["CONDA_ENV"]` for environment name
- If present, wraps command with `conda run --name <env> <command>`
- Otherwise, delegates to base executor
- Maintains streaming capabilities (stdin/stdout/stderr)

**Benefits:**
- ✅ Composable - wraps any Executor
- ✅ Transparent - compute protocol unchanged
- ✅ Testable - can mock base executor
- ✅ Streaming - preserves real-time I/O

#### 2. CondaManager (Server-Side)

**Purpose**: Manage conda environments (create, list, remove, update) using Executor.

**Location**: `core/conda/manager.go`

```go
// CondaManager manages conda environments using an Executor
type CondaManager struct {
    executor Executor
    condaPath string
}

// NewCondaManager creates a new CondaManager
func NewCondaManager(executor Executor, condaPath string) *CondaManager

// ListEnvironments returns all conda environments
func (m *CondaManager) ListEnvironments(ctx context.Context) (map[string]string, error)

// GetEnvironment returns the path of a specific environment
func (m *CondaManager) GetEnvironment(ctx context.Context, name string) (string, error)

// CreateEnvironment creates a new conda environment
func (m *CondaManager) CreateEnvironment(ctx context.Context, name, pythonVersion string) (string, error)

// RemoveEnvironment removes a conda environment
func (m *CondaManager) RemoveEnvironment(ctx context.Context, name string) error

// UpdateEnvironment updates a conda environment from a YAML file
func (m *CondaManager) UpdateEnvironment(ctx context.Context, name, yamlPath string) error

// InstallPackage installs a package in an environment
func (m *CondaManager) InstallPackage(ctx context.Context, env, packageName string) error
```

**Benefits:**
- ✅ Uses Executor interface - testable with mocks
- ✅ Streaming support - can show progress in real-time
- ✅ Separated from path/installation logic

#### 3. CondaService (Client-Side)

**Purpose**: High-level API for remote conda operations over compute protocol.

**Location**: `p2p/protocols/compute/service/conda_service.go`

```go
// CondaService provides high-level conda operations over compute protocol
type CondaService struct {
    client *Client
}

// NewCondaService creates a new CondaService
func NewCondaService(client *Client) *CondaService

// ListEnvironments lists all conda environments on remote device
func (s *CondaService) ListEnvironments(ctx context.Context) (map[string]string, error)

// CreateEnvironment creates a conda environment on remote device
func (s *CondaService) CreateEnvironment(ctx context.Context, name, pythonVersion string) (string, error)

// RemoveEnvironment removes a conda environment on remote device
func (s *CondaService) RemoveEnvironment(ctx context.Context, name string) error

// RunPython executes Python code in a conda environment
func (s *CondaService) RunPython(ctx context.Context, env string, code string, stdin io.Reader) (*RawExecutionHandle, error)

// RunScript executes a Python script in a conda environment
func (s *CondaService) RunScript(ctx context.Context, env, scriptPath string, args []string, stdin io.Reader) (*RawExecutionHandle, error)

// InstallPackage installs a package in a conda environment
func (s *CondaService) InstallPackage(ctx context.Context, env, packageName string) error
```

**Benefits:**
- ✅ High-level API - easy to use from GUI/API
- ✅ Remote operations - works over network
- ✅ Streaming - real-time output for long operations
- ✅ Consistent with compute protocol patterns

#### 4. Refactored Infrastructure Layer

**Purpose**: Path detection, installation, file operations (refactored from current `conda.go`).

**Location**: `core/conda/infrastructure.go`

```go
// Infrastructure functions (no Executor dependency)
func FindCondaPath() (string, error)
func GetCondaVersion(condaPath string) (string, error)
func DownloadCondaInstaller(version string) (string, error)
func InstallConda(installerPath string) error
func GetCondaDownloadPath() (string, error)
```

**Benefits:**
- ✅ Pure functions - easy to test
- ✅ No execution logic - just file/path operations
- ✅ Reusable - can be used by both local and remote operations

## Implementation Plan

### Phase 1: Refactor Infrastructure Layer
**Goal**: Extract path/installation logic from `conda.go`

**Approach**: TDD - Write tests first, then refactor

**Tasks:**
1. Create `core/conda/infrastructure_test.go` with all test cases
2. Create `core/conda/infrastructure.go` with minimal implementation
3. Move path detection, installation, download functions
4. Remove `runCommand` helper (replaced by Executor)
5. Update existing `conda_test.go` to use new infrastructure

**Test Cases (Must All Pass):**

**Unit Tests - Path Detection:**
- `TestFindCondaPath_Windows_SystemPath` - Finds conda in ProgramData
- `TestFindCondaPath_Windows_UserPath` - Finds conda in user directory
- `TestFindCondaPath_Linux_SystemPath` - Finds conda in system PATH
- `TestFindCondaPath_Linux_LocalInstall` - Finds conda in local install
- `TestFindCondaPath_Darwin_SystemPath` - Finds conda in system PATH
- `TestFindCondaPath_Darwin_LocalInstall` - Finds conda in local install
- `TestFindCondaPath_NotFound` - Returns error when conda not found
- `TestFindCondaPath_MultiplePaths` - Prefers system path over local

**Unit Tests - Version Detection:**
- `TestGetCondaVersion_Success` - Parses version from conda --version
- `TestGetCondaVersion_InvalidOutput` - Handles malformed version output
- `TestGetCondaVersion_CommandFails` - Handles conda command failure
- `TestGetCondaVersion_EmptyOutput` - Handles empty output

**Unit Tests - Download Path:**
- `TestGetCondaDownloadPath_Success` - Creates download directory
- `TestGetCondaDownloadPath_PermissionDenied` - Handles permission errors
- `TestGetCondaDownloadPath_ExistingDirectory` - Uses existing directory

**Unit Tests - URL Generation:**
- `TestDownloadUrl_Linux_AMD64` - Generates correct Linux x86_64 URL
- `TestDownloadUrl_Linux_ARM64` - Generates correct Linux ARM64 URL
- `TestDownloadUrl_Darwin_AMD64` - Generates correct macOS x86_64 URL
- `TestDownloadUrl_Darwin_ARM64` - Generates correct macOS ARM64 URL
- `TestDownloadUrl_Windows_AMD64` - Generates correct Windows x86_64 URL
- `TestDownloadUrl_UnsupportedArch` - Returns error for unsupported arch
- `TestDownloadUrl_UnsupportedOS` - Returns error for unsupported OS

**Edge Cases:**
- `TestFindCondaPath_EmptyPATH` - Handles empty PATH environment variable
- `TestFindCondaPath_Symlink` - Handles symlinked conda executables
- `TestGetCondaVersion_NonNumericVersion` - Handles non-standard version formats
- `TestGetCondaDownloadPath_DiskFull` - Handles disk full scenario
- `TestDownloadUrl_InvalidVersion` - Handles invalid version strings

**Phase Gate:**
- ✅ All unit tests pass (`go test ./core/conda -timeout 30s -count=1`)
- ✅ No linting errors
- ⚠️ Code coverage: 61.6% for infrastructure.go (below 80% target due to platform-specific branches in `DownloadCondaInstaller` that cannot all be tested on a single platform; all testable code on current platform is covered)
- ✅ All edge case tests pass

**Status:** ✅ **COMPLETE**

**Files Created/Modified:**
- `core/conda/infrastructure.go` (new) - Extracted path detection, version detection, download path, and URL generation
- `core/conda/infrastructure_test.go` (new) - Comprehensive test suite with 30+ test cases
- `core/conda/conda.go` (refactored) - Updated to use new infrastructure functions, kept `runCommand` for backward compatibility (will be removed in Phase 3)

**Notes:**
- `runCommand` and `saveOutput` are kept in `conda.go` for now as they're still used by functions that will be refactored in Phase 3 (EnvList, CreateEnv, etc.)
- `GetCondaVersion` uses `exec.Command` directly; will be refactored in Phase 3 to use Executor interface
- Coverage is below 80% due to platform-specific code branches, but all testable paths on the current platform are covered

### Phase 2: Implement CondaExecutor
**Goal**: Create executor that activates conda environments

**Approach**: TDD - Write tests first, then implement

**Tasks:**
1. Create `p2p/protocols/compute/service/conda_executor_test.go` with all test cases
2. Create `p2p/protocols/compute/service/conda_executor.go` with minimal implementation
3. Implement `CondaExecutor` wrapping `RawExecutor`
4. Handle `CONDA_ENV` environment variable (use `CONDA_ENV` as decided)
5. Ensure streaming works correctly

**Test Cases (Must All Pass):**

**Unit Tests - Basic Functionality:**
- `TestCondaExecutor_ExecuteWithoutEnv` - Delegates to base executor when no CONDA_ENV
- `TestCondaExecutor_ExecuteWithEnv` - Wraps command with conda run when CONDA_ENV set
- `TestCondaExecutor_CommandArgs` - Preserves command arguments correctly
- `TestCondaExecutor_EnvironmentVariables` - Merges CONDA_ENV with other env vars
- `TestCondaExecutor_WorkingDir` - Preserves working directory

**Unit Tests - Streaming:**
- `TestCondaExecutor_StdoutStreaming` - Streams stdout in real-time
- `TestCondaExecutor_StderrStreaming` - Streams stderr in real-time
- `TestCondaExecutor_StdinStreaming` - Forwards stdin correctly
- `TestCondaExecutor_ConcurrentStreams` - Handles concurrent stdout/stderr
- `TestCondaExecutor_LargeOutput` - Handles large output streams (>1MB)

**Unit Tests - Error Handling:**
- `TestCondaExecutor_InvalidEnv` - Handles non-existent conda environment
- `TestCondaExecutor_CondaNotFound` - Handles conda executable not found
- `TestCondaExecutor_CommandFailure` - Propagates command execution errors
- `TestCondaExecutor_ContextCancellation` - Handles context cancellation
- `TestCondaExecutor_BaseExecutorError` - Propagates base executor errors

**Unit Tests - Edge Cases:**
- `TestCondaExecutor_EmptyEnvName` - Handles empty CONDA_ENV value
- `TestCondaExecutor_EnvNameWithSpaces` - Handles environment names with spaces
- `TestCondaExecutor_SpecialCharactersInEnv` - Handles special chars in env name
- `TestCondaExecutor_MultipleEnvVars` - Handles multiple environment variables
- `TestCondaExecutor_EnvVarOverride` - CONDA_ENV doesn't override other vars
- `TestCondaExecutor_NilBaseExecutor` - Handles nil base executor gracefully
- `TestCondaExecutor_EmptyCondaPath` - Handles empty conda path

**Integration Tests:**
- `TestCondaExecutor_RealCondaEnv` - Executes command in real conda environment (if available)
- `TestCondaExecutor_ExitCode` - Correctly propagates exit codes
- `TestCondaExecutor_CancelDuringExecution` - Cancels long-running commands

**Phase Gate:**
- ✅ All unit tests pass (`go test ./p2p/protocols/compute/service -timeout 30s -count=1 -run TestCondaExecutor`)
- ✅ All integration tests pass (if conda available)
- ✅ No linting errors in conda_executor.go
- ✅ Code coverage: 100% for conda_executor.go
- ✅ All edge case tests pass
- ✅ Streaming tests verify real-time output

**Status:** ✅ **COMPLETE**

**Files Created:**
- `p2p/protocols/compute/service/conda_executor.go` (new) - Implements CondaExecutor wrapping RawExecutor
- `p2p/protocols/compute/service/conda_executor_test.go` (new) - Comprehensive test suite with 30+ test cases

**Implementation Details:**
- `CondaExecutor` wraps a base `Executor` and activates conda environments when `CONDA_ENV` is set
- When `CONDA_ENV` is set, command is wrapped with `conda run -n <env> <command> <args>`
- All other environment variables are preserved (except `CONDA_ENV` which is handled by conda)
- Streaming (stdin/stdout/stderr) works correctly through the wrapper
- Empty `CONDA_ENV` values are treated as no conda environment (delegates to base executor)

### Phase 3: Implement CondaManager
**Goal**: Environment management using Executor interface

**Approach**: TDD - Write tests first, then implement

**Tasks:**
1. Create `core/conda/manager_test.go` with all test cases
2. Create `core/conda/manager.go` with minimal implementation
3. Implement environment CRUD operations
4. Use Executor for all conda CLI commands
5. Parse output using existing logic (refactored)
6. Implement streaming progress reporting via stdout/stderr

**Test Cases (Must All Pass):**

**Unit Tests - ListEnvironments:**
- `TestCondaManager_ListEnvironments_Success` - Lists all environments correctly
- `TestCondaManager_ListEnvironments_Empty` - Handles no environments
- `TestCondaManager_ListEnvironments_ExcludesBase` - Excludes base environment
- `TestCondaManager_ListEnvironments_ParsesOutput` - Correctly parses conda env list output
- `TestCondaManager_ListEnvironments_CommandFails` - Handles conda command failure
- `TestCondaManager_ListEnvironments_InvalidOutput` - Handles malformed output
- `TestCondaManager_ListEnvironments_Streaming` - Streams output in real-time

**Unit Tests - GetEnvironment:**
- `TestCondaManager_GetEnvironment_Success` - Returns environment path
- `TestCondaManager_GetEnvironment_NotFound` - Returns error for non-existent env
- `TestCondaManager_GetEnvironment_EmptyName` - Handles empty environment name

**Unit Tests - CreateEnvironment:**
- `TestCondaManager_CreateEnvironment_Success` - Creates environment successfully
- `TestCondaManager_CreateEnvironment_AlreadyExists` - Returns existing env path if exists
- `TestCondaManager_CreateEnvironment_InvalidVersion` - Handles invalid Python version
- `TestCondaManager_CreateEnvironment_CommandFails` - Handles creation failure
- `TestCondaManager_CreateEnvironment_Streaming` - Streams creation progress
- `TestCondaManager_CreateEnvironment_Timeout` - Handles long-running creation
- `TestCondaManager_CreateEnvironment_Concurrent` - Handles concurrent creation requests

**Unit Tests - RemoveEnvironment:**
- `TestCondaManager_RemoveEnvironment_Success` - Removes environment successfully
- `TestCondaManager_RemoveEnvironment_NotFound` - Returns error for non-existent env
- `TestCondaManager_RemoveEnvironment_CommandFails` - Handles removal failure
- `TestCondaManager_RemoveEnvironment_Streaming` - Streams removal progress

**Unit Tests - UpdateEnvironment:**
- `TestCondaManager_UpdateEnvironment_Success` - Updates environment from YAML
- `TestCondaManager_UpdateEnvironment_NotFound` - Returns error for non-existent env
- `TestCondaManager_UpdateEnvironment_InvalidYAML` - Handles invalid YAML file
- `TestCondaManager_UpdateEnvironment_FileNotFound` - Handles missing YAML file
- `TestCondaManager_UpdateEnvironment_Streaming` - Streams update progress

**Unit Tests - InstallPackage:**
- `TestCondaManager_InstallPackage_Success` - Installs package successfully
- `TestCondaManager_InstallPackage_AlreadyInstalled` - Handles already installed package
- `TestCondaManager_InstallPackage_NotFound` - Handles package not found
- `TestCondaManager_InstallPackage_EnvNotFound` - Returns error for non-existent env
- `TestCondaManager_InstallPackage_Streaming` - Streams installation progress
- `TestCondaManager_InstallPackage_NetworkError` - Handles network failures

**Unit Tests - Edge Cases:**
- `TestCondaManager_NilExecutor` - Handles nil executor gracefully
- `TestCondaManager_EmptyCondaPath` - Handles empty conda path
- `TestCondaManager_ContextCancellation` - Handles context cancellation during operations
- `TestCondaManager_ConcurrentOperations` - Handles concurrent operations on same env
- `TestCondaManager_SpecialCharsInEnvName` - Handles special characters in env names
- `TestCondaManager_EnvNameCollision` - Handles environment name conflicts
- `TestCondaManager_LargeOutput` - Handles large conda command output
- `TestCondaManager_PartialOutput` - Handles partial/truncated output

**Integration Tests:**
- `TestCondaManager_EndToEnd` - Create → List → Install → Remove workflow
- `TestCondaManager_RealCondaOperations` - Tests with real conda (if available)

**Phase Gate:**
- ✅ All unit tests pass (30+ tests passing)
- ✅ All integration tests pass (with Docker environment)
- ✅ No linting errors in manager.go
- ✅ Code coverage: High coverage for manager.go (all functions tested)
- ✅ All edge case tests pass
- ✅ Streaming tests verify real-time progress reporting (in Docker environment)

**Status:** ✅ **COMPLETE**

**Files Created:**
- `core/conda/manager.go` (new) - Implements CondaManager using Executor interface
- `core/conda/manager_test.go` (new) - Comprehensive test suite with 30+ test cases
- `core/conda/manager_edge_cases_test.go` (new) - Additional edge case tests
- `core/conda/test_helper.go` (new) - Test helper functions for conditional test execution

**Implementation Details:**
- `CondaManager` uses `Executor` interface for all conda CLI commands
- Parsing logic refactored from `conda.go` to `manager.go`
- All CRUD operations implemented: ListEnvironments, GetEnvironment, CreateEnvironment, RemoveEnvironment, UpdateEnvironment, InstallPackage
- Streaming works via Executor's stdout/stderr streams
- **Error handling improvements:**
  - All methods now read stdout and stderr concurrently to prevent blocking
  - Error messages include both stdout and stderr for better debugging
  - Logging added via `managerLogger` for diagnostic information
  - `CreateEnvironment` detects "already exists" errors in concurrent scenarios
  - `RemoveEnvironment` uses multiple success patterns and verifies removal by checking if environment still exists
- **Context cancellation:** Proper handling of context cancellation at start of operations
- Tests use mock executor for fast unit tests, real executor for integration tests

**Docker Testing Setup:**
- `Dockerfile.conda-test` - Docker image with Go and Miniconda pre-installed
- `docker-compose.conda-test.yml` - Docker Compose service for running conda tests
- Conda Terms of Service (ToS) automatically accepted during image build
- Tests can be run with: `docker compose -f docker-compose.conda-test.yml run --build --rm conda-test`
- `ShouldRunCondaTests()` helper function conditionally enables tests based on `CONDATEST_ENABLED` environment variable
- All tests pass in Docker environment (~99 seconds total test time)

### Phase 4: Implement CondaService (Client-Side)
**Goal**: High-level remote conda operations

**Approach**: TDD - Write tests first, then implement

**Tasks:**
1. Create `p2p/protocols/compute/service/conda_service_test.go` with all test cases
2. Create `p2p/protocols/compute/service/conda_service.go` with minimal implementation
3. Implement remote environment management
4. Implement Python execution helpers
5. Ensure error propagation via compute protocol

**Test Cases (Must All Pass):**

**Unit Tests - ListEnvironments:**
- `TestCondaService_ListEnvironments_Success` - Lists remote environments
- `TestCondaService_ListEnvironments_ConnectionError` - Handles connection failures
- `TestCondaService_ListEnvironments_CommandError` - Handles remote command errors
- `TestCondaService_ListEnvironments_Timeout` - Handles timeout
- `TestCondaService_ListEnvironments_Streaming` - Streams output in real-time

**Unit Tests - CreateEnvironment:**
- `TestCondaService_CreateEnvironment_Success` - Creates remote environment
- `TestCondaService_CreateEnvironment_AlreadyExists` - Handles existing environment
- `TestCondaService_CreateEnvironment_InvalidVersion` - Handles invalid version
- `TestCondaService_CreateEnvironment_Streaming` - Streams creation progress
- `TestCondaService_CreateEnvironment_Concurrent` - Handles concurrent requests

**Unit Tests - RemoveEnvironment:**
- `TestCondaService_RemoveEnvironment_Success` - Removes remote environment
- `TestCondaService_RemoveEnvironment_NotFound` - Handles non-existent env
- `TestCondaService_RemoveEnvironment_Streaming` - Streams removal progress

**Unit Tests - RunPython:**
- `TestCondaService_RunPython_Success` - Executes Python code successfully
- `TestCondaService_RunPython_WithStdin` - Handles stdin input
- `TestCondaService_RunPython_Streaming` - Streams stdout/stderr in real-time
- `TestCondaService_RunPython_Error` - Handles Python execution errors
- `TestCondaService_RunPython_EnvNotFound` - Handles non-existent environment
- `TestCondaService_RunPython_ExitCode` - Correctly propagates exit codes
- `TestCondaService_RunPython_Cancel` - Handles cancellation

**Unit Tests - RunScript:**
- `TestCondaService_RunScript_Success` - Executes Python script successfully
- `TestCondaService_RunScript_WithArgs` - Handles script arguments
- `TestCondaService_RunScript_WithStdin` - Handles stdin input
- `TestCondaService_RunScript_FileNotFound` - Handles missing script file
- `TestCondaService_RunScript_Streaming` - Streams script output
- `TestCondaService_RunScript_Error` - Handles script execution errors

**Unit Tests - InstallPackage:**
- `TestCondaService_InstallPackage_Success` - Installs package remotely
- `TestCondaService_InstallPackage_AlreadyInstalled` - Handles already installed
- `TestCondaService_InstallPackage_NotFound` - Handles package not found
- `TestCondaService_InstallPackage_Streaming` - Streams installation progress

**Unit Tests - Error Propagation:**
- `TestCondaService_ErrorPropagation_CommandError` - Propagates command errors correctly
- `TestCondaService_ErrorPropagation_NetworkError` - Propagates network errors
- `TestCondaService_ErrorPropagation_Timeout` - Propagates timeout errors
- `TestCondaService_ErrorPropagation_ServerError` - Propagates server-side errors

**Unit Tests - Edge Cases:**
- `TestCondaService_NilClient` - Handles nil compute client
- `TestCondaService_DisconnectedClient` - Handles disconnected client
- `TestCondaService_EmptyEnvName` - Handles empty environment names
- `TestCondaService_ConcurrentOperations` - Handles concurrent service calls
- `TestCondaService_LargeCode` - Handles large Python code blocks
- `TestCondaService_SpecialChars` - Handles special characters in env/package names

**Integration Tests:**
- `TestCondaService_EndToEnd_Remote` - Full workflow over compute protocol
- `TestCondaService_RealRemoteExecution` - Tests with real remote device (if available)

**Phase Gate:**
- ✅ All unit tests pass (`go test ./p2p/protocols/compute/service -timeout 30s -count=1 -run TestCondaService`)
- ✅ All integration tests pass
- ✅ No linting errors
- ✅ Code coverage ≥ 85% for conda_service.go
- ✅ All edge case tests pass

**Status:** ✅ **COMPLETE**

**Implementation Summary:**
- Created `conda_service.go` with `CondaService` providing high-level remote conda operations
- Implemented all remote environment management operations (ListEnvironments, GetEnvironment, CreateEnvironment, RemoveEnvironment)
- Implemented Python execution helpers (RunPython, RunScript)
- Implemented InstallPackage for remote package installation
- All operations use `CONDA_ENV` environment variable convention
- Error propagation via compute protocol
- Comprehensive test suite with 30+ test cases covering all operations and edge cases
- Helper functions for creating mock handles in tests
- Proper handling of stdin/stdout/stderr streams with goroutines
- Concurrent stdout/stderr reading to prevent blocking
- ✅ Error propagation tests verify compute protocol error reporting

**Files:**
- `p2p/protocols/compute/service/conda_service.go` (new)
- `p2p/protocols/compute/service/conda_service_test.go` (new)

### Phase 5: Server Integration
**Goal**: Wire CondaExecutor into compute server

**Approach**: TDD - Write tests first, then integrate

**Tasks:**
1. Create/update `p2p/protocols/compute/compute_test.go` with integration tests
2. Update `p2p/protocols/compute/compute.go` to use `CondaExecutor`
3. Create server with `CondaExecutor` wrapping `RawExecutor`
4. Ensure backward compatibility with raw commands

**Test Cases (Must All Pass):**

**Integration Tests - Server Setup:**
- `TestComputeServer_WithCondaExecutor` - Server uses CondaExecutor correctly
- `TestComputeServer_RawCommandStillWorks` - Raw commands work without CONDA_ENV
- `TestComputeServer_CondaCommandWorks` - Commands with CONDA_ENV use conda
- `TestComputeServer_MixedCommands` - Handles mix of raw and conda commands

**Integration Tests - Environment Activation:**
- `TestComputeServer_ExecuteInCondaEnv` - Executes commands in conda environment
- `TestComputeServer_InvalidCondaEnv` - Handles invalid environment gracefully
- `TestComputeServer_StreamingInCondaEnv` - Streaming works in conda environment
- `TestComputeServer_CancelInCondaEnv` - Cancellation works in conda environment

**Integration Tests - Error Handling:**
- `TestComputeServer_CondaErrorPropagation` - Conda errors propagate correctly
- `TestComputeServer_CommandErrorInCondaEnv` - Command errors in conda env propagate
- `TestComputeServer_NetworkError` - Network errors handled correctly

**Edge Cases:**
- `TestComputeServer_ConcurrentCondaCommands` - Handles concurrent conda commands
- `TestComputeServer_CondaPathNotFound` - Handles conda not found on server
- `TestComputeServer_EmptyCondaPath` - Handles empty conda path

**Phase Gate:**
- ✅ All integration tests pass (`go test ./p2p/protocols/compute/service -timeout 30s -run TestComputeServer_`)
- ✅ Existing compute protocol tests still pass
- ✅ No linting errors
- ✅ Code coverage maintained for compute.go
- ✅ All edge case tests pass

**Status:** ✅ **COMPLETE**

**Implementation Summary:**
- Updated `computeStreamHandler` in `compute.go` to use `CondaExecutor` wrapping `RawExecutor`
- Server automatically detects conda path using `core/conda.FindCondaPath()`
- Falls back to `RawExecutor` only if conda is not found (backward compatible)
- Created comprehensive integration tests covering all scenarios:
  - Server setup with CondaExecutor
  - Raw commands still work without CONDA_ENV
  - Conda commands work with CONDA_ENV
  - Mixed raw and conda commands
  - Environment activation and execution
  - Invalid environment handling
  - Streaming in conda environments
  - Cancellation in conda environments
  - Error propagation
  - Concurrent commands
  - Edge cases (conda path not found, empty conda path)
- All tests passing with proper timeout handling

**Files:**
- `p2p/protocols/compute/compute.go` (updated)
- `p2p/protocols/compute/service/conda_integration_test.go` (new)

### Phase 6: Unified Conda Operations & Remote Support
**Goal**: Extend CondaService with all pre-conda features, add server-side conda command routing, and create unified interface for local/remote operations

**Approach**: TDD - Write tests first, then implement

**Sub-Phases:**
- **Phase 6A**: Extend CondaService with pre-conda features
- **Phase 6B**: Add server-side conda command routing
- **Phase 6C**: Create unified CondaOperations interface
- **Phase 6D**: Update CLI to use unified interface

**Tasks (Phase 6A):**
1. Add pre-conda methods to CondaService:
   - `InstallConda()` - Remote conda installation
   - `GetCondaPath()` - Get remote conda path  
   - `GetCondaVersion()` - Get remote conda version
   
   **Note**: `DownloadCondaInstaller()` is excluded as it's an internal implementation detail. `InstallConda()` will handle the full installation process internally, including URL generation and download.
   
2. Add missing environment management methods if needed
3. Write comprehensive tests for all new methods

**Tasks (Phase 6B):**
1. **Server Command Routing (Option 1 - Automatic Detection)**:
   - Add automatic conda command detection in server
   - Resolve conda path using `FindCondaPath()` on server
   - Route conda commands to absolute path automatically
   - Handle errors gracefully (conda not found)

2. **Remove CONDA_ENV Environment Variable**:
   - Update CondaExecutor to remove CONDA_ENV dependency (or simplify/remove CondaExecutor)
   - Update CondaService methods to use `conda run -n <envName>` directly instead of setting CONDA_ENV
   - All methods already take explicit `envName` parameter (like RunPython/RunScript)
   - Update all tests to remove CONDA_ENV usage

3. **Testing**:
   - Write integration tests for command routing
   - Update existing tests to use explicit envName parameters
   - Verify backward compatibility where applicable

**Tasks (Phase 6C):**
1. Define `CondaOperations` interface
2. Implement `LocalCondaOps` using `CondaManager`
3. Implement `RemoteCondaOps` using `CondaService`
4. Create factory function to choose strategy
5. Write tests for unified interface

**Tasks (Phase 6D):**
1. Update CLI to use `CondaOperations` interface
2. Add remote device selection
3. Route commands to local or remote based on context
4. Maintain backward compatibility

**Test Cases (Must All Pass):**

**GUI Tests - Environment Management:**
- `TestGUI_ListEnvironments` - GUI lists environments correctly
- `TestGUI_CreateEnvironment` - GUI creates environment with progress
- `TestGUI_RemoveEnvironment` - GUI removes environment
- `TestGUI_EnvironmentError` - GUI displays errors correctly
- `TestGUI_EnvironmentStreaming` - GUI shows real-time progress

**GUI Tests - Python Execution:**
- `TestGUI_RunPythonCode` - GUI executes Python code
- `TestGUI_RunPythonScript` - GUI executes Python script
- `TestGUI_PythonOutput` - GUI displays Python output
- `TestGUI_PythonError` - GUI displays Python errors
- `TestGUI_PythonStreaming` - GUI streams Python output in real-time
- `TestGUI_PythonCancel` - GUI cancels Python execution

**API Tests - Endpoints:**
- `TestAPI_ListEnvironments` - API endpoint lists environments
- `TestAPI_CreateEnvironment` - API endpoint creates environment
- `TestAPI_RemoveEnvironment` - API endpoint removes environment
- `TestAPI_RunPython` - API endpoint executes Python
- `TestAPI_ErrorHandling` - API handles errors correctly

**Edge Cases:**
- `TestGUI_DisconnectedDevice` - GUI handles disconnected device
- `TestGUI_ConcurrentOperations` - GUI handles concurrent operations
- `TestGUI_LargeOutput` - GUI handles large Python output
- `TestAPI_InvalidRequests` - API handles invalid requests

**Phase Gate:**
- ✅ All GUI tests pass (`go test ./core/gui -timeout 30s -count=1`)
- ✅ All API tests pass (`go test ./core/socket -timeout 30s -count=1`)
- ✅ No linting errors
- ✅ Manual UI testing confirms functionality
- ✅ All edge case tests pass

**Files:**
- `core/gui/gui.go` (updated)
- `core/gui/gui_test.go` (updated)
- `core/socket/socket.go` (updated if needed)
- `core/socket/socket_test.go` (updated if needed)

## Design Patterns Used

### 1. Decorator Pattern
- `CondaExecutor` decorates `RawExecutor` with conda environment activation
- Allows composition: `CondaExecutor(RawExecutor)`

### 2. Strategy Pattern
- `Executor` interface allows different execution strategies
- `RawExecutor`, `CondaExecutor` are different strategies

### 3. Adapter Pattern
- `CondaManager` adapts conda CLI to `Executor` interface
- Bridges legacy conda operations to new architecture

### 4. Facade Pattern
- `CondaService` provides simplified interface to complex conda operations
- Hides complexity of compute protocol from clients

### 5. Dependency Injection
- All components accept dependencies via constructors
- Enables testing with mocks

## Testing Strategy

### Unit Tests
- **CondaExecutor**: Mock base executor, verify conda activation
- **CondaManager**: Mock executor, verify environment operations
- **CondaService**: Mock compute client, verify remote operations
- **Infrastructure**: Test path detection, parsing logic

### Integration Tests
- **End-to-end**: Create env → Install package → Run script
- **Remote execution**: Test conda operations over compute protocol
- **Streaming**: Verify real-time output for long operations

### Test Fixtures
- Mock conda CLI output for parsing tests
- Mock executor for manager tests
- Mock compute client for service tests

### Docker Testing Environment
- **Purpose**: Provide consistent environment with conda pre-installed for running tests that require actual conda operations
- **Setup**: 
  - `Dockerfile.conda-test` - Based on `golang:1.24`, installs Miniconda, accepts ToS
  - `docker-compose.conda-test.yml` - Service definition for conda tests
  - `core/conda/test_helper.go` - Helper functions for conditional test execution
- **Usage**: 
  - Run all conda tests: `docker compose -f docker-compose.conda-test.yml run --build --rm conda-test`
  - Tests automatically detect Docker environment via `ShouldRunCondaTests()`
  - Conda ToS automatically accepted during image build
- **Benefits**:
  - Consistent test environment across different machines
  - All tests pass reliably (no network/conda availability issues)
  - Better error reporting with stdout/stderr logging enabled

## Migration Path

### Backward Compatibility
- Keep existing `conda.go` functions during transition
- Mark as deprecated
- Provide migration guide

### Gradual Migration
1. Implement new components alongside old code
2. Update tests to use new components
3. Update GUI/API to use new components
4. Remove deprecated code

## Benefits

### Clean Architecture
- ✅ Clear separation of concerns
- ✅ Dependency inversion (high-level depends on abstractions)
- ✅ Testable components
- ✅ Extensible design

### Functionality
- ✅ Streaming support for long operations
- ✅ Real-time output for environment creation
- ✅ Remote conda operations
- ✅ Consistent with compute protocol

### Maintainability
- ✅ Single responsibility per component
- ✅ Easy to test in isolation
- ✅ Easy to extend (new executor types)
- ✅ Clear dependencies

## Open Questions - Answered

1. **Environment Variable Convention**: ✅ **Use `CONDA_ENV`**
   - Simple, concise, follows common environment variable naming
   - Example: `req.Env["CONDA_ENV"] = "myenv"`

2. **Error Handling**: ✅ **Propagate via compute protocol error reporting**
   - Errors from conda operations are returned through the compute protocol's standard error mechanism
   - `ComputeRunResponse.Error` field for run-time errors
   - `ComputeStatusResponse` for execution status
   - Client receives errors via `RawExecutionHandle.Done` channel
   - No special error types needed - use standard error propagation

3. **Progress Reporting**: ✅ **Yes, streaming using stdout/stderr**
   - Environment creation streams progress via stdout/stderr
   - Long operations (install, create, update) stream output in real-time
   - Client can display progress in GUI/terminal
   - No separate progress API needed - use existing streaming

4. **Caching**: ✅ **No caching**
   - Always fetch fresh environment list
   - Environments can change externally (user creates/deletes via CLI)
   - Simplicity over optimization
   - If needed later, can be added as optimization

5. **Concurrency**: ✅ **Yes, support concurrent operations**
   - Multiple environments can be created simultaneously
   - Each operation uses its own execution context
   - Server handles concurrent requests via existing compute protocol
   - Client can make concurrent requests safely

## TDD Workflow & Phase Gates

### TDD Process for Each Phase

1. **Write Tests First**: Create comprehensive test file with all test cases (including edge cases)
2. **Run Tests**: Verify tests fail (red phase)
3. **Implement Minimal Code**: Write just enough code to make tests pass
4. **Refactor**: Improve code while keeping tests green
5. **Verify Phase Gate**: Ensure ALL tests pass before proceeding

### Phase Gate Requirements

**Every phase must satisfy:**
- ✅ All unit tests pass with `go test -timeout 30s -count=1`
- ✅ All integration tests pass (if applicable)
- ✅ No linting errors
- ✅ Code coverage ≥ 80-85% (varies by phase)
- ✅ All edge case tests pass
- ✅ No skipped tests (unless explicitly marked as optional)

### Test Coverage Requirements

- **Unit Tests**: Test individual components in isolation with mocks
- **Integration Tests**: Test component interactions
- **Edge Cases**: Test error conditions, boundary cases, concurrent operations
- **Streaming Tests**: Verify real-time output for long operations
- **Error Propagation Tests**: Verify errors flow correctly through layers

### Test Timeout

- All test commands must use `-timeout 30s` flag (or `-timeout 10m` for Docker tests with real conda operations)
- Tests should be deterministic (no `time.Sleep` in test logic)
- Use channel synchronization and `go test -timeout` for overall safety

### Error Reporting and Debugging

- **Logging**: All conda operations log stdout/stderr at WARN/INFO level when failures occur
- **Error Messages**: Error messages include both stdout and stderr for comprehensive debugging
- **Test Logging**: `InitTestLogging()` helper enables debug logging in tests for better visibility
- **Concurrent Operations**: Proper handling of race conditions (e.g., concurrent environment creation)

## Implementation Status

### Completed Phases

- ✅ **Phase 1**: Infrastructure Layer Refactoring - COMPLETE
- ✅ **Phase 2**: CondaExecutor Implementation - COMPLETE
- ✅ **Phase 3**: CondaManager Implementation - COMPLETE
  - All CRUD operations implemented and tested
  - Comprehensive error handling with stdout/stderr logging
  - Docker testing environment set up
  - All tests passing in Docker environment (~99 seconds total test time)

### Remaining Phases

- ⏳ **Phase 4**: CondaService (Client-Side) - PENDING
- ⏳ **Phase 5**: Server Integration - PENDING
- ⏳ **Phase 6**: Client Integration & GUI - PENDING

## Next Steps

1. Proceed with Phase 4: Implement CondaService (Client-Side)
2. Follow strict phase gates - do not proceed until ALL tests pass
3. Continue updating documentation as we go

