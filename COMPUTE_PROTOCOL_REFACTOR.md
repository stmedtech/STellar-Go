# Compute Protocol Refactoring Plan

## Overview

This document outlines a **ground-up implementation** of the compute protocol for raw command execution with byte-level streaming of stdin/stdout/stderr, using multiplexed streams for control and logging. The plan follows strict TDD principles with **mandatory phase gates** - all tests must pass before proceeding to the next phase.

## Principles

1. **Clean Slate**: Remove all legacy compute protocol code. Start fresh.
2. **No Backward Compatibility**: No support for deprecated methods or old protocols.
3. **Strict Phase Gates**: **ALL tests in a phase must pass before proceeding to the next phase.**
4. **TDD First**: Write tests first, then implement minimal code to pass.
5. **Edge Case Coverage**: Comprehensive test coverage including all failure modes.

## Requirements

### Core Requirements
1. **Raw Command Execution**: Execute arbitrary commands in terminal
2. **Byte-Level Streaming**: Direct stdin/stdout/stderr using `io.ReadWriteCloser` interfaces
3. **Multiplexed Control**: Control stream (stream 0) manages entire protocol lifespan
4. **Force Cancel**: Client can forcefully cancel running commands via control stream
5. **Long-Lived Log Stream**: Dedicated stream for realtime audit logs (stdout, stderr, stdin, metadata)
6. **Comprehensive Integration Tests**: Full workflow coverage including all edge cases

## Architecture Design

### Stream Allocation Strategy

```
Stream 0 (Control Stream):
  - Protocol handshake (hello/ack)
  - Command execution requests (run, cancel, status)
  - Control responses and errors
  - Lifecycle management

Stream 1: stdin (client → server)
  - Client writes to process stdin
  - Server reads from stream and pipes to process

Stream 2: stdout (server → client)
  - Server reads from process stdout
  - Server writes to stream for client to read

Stream 3: stderr (server → client)
  - Server reads from process stderr
  - Server writes to stream for client to read

Stream 4: Log stream (server → client)
  - Long-lived stream for audit logs
  - Structured JSON lines format
  - Realtime stdout/stderr/stdin capture
  - Execution metadata (start time, exit code, etc.)
```

**Decision**: Use separate streams for each I/O channel to avoid framing complexity and enable true concurrent I/O.

### Protocol Packet Schema

#### Control Stream Packets

```go
// Request to execute a raw command
type ComputeRunRequest struct {
    RunID      string            `json:"run_id"`       // Unique execution ID
    Command    string            `json:"command"`       // Command to execute
    Args       []string          `json:"args"`          // Command arguments
    Env        map[string]string `json:"env"`           // Environment variables (optional)
    WorkingDir string            `json:"working_dir"`   // Working directory (optional)
}

// Response to run request
type ComputeRunResponse struct {
    RunID      string `json:"run_id"`
    Accepted   bool   `json:"accepted"`
    StdinID    uint32 `json:"stdin_id"`    // Stream ID for stdin (0 = no stdin)
    StdoutID   uint32 `json:"stdout_id"`   // Stream ID for stdout
    StderrID   uint32 `json:"stderr_id"`   // Stream ID for stderr
    LogID      uint32 `json:"log_id"`      // Stream ID for audit logs
    Error      string `json:"error,omitempty"`
}

// Request to cancel execution
type ComputeCancelRequest struct {
    RunID string `json:"run_id"`
}

// Response to cancel request
type ComputeCancelResponse struct {
    RunID    string `json:"run_id"`
    Success  bool   `json:"success"`
    Error    string `json:"error,omitempty"`
}

// Request execution status
type ComputeStatusRequest struct {
    RunID string `json:"run_id"`
}

// Response with execution status
type ComputeStatusResponse struct {
    RunID     string `json:"run_id"`
    Status    string `json:"status"`     // "running", "completed", "canceled", "failed"
    ExitCode  *int   `json:"exit_code,omitempty"`
    StartTime string `json:"start_time,omitempty"`
    EndTime   string `json:"end_time,omitempty"`
    Error     string `json:"error,omitempty"`
}
```

#### Log Stream Format

The log stream carries structured audit entries as JSON lines:

```json
{"type":"metadata","run_id":"abc123","timestamp":"2025-01-01T12:00:00Z","event":"started","command":"ls -la","args":["-la"]}
{"type":"stdout","run_id":"abc123","timestamp":"2025-01-01T12:00:00Z","data":"file1.txt\nfile2.txt\n"}
{"type":"stderr","run_id":"abc123","timestamp":"2025-01-01T12:00:00Z","data":"warning: deprecated\n"}
{"type":"stdin","run_id":"abc123","timestamp":"2025-01-01T12:00:00Z","data":"user input\n"}
{"type":"metadata","run_id":"abc123","timestamp":"2025-01-01T12:00:00Z","event":"completed","exit_code":0}
```

## Implementation Plan

### Phase 0: Cleanup and Preparation ✅ COMPLETE

**Goal**: Remove all legacy compute protocol code and prepare clean foundation.

**Tasks**:
- [x] Delete all existing compute protocol implementation files
- [x] Remove legacy executor interfaces and implementations
- [x] Remove conda/python specific code from compute protocol
- [x] Clean up any deprecated test files
- [x] Verify no references to old compute protocol remain

**Files Deleted**:
- ✅ `p2p/protocols/compute/service/server.go`
- ✅ `p2p/protocols/compute/service/client.go`
- ✅ `p2p/protocols/compute/service/types.go`
- ✅ `p2p/protocols/compute/service/integration_test.go`
- ✅ `p2p/protocols/compute/service/server_test.go`
- ✅ `p2p/protocols/compute/service/test_helpers.go`
- ✅ `p2p/protocols/compute/service/handlers.go`
- ✅ `core/protocols/compute/compute.go` (replaced with stub)
- ✅ `core/protocols/compute/compute_test.go`
- ✅ `core/protocols/compute/executor.go`

**Phase Gate**: 
- ✅ All legacy files removed
- ✅ No compilation errors
- ✅ Clean directory structure ready for new implementation

---

### Phase 1: Protocol Packet Definitions

**Goal**: Define and test all protocol packets for raw command execution.

**Location**: `p2p/protocols/common/protocol/handshake.go`

**Tasks**:
- [ ] Add `HandshakeTypeComputeRun` constant
- [ ] Add `HandshakeTypeComputeRunResponse` constant
- [ ] Add `HandshakeTypeComputeCancel` constant
- [ ] Add `HandshakeTypeComputeCancelResponse` constant
- [ ] Add `HandshakeTypeComputeStatus` constant
- [ ] Add `HandshakeTypeComputeStatusResponse` constant
- [ ] Add `HandshakeTypeComputeError` constant
- [ ] Define `ComputeRunRequest` struct
- [ ] Define `ComputeRunResponse` struct
- [ ] Define `ComputeCancelRequest` struct
- [ ] Define `ComputeCancelResponse` struct
- [ ] Define `ComputeStatusRequest` struct
- [ ] Define `ComputeStatusResponse` struct
- [ ] Create `NewComputeRunPacket` function
- [ ] Create `NewComputeRunResponsePacket` function
- [ ] Create `NewComputeCancelPacket` function
- [ ] Create `NewComputeCancelResponsePacket` function
- [ ] Create `NewComputeStatusPacket` function
- [ ] Create `NewComputeStatusResponsePacket` function
- [ ] Create `NewComputeErrorPacket` function

**Test File**: `p2p/protocols/common/protocol/compute_packet_test.go`

**Required Test Cases** (ALL must pass):
```go
func TestComputeRunPacket_Marshal(t *testing.T)
func TestComputeRunPacket_Unmarshal(t *testing.T)
func TestComputeRunPacket_EmptyCommand(t *testing.T)
func TestComputeRunPacket_WithArgs(t *testing.T)
func TestComputeRunPacket_WithEnv(t *testing.T)
func TestComputeRunPacket_WithWorkingDir(t *testing.T)

func TestComputeRunResponsePacket_Marshal(t *testing.T)
func TestComputeRunResponsePacket_Unmarshal(t *testing.T)
func TestComputeRunResponsePacket_Accepted(t *testing.T)
func TestComputeRunResponsePacket_Rejected(t *testing.T)
func TestComputeRunResponsePacket_StreamIDs(t *testing.T)

func TestComputeCancelPacket_Marshal(t *testing.T)
func TestComputeCancelPacket_Unmarshal(t *testing.T)
func TestComputeCancelPacket_EmptyRunID(t *testing.T)

func TestComputeCancelResponsePacket_Marshal(t *testing.T)
func TestComputeCancelResponsePacket_Unmarshal(t *testing.T)
func TestComputeCancelResponsePacket_Success(t *testing.T)
func TestComputeCancelResponsePacket_Failure(t *testing.T)

func TestComputeStatusPacket_Marshal(t *testing.T)
func TestComputeStatusPacket_Unmarshal(t *testing.T)

func TestComputeStatusResponsePacket_Marshal(t *testing.T)
func TestComputeStatusResponsePacket_Unmarshal(t *testing.T)
func TestComputeStatusResponsePacket_AllStatuses(t *testing.T)
func TestComputeStatusResponsePacket_WithExitCode(t *testing.T)

func TestComputeErrorPacket_Marshal(t *testing.T)
func TestComputeErrorPacket_Unmarshal(t *testing.T)

func TestComputePackets_InvalidJSON(t *testing.T)
func TestComputePackets_MissingFields(t *testing.T)
```

**Phase Gate**: 
- ✅ All packet tests pass
- ✅ 100% code coverage for packet functions
- ✅ No linting errors

---

### Phase 2: Executor Interface and Implementation

**Goal**: Define clean executor interface and implement raw command execution.

**Location**: `p2p/protocols/compute/service/types.go` (new)

**Interface Definition**:
```go
type Executor interface {
    ExecuteRaw(ctx context.Context, req RawExecutionRequest) (*RawExecution, error)
}

type RawExecutionRequest struct {
    Command    string
    Args       []string
    Env        map[string]string
    WorkingDir string
    Stdin      io.Reader  // nil if no stdin
}

type RawExecution struct {
    RunID      string
    Stdin      io.WriteCloser  // Write to process stdin
    Stdout     io.ReadCloser   // Read from process stdout
    Stderr     io.ReadCloser   // Read from process stderr
    Done       <-chan error    // Signals completion
    ExitCode   <-chan int      // Exit code when done
    Cancel     context.CancelFunc
}
```

**Location**: `p2p/protocols/compute/service/executor.go` (new)

**Implementation**: Use `os/exec` package to execute commands.

**Test File**: `p2p/protocols/compute/service/executor_test.go`

**Required Test Cases** (ALL must pass):
```go
func TestRawExecutor_ExecuteSimpleCommand(t *testing.T)
func TestRawExecutor_ExecuteCommandWithArgs(t *testing.T)
func TestRawExecutor_ExecuteWithStdin(t *testing.T)
func TestRawExecutor_ExecuteWithEnv(t *testing.T)
func TestRawExecutor_ExecuteWithWorkingDir(t *testing.T)
func TestRawExecutor_ExecuteWithAllOptions(t *testing.T)
func TestRawExecutor_CancelExecution(t *testing.T)
func TestRawExecutor_ExitCode_Success(t *testing.T)
func TestRawExecutor_ExitCode_Failure(t *testing.T)
func TestRawExecutor_StdoutCapture(t *testing.T)
func TestRawExecutor_StderrCapture(t *testing.T)
func TestRawExecutor_ConcurrentExecutions(t *testing.T)
func TestRawExecutor_InvalidCommand(t *testing.T)
func TestRawExecutor_CommandNotFound(t *testing.T)
func TestRawExecutor_StdinClosure(t *testing.T)
func TestRawExecutor_LongRunningCommand(t *testing.T)
func TestRawExecutor_BinaryData(t *testing.T)
func TestRawExecutor_LargeOutput(t *testing.T)
func TestRawExecutor_ContextCancellation(t *testing.T)
func TestRawExecutor_ProcessKill(t *testing.T)
func TestRawExecutor_ResourceCleanup(t *testing.T)
```

**Phase Gate**: 
- ✅ All executor tests pass
- ✅ 100% code coverage for executor
- ✅ No resource leaks (verified with race detector)
- ✅ No linting errors

---

### Phase 3: Server Implementation

**Goal**: Implement server-side command execution handler with multiplexed streams.

**Location**: `p2p/protocols/compute/service/server.go` (new)

**Key Components**:
- Server struct with executor and run tracking
- `handleRun` method for executing commands
- `handleCancel` method for canceling executions
- `handleStatus` method for status queries
- Stream management (stdin/stdout/stderr/log)
- Log streaming goroutines
- Run monitoring and cleanup

**Test File**: `p2p/protocols/compute/service/server_test.go`

**Required Test Cases** (ALL must pass):
```go
func TestServer_HandleRun_Success(t *testing.T)
func TestServer_HandleRun_InvalidRequest(t *testing.T)
func TestServer_HandleRun_EmptyCommand(t *testing.T)
func TestServer_HandleRun_DuplicateRunID(t *testing.T)
func TestServer_HandleRun_StreamCreation(t *testing.T)
func TestServer_HandleRun_StreamIDsInResponse(t *testing.T)
func TestServer_HandleRun_ExecutorError(t *testing.T)
func TestServer_HandleRun_WithEnv(t *testing.T)
func TestServer_HandleRun_WithWorkingDir(t *testing.T)

func TestServer_HandleCancel_Success(t *testing.T)
func TestServer_HandleCancel_InvalidRunID(t *testing.T)
func TestServer_HandleCancel_AlreadyCompleted(t *testing.T)
func TestServer_HandleCancel_EmptyRunID(t *testing.T)

func TestServer_HandleStatus_Running(t *testing.T)
func TestServer_HandleStatus_Completed(t *testing.T)
func TestServer_HandleStatus_Canceled(t *testing.T)
func TestServer_HandleStatus_InvalidRunID(t *testing.T)

func TestServer_StreamStdout(t *testing.T)
func TestServer_StreamStderr(t *testing.T)
func TestServer_StreamLogs(t *testing.T)
func TestServer_StreamClosure(t *testing.T)

func TestServer_ConcurrentRuns(t *testing.T)
func TestServer_RunCleanup(t *testing.T)
func TestServer_MultiplexerErrors(t *testing.T)
```

**Phase Gate**: 
- ✅ All server tests pass
- ✅ 100% code coverage for server
- ✅ No race conditions (verified with race detector)
- ✅ Proper resource cleanup verified
- ✅ No linting errors

---

### Phase 4: Client Implementation

**Goal**: Implement client-side API for executing commands.

**Location**: `p2p/protocols/compute/service/client.go` (new)

**Key Components**:
- Client struct with multiplexer
- `Run` method for executing commands
- `Cancel` method for canceling executions
- `Status` method for status queries
- `RawExecutionHandle` struct for managing active executions
- Stream access methods

**Test File**: `p2p/protocols/compute/service/client_test.go`

**Required Test Cases** (ALL must pass):
```go
func TestClient_Run_Success(t *testing.T)
func TestClient_Run_StreamAccess(t *testing.T)
func TestClient_Run_StdinWrite(t *testing.T)
func TestClient_Run_StdoutRead(t *testing.T)
func TestClient_Run_StderrRead(t *testing.T)
func TestClient_Run_LogRead(t *testing.T)
func TestClient_Run_Rejected(t *testing.T)
func TestClient_Run_InvalidRequest(t *testing.T)
func TestClient_Run_Timeout(t *testing.T)

func TestClient_Cancel_Success(t *testing.T)
func TestClient_Cancel_InvalidRunID(t *testing.T)
func TestClient_Cancel_Timeout(t *testing.T)

func TestClient_Status_Success(t *testing.T)
func TestClient_Status_InvalidRunID(t *testing.T)
func TestClient_Status_Timeout(t *testing.T)

func TestClient_ConcurrentRuns(t *testing.T)
func TestClient_StreamErrors(t *testing.T)
func TestClient_ConnectionErrors(t *testing.T)
```

**Phase Gate**: 
- ✅ All client tests pass
- ✅ 100% code coverage for client
- ✅ No race conditions
- ✅ No linting errors

---

### Phase 5: Comprehensive Integration Tests

**Goal**: End-to-end integration tests covering all workflows and edge cases.

**Location**: `p2p/protocols/compute/service/integration_test.go` (new)

**Test Helper Functions**:
```go
func setupIntegrationTest(t *testing.T) (*Client, *Server, func())
func readStreamWithTimeout(r io.Reader, timeout time.Duration) ([]byte, error)
func verifyLogEntry(t *testing.T, logData []byte, expectedType string, expectedRunID string)
func waitForCompletion(t *testing.T, handle *RawExecutionHandle, timeout time.Duration)
```

**Required Test Cases** (ALL must pass):

#### Basic Execution Tests
```go
func TestIntegration_BasicCommandExecution(t *testing.T)
func TestIntegration_CommandWithArgs(t *testing.T)
func TestIntegration_CommandWithEnv(t *testing.T)
func TestIntegration_CommandWithWorkingDir(t *testing.T)
func TestIntegration_CommandWithStdin(t *testing.T)
func TestIntegration_CommandWithStdout(t *testing.T)
func TestIntegration_CommandWithStderr(t *testing.T)
func TestIntegration_CommandWithAllIO(t *testing.T)
```

#### Cancellation Tests
```go
func TestIntegration_ForceCancel(t *testing.T)
func TestIntegration_CancelLongRunning(t *testing.T)
func TestIntegration_CancelAfterCompletion(t *testing.T)
func TestIntegration_CancelInvalidRunID(t *testing.T)
```

#### Status Tests
```go
func TestIntegration_StatusRunning(t *testing.T)
func TestIntegration_StatusCompleted(t *testing.T)
func TestIntegration_StatusCanceled(t *testing.T)
func TestIntegration_StatusWithExitCode(t *testing.T)
```

#### Concurrent Execution Tests
```go
func TestIntegration_ConcurrentExecutions(t *testing.T)
func TestIntegration_ConcurrentWithSameCommand(t *testing.T)
func TestIntegration_ConcurrentCancel(t *testing.T)
func TestIntegration_ManyConcurrentExecutions(t *testing.T) // 100+ concurrent
```

#### Error Handling Tests
```go
func TestIntegration_InvalidCommand(t *testing.T)
func TestIntegration_CommandNotFound(t *testing.T)
func TestIntegration_NonZeroExitCode(t *testing.T)
func TestIntegration_StreamErrors(t *testing.T)
func TestIntegration_MultiplexerErrors(t *testing.T)
func TestIntegration_ControlStreamErrors(t *testing.T)
func TestIntegration_ServerShutdownDuringExecution(t *testing.T)
func TestIntegration_ClientDisconnectDuringExecution(t *testing.T)
```

#### Stream Handling Tests
```go
func TestIntegration_StreamClosure_Stdin(t *testing.T)
func TestIntegration_StreamClosure_Stdout(t *testing.T)
func TestIntegration_StreamClosure_Stderr(t *testing.T)
func TestIntegration_StreamClosure_Log(t *testing.T)
func TestIntegration_StreamClosure_AllStreams(t *testing.T)
func TestIntegration_LargeOutput(t *testing.T) // 10MB+ output
func TestIntegration_BinaryData(t *testing.T)
func TestIntegration_UnicodeData(t *testing.T)
func TestIntegration_EmptyOutput(t *testing.T)
```

#### Log Stream Tests
```go
func TestIntegration_LogStream_RealTime(t *testing.T)
func TestIntegration_LogStream_AllTypes(t *testing.T)
func TestIntegration_LogStream_Timestamps(t *testing.T)
func TestIntegration_LogStream_Metadata(t *testing.T)
func TestIntegration_LogStream_Ordering(t *testing.T)
func TestIntegration_LogStream_Completion(t *testing.T)
```

#### Edge Cases
```go
func TestIntegration_EmptyCommand(t *testing.T)
func TestIntegration_EmptyArgs(t *testing.T)
func TestIntegration_EmptyEnv(t *testing.T)
func TestIntegration_EmptyWorkingDir(t *testing.T)
func TestIntegration_NoStdin(t *testing.T)
func TestIntegration_NoStdout(t *testing.T)
func TestIntegration_NoStderr(t *testing.T)
func TestIntegration_VeryLongCommand(t *testing.T)
func TestIntegration_VeryLongArgs(t *testing.T)
func TestIntegration_SpecialCharacters(t *testing.T)
func TestIntegration_NewlinesInOutput(t *testing.T)
func TestIntegration_ZeroLengthOutput(t *testing.T)
```

#### Resource Management Tests
```go
func TestIntegration_ResourceCleanup_OnCompletion(t *testing.T)
func TestIntegration_ResourceCleanup_OnCancel(t *testing.T)
func TestIntegration_ResourceCleanup_OnError(t *testing.T)
func TestIntegration_ResourceCleanup_OnDisconnect(t *testing.T)
func TestIntegration_NoResourceLeaks(t *testing.T) // Use race detector
```

**Phase Gate**: 
- ✅ ALL integration tests pass (60+ test cases)
- ✅ No flaky tests (run 10 times, all pass)
- ✅ No race conditions (verified with race detector)
- ✅ No resource leaks (verified with profiling)
- ✅ Performance acceptable (< 100ms command start latency)
- ✅ Supports 100+ concurrent executions

---

### Phase 6: Public API and Stream Handler

**Goal**: Create public API and libp2p stream handler integration.

**Location**: `p2p/protocols/compute/compute.go` (new)

**Tasks**:
- [ ] Create `BindComputeStream` function for libp2p integration
- [ ] Create `computeStreamHandler` function
- [ ] Create public API functions for client usage
- [ ] Integration with existing node infrastructure

**Test File**: `p2p/protocols/compute/compute_test.go`

**Required Test Cases** (ALL must pass):
```go
func TestBindComputeStream(t *testing.T)
func TestComputeStreamHandler_Handshake(t *testing.T)
func TestComputeStreamHandler_Execution(t *testing.T)
func TestComputeStreamHandler_ErrorHandling(t *testing.T)
```

**Phase Gate**: 
- ✅ All public API tests pass
- ✅ Integration with libp2p verified
- ✅ No linting errors

---

### Phase 7: End-to-End Libp2p Tests

**Goal**: Full end-to-end tests with real libp2p networks.

**Location**: `tests/integration/compute_raw_test.go` (new)

**Required Test Cases** (ALL must pass):
```go
func TestE2E_DirectConnection(t *testing.T)
func TestE2E_RelayConnection(t *testing.T)
func TestE2E_ConcurrentExecutions(t *testing.T)
func TestE2E_MultipleNodes(t *testing.T)
func TestE2E_NetworkPartition(t *testing.T)
func TestE2E_Reconnection(t *testing.T)
```

**Phase Gate**: 
- ✅ All E2E tests pass
- ✅ Works over direct connections
- ✅ Works over relay connections
- ✅ Handles network failures gracefully

---

## File Structure

```
p2p/protocols/compute/
├── service/
│   ├── types.go                 # Interface definitions (new)
│   ├── executor.go              # Raw executor implementation (new)
│   ├── server.go                 # Server implementation (new)
│   ├── client.go                 # Client implementation (new)
│   ├── executor_test.go         # Executor tests (new)
│   ├── server_test.go            # Server tests (new)
│   ├── client_test.go            # Client tests (new)
│   ├── integration_test.go       # Integration tests (new)
│   └── test_helpers.go           # Test utilities (new)
└── compute.go                    # Public API (new)
```

## Success Criteria

### Functional Requirements
- ✅ Execute arbitrary raw commands
- ✅ Byte-level stdin/stdout/stderr streaming
- ✅ Force cancel via control stream
- ✅ Realtime log streaming
- ✅ Status monitoring
- ✅ ALL tests pass (unit + integration + E2E)

### Quality Requirements
- ✅ 90%+ code coverage
- ✅ ALL edge cases covered
- ✅ No race conditions (race detector clean)
- ✅ Proper resource cleanup (no leaks)
- ✅ Clear error messages
- ✅ No flaky tests

### Performance Requirements
- ✅ Command start latency < 100ms
- ✅ Efficient stream multiplexing
- ✅ Minimal memory overhead
- ✅ Support 100+ concurrent executions
- ✅ Handle 10MB+ output streams

## Phase Gate Enforcement

**CRITICAL**: Each phase has a mandatory gate. **DO NOT proceed to the next phase until:**
1. ✅ ALL tests in current phase pass
2. ✅ Code coverage requirements met
3. ✅ No linting errors
4. ✅ No race conditions
5. ✅ All edge cases covered

**If any test fails or requirement is not met, fix it before proceeding.**

## Dependencies

- `p2p/protocols/common/multiplexer` - Stream multiplexing
- `p2p/protocols/common/protocol` - Packet protocol
- `p2p/protocols/common/service` - Base service layer
- `os/exec` - Process execution
- `context` - Cancellation support

## References

- Proxy Protocol Implementation: `p2p/protocols/proxy/service/`
- File Protocol Implementation: `p2p/protocols/file/service/`
- Multiplexer: `p2p/protocols/common/multiplexer/`
- Protocol Layer: `p2p/protocols/common/protocol/`
- DEV.md: Integration guardrails and patterns

## Appendix: Example Usage

### Client-Side Usage

```go
// Create client
client := compute_service.NewClient("my-client", stream)
defer client.Close()

// Connect
if err := client.Connect(); err != nil {
    log.Fatal(err)
}

// Execute raw command
req := protocol.ComputeRunRequest{
    RunID:      "run-123",
    Command:    "python",
    Args:       []string{"-c", "print('Hello')"},
    Env:        map[string]string{"PYTHONPATH": "/opt/lib"},
    WorkingDir: "/tmp",
}

handle, err := client.Run(context.Background(), req)
if err != nil {
    log.Fatal(err)
}
defer handle.Cancel()

// Write to stdin
if handle.Stdin != nil {
    handle.Stdin.Write([]byte("input\n"))
    handle.Stdin.Close()
}

// Read from stdout
stdoutData, _ := io.ReadAll(handle.Stdout)
fmt.Printf("Output: %s\n", stdoutData)

// Read from stderr
stderrData, _ := io.ReadAll(handle.Stderr)
if len(stderrData) > 0 {
    fmt.Printf("Errors: %s\n", stderrData)
}

// Monitor logs
go func() {
    scanner := bufio.NewScanner(handle.Logs)
    for scanner.Scan() {
        var entry LogEntry
        json.Unmarshal(scanner.Bytes(), &entry)
        fmt.Printf("Log: %+v\n", entry)
    }
}()

// Wait for completion
<-handle.Done
```

### Server-Side Usage

```go
// Server is set up in stream handler
func computeStreamHandler(s network.Stream) {
    executor := compute_service.NewRawExecutor()
    server := compute_service.NewServer(s, executor)
    defer server.Close()
    
    if err := server.Accept(); err != nil {
        return
    }
    
    ctx := context.Background()
    if err := server.Serve(ctx); err != nil {
        log.Printf("Serve error: %v", err)
    }
}
```
