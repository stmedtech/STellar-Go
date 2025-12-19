package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"stellar/p2p/protocols/common/protocol"
	base_service "stellar/p2p/protocols/common/service"
	"stellar/p2p/protocols/compute/streams"
)

// RunStatus represents the status of a command execution
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusCanceled  RunStatus = "canceled"
	RunStatusFailed    RunStatus = "failed"
)

// RunInfo tracks information about an active command execution
type RunInfo struct {
	RunID      string
	Status     RunStatus
	StartTime  time.Time
	EndTime    *time.Time
	ExitCode   *int
	Execution  *RawExecution
	CancelFunc context.CancelFunc
	mu         sync.RWMutex
}

// Server handles server-side compute operations
type Server struct {
	*base_service.BaseServer
	executor Executor
	runs     map[string]*RunInfo
	runsMu   sync.RWMutex
}

// NewServer creates a new compute server with automatic multiplexer setup
func NewServer(conn io.ReadWriteCloser, executor Executor) *Server {
	if conn == nil {
		return nil
	}

	handshake := &computeHandshakeHandler{}
	dispatcher := &computePacketDispatcher{}

	base := base_service.NewBaseServer(conn, handshake, dispatcher)
	if base == nil {
		return nil
	}

	s := &Server{
		BaseServer: base,
		executor:   executor,
		runs:       make(map[string]*RunInfo),
	}
	dispatcher.server = s
	return s
}

// Accept performs handshake and accepts a client connection
func (s *Server) Accept() error {
	return s.BaseServer.Accept()
}

// Serve runs the control-plane event loop until the context is canceled or an error occurs.
func (s *Server) Serve(ctx context.Context) error {
	return s.BaseServer.Serve(ctx)
}

// Close shuts down the server and cancels any active runs.
// Always force-stops running processes to keep tests and protocol behavior deterministic.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}

	// Snapshot runs
	s.runsMu.RLock()
	runs := make([]*RunInfo, 0, len(s.runs))
	for _, ri := range s.runs {
		runs = append(runs, ri)
	}
	s.runsMu.RUnlock()

	// Force-cancel running processes and emit a terminal status event best-effort.
	for _, ri := range runs {
		if ri == nil {
			continue
		}
		ri.mu.Lock()
		if ri.Status == RunStatusRunning {
			ri.Status = RunStatusCanceled
			now := time.Now()
			ri.EndTime = &now
			// cancel process
			if ri.CancelFunc != nil {
				ri.CancelFunc()
			}
			// best-effort: notify client
			_ = s.sendStatusSuccess(ri.RunID, ri.Status, ri.ExitCode, ri.StartTime, ri.EndTime)
		}
		ri.mu.Unlock()
	}

	// Close underlying base server (control plane + multiplexer)
	return s.BaseServer.Close()
}

// computeHandshakeHandler implements server-side handshake handling for compute protocol
type computeHandshakeHandler struct{}

func (h *computeHandshakeHandler) ExpectedHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeComputeHello
}

func (h *computeHandshakeHandler) UnmarshalHelloPayload(payload []byte) (interface{}, error) {
	var hello protocol.ComputeHelloPayload
	if err := json.Unmarshal(payload, &hello); err != nil {
		return nil, fmt.Errorf("unmarshal hello payload: %w", err)
	}
	return hello, nil
}

func (h *computeHandshakeHandler) CreateAckPacket() (*protocol.Packet, error) {
	return protocol.NewComputeHelloAckPacket()
}

func (h *computeHandshakeHandler) ExtractClientID(helloPayload interface{}) string {
	payload, ok := helloPayload.(protocol.ComputeHelloPayload)
	if !ok {
		return ""
	}
	return payload.ClientID
}

// computePacketDispatcher routes control-plane packets to the compute server handlers
type computePacketDispatcher struct {
	server *Server
}

func (d *computePacketDispatcher) Dispatch(ctx context.Context, packet *protocol.HandshakePacket) error {
	if d == nil || d.server == nil {
		return fmt.Errorf("dispatcher not initialized")
	}
	switch packet.Type {
	case protocol.HandshakeTypeComputeRun:
		return d.server.handleRun(ctx, packet)
	case protocol.HandshakeTypeComputeCancel:
		return d.server.handleCancel(packet)
	case protocol.HandshakeTypeComputeStatus:
		return d.server.handleStatus(packet)
	default:
		return fmt.Errorf("unknown type: %s", packet.Type)
	}
}

// handleRun executes a raw command
func (s *Server) handleRun(ctx context.Context, packet *protocol.HandshakePacket) error {
	var req protocol.ComputeRunRequest
	if err := packet.UnmarshalPayload(&req); err != nil {
		return s.sendRunError(req.RunID, fmt.Sprintf("invalid request: %v", err))
	}

	// Validate request
	if req.RunID == "" {
		return s.sendRunError("", "run_id is required")
	}
	if req.Command == "" {
		return s.sendRunError(req.RunID, "command is required")
	}

	// Check for duplicate run ID
	s.runsMu.Lock()
	if _, exists := s.runs[req.RunID]; exists {
		s.runsMu.Unlock()
		return s.sendRunError(req.RunID, "run_id already exists")
	}
	s.runsMu.Unlock()

	// Allocate streams (server allocates all streams)
	stdinID := s.NextStreamID()
	stdoutID := s.NextStreamID()
	stderrID := s.NextStreamID()
	logID := s.NextStreamID()

	// Open streams
	stdinStream, err := s.Multiplexer().OpenStreamWithID(stdinID)
	if err != nil {
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open stdin stream: %v", err))
	}

	stdoutStream, err := s.Multiplexer().OpenStreamWithID(stdoutID)
	if err != nil {
		_ = s.Multiplexer().CloseStream(stdinID)
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open stdout stream: %v", err))
	}

	stderrStream, err := s.Multiplexer().OpenStreamWithID(stderrID)
	if err != nil {
		_ = s.Multiplexer().CloseStream(stdinID)
		_ = s.Multiplexer().CloseStream(stdoutID)
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open stderr stream: %v", err))
	}

	logStream, err := s.Multiplexer().OpenStreamWithID(logID)
	if err != nil {
		_ = s.Multiplexer().CloseStream(stdinID)
		_ = s.Multiplexer().CloseStream(stdoutID)
		_ = s.Multiplexer().CloseStream(stderrID)
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open log stream: %v", err))
	}

	// Check if this is a conda operation (__conda prefix)
	if req.Command == "__conda" {
		return s.handleCondaOperation(ctx, req, stdinID, stdoutID, stderrID, logID, stdinStream, stdoutStream, stderrStream, logStream)
	}

	// Create execution request
	// Note: Stdin in RawExecutionRequest is the source of stdin data
	// The client writes to stdinStream, so we read from it
	execReq := RawExecutionRequest{
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		Stdin:      stdinStream, // Server reads from this (client writes to it)
	}

	// Execute command
	execCtx, cancel := context.WithCancel(ctx)
	execution, err := s.executor.ExecuteRaw(execCtx, execReq)
	if err != nil {
		_ = s.Multiplexer().CloseStream(stdinID)
		_ = s.Multiplexer().CloseStream(stdoutID)
		_ = s.Multiplexer().CloseStream(stderrID)
		_ = s.Multiplexer().CloseStream(logID)
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to execute command: %v", err))
	}

	// Update execution RunID to match request
	execution.RunID = req.RunID

	// Create run info
	runInfo := &RunInfo{
		RunID:      req.RunID,
		Status:     RunStatusRunning,
		StartTime:  time.Now(),
		Execution:  execution,
		CancelFunc: cancel,
	}

	// Store run info
	s.runsMu.Lock()
	s.runs[req.RunID] = runInfo
	s.runsMu.Unlock()

	// Start stream forwarding goroutines BEFORE sending response
	// This ensures they're ready to forward data as soon as the command produces output
	var ioWg sync.WaitGroup
	ioWg.Add(2)
	go func() {
		defer ioWg.Done()
		s.forwardStdout(execution, stdoutStream, logStream, req.RunID)
	}()
	go func() {
		defer ioWg.Done()
		s.forwardStderr(execution, stderrStream, logStream, req.RunID)
	}()
	// Close log stream once stdout/stderr forwarding is done so the client can `io.ReadAll` it deterministically.
	go func() {
		ioWg.Wait()
		_ = logStream.Close()
	}()
	go s.monitorExecution(req.RunID, execution)

	// Send success response after starting forwarding goroutines
	if err := s.sendRunSuccess(req.RunID, stdinID, stdoutID, stderrID, logID); err != nil {
		// Cleanup on error
		s.cleanupRun(req.RunID)
		return err
	}

	return nil
}

// handleCancel cancels a running command
func (s *Server) handleCancel(packet *protocol.HandshakePacket) error {
	var req protocol.ComputeCancelRequest
	if err := packet.UnmarshalPayload(&req); err != nil {
		return s.sendCancelError(req.RunID, fmt.Sprintf("invalid request: %v", err))
	}

	if req.RunID == "" {
		return s.sendCancelError("", "run_id is required")
	}

	s.runsMu.Lock()
	runInfo, exists := s.runs[req.RunID]
	s.runsMu.Unlock()

	if !exists {
		return s.sendCancelError(req.RunID, "run_id not found")
	}

	runInfo.mu.Lock()
	status := runInfo.Status
	runInfo.mu.Unlock()

	if status != RunStatusRunning {
		return s.sendCancelError(req.RunID, fmt.Sprintf("run is not running (status: %s)", status))
	}

	// Cancel execution if CancelFunc is available
	runInfo.mu.Lock()
	cancelFunc := runInfo.CancelFunc
	runInfo.mu.Unlock()

	if cancelFunc != nil {
		cancelFunc()
	}

	runInfo.mu.Lock()
	runInfo.Status = RunStatusCanceled
	now := time.Now()
	runInfo.EndTime = &now
	runInfo.mu.Unlock()

	// Best-effort: also emit a status event so the client can complete deterministically without polling.
	_ = s.sendStatusSuccess(req.RunID, RunStatusCanceled, runInfo.ExitCode, runInfo.StartTime, runInfo.EndTime)

	return s.sendCancelSuccess(req.RunID)
}

// handleStatus returns the status of a command execution
func (s *Server) handleStatus(packet *protocol.HandshakePacket) error {
	var req protocol.ComputeStatusRequest
	if err := packet.UnmarshalPayload(&req); err != nil {
		return s.sendStatusError(req.RunID, fmt.Sprintf("invalid request: %v", err))
	}

	if req.RunID == "" {
		return s.sendStatusError("", "run_id is required")
	}

	s.runsMu.RLock()
	runInfo, exists := s.runs[req.RunID]
	s.runsMu.RUnlock()

	if !exists {
		return s.sendStatusError(req.RunID, "run_id not found")
	}

	runInfo.mu.RLock()
	status := runInfo.Status
	startTime := runInfo.StartTime
	endTime := runInfo.EndTime
	exitCode := runInfo.ExitCode
	runInfo.mu.RUnlock()

	return s.sendStatusSuccess(req.RunID, status, exitCode, startTime, endTime)
}

// ExecutionStreamReader interface moved to streams.go to avoid import cycles

// RawExecution adapter methods (for unified interface)
// Exported to satisfy ExecutionStreamReader interface
func (e *RawExecution) GetStdout() io.ReadCloser { return e.Stdout }
func (e *RawExecution) GetStderr() io.ReadCloser { return e.Stderr }
func (e *RawExecution) GetDone() <-chan error    { return e.Done }
func (e *RawExecution) GetExitCode() <-chan int  { return e.ExitCode }

// forwardStdout forwards stdout from execution to stream and logs
// Works with both RawExecution and CommandExecution via unified interface
func (s *Server) forwardStdout(execution streams.ExecutionStreamReader, stream io.WriteCloser, logStream io.WriteCloser, runID string) {
	defer stream.Close()

	buf := make([]byte, 4096)
	stdout := execution.GetStdout()
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			// Write to stdout stream
			if _, writeErr := stream.Write(buf[:n]); writeErr != nil {
				// Stream closed, stop forwarding
				return
			}

			// Log to log stream
			s.writeLogEntry(logStream, runID, "stdout", string(buf[:n]))
		}
		if err != nil {
			if err == io.EOF {
				// Normal end of stream
				return
			}
			// Log error
			s.writeLogEntry(logStream, runID, "error", fmt.Sprintf("stdout read error: %v", err))
			return
		}
		// If n == 0 and err == nil, continue reading (shouldn't happen but be safe)
	}
}

// forwardStderr forwards stderr from execution to stream and logs
// Works with both RawExecution and CommandExecution via unified interface
func (s *Server) forwardStderr(execution streams.ExecutionStreamReader, stream io.WriteCloser, logStream io.WriteCloser, runID string) {
	defer stream.Close()

	buf := make([]byte, 4096)
	stderr := execution.GetStderr()
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			// Write to stderr stream
			if _, writeErr := stream.Write(buf[:n]); writeErr != nil {
				// Stream closed, stop forwarding
				return
			}

			// Log to log stream
			s.writeLogEntry(logStream, runID, "stderr", string(buf[:n]))
		}
		if err != nil {
			if err == io.EOF {
				// Normal end of stream
				return
			}
			// Log error
			s.writeLogEntry(logStream, runID, "error", fmt.Sprintf("stderr read error: %v", err))
			return
		}
		// If n == 0 and err == nil, continue reading (shouldn't happen but be safe)
	}
}

// bridgeCondaExecution bridges a conda CommandExecution to multiplexer streams
// This is a DRY helper that handles all conda operation streaming uniformly
// No content transformation - pure raw redirection
// forwardingWg is used to track when forwarding goroutines complete
func (s *Server) bridgeCondaExecution(execution streams.ExecutionStreamReader, stdoutStream, stderrStream, logStream io.WriteCloser, runID string, runInfo *RunInfo, forwardingWg *sync.WaitGroup) {
	// Convert CommandExecution to RawExecution for monitoring
	cmdExec, ok := execution.(interface {
		GetRunID() string
		GetStdin() io.WriteCloser
		GetDone() <-chan error
		GetExitCode() <-chan int
		GetCancel() context.CancelFunc
	})
	if ok {
		cancelFunc := cmdExec.GetCancel()
		// Store execution in run info for monitoring
		runInfo.Execution = &RawExecution{
			RunID:    runID,
			Stdin:    cmdExec.GetStdin(),
			Stdout:   execution.GetStdout(),
			Stderr:   execution.GetStderr(),
			Done:     cmdExec.GetDone(),
			ExitCode: cmdExec.GetExitCode(),
			Cancel:   cancelFunc,
		}
		// Set CancelFunc in runInfo for cancellation support
		runInfo.mu.Lock()
		runInfo.CancelFunc = cancelFunc
		runInfo.mu.Unlock()
		// Start monitoring execution completion
		// Note: For createStringOutputExecution, channels are already closed with values.
		// handleCondaOperation reads from channels BEFORE calling bridgeCondaExecution,
		// so it should get the values first. monitorExecution will read zero values, but
		// handleCondaOperation's status update will overwrite monitorExecution's update.
		go s.monitorExecution(runID, runInfo.Execution)
	}

	// Bridge stdout/stderr to multiplexer streams and log (reuse proven forwardStdout/forwardStderr)
	forwardingWg.Add(2)
	go func() {
		defer forwardingWg.Done()
		s.forwardStdout(execution, stdoutStream, logStream, runID)
	}()
	go func() {
		defer forwardingWg.Done()
		s.forwardStderr(execution, stderrStream, logStream, runID)
	}()
	// Note: logStream will be closed by the caller after forwardingWg completes
}

// monitorExecution monitors execution completion and updates run info
func (s *Server) monitorExecution(runID string, execution *RawExecution) {
	// Wait for completion
	<-execution.Done

	// Get exit code
	var exitCode *int
	if code, ok := <-execution.ExitCode; ok {
		exitCode = &code
	}

	// Update run info
	s.runsMu.RLock()
	runInfo, exists := s.runs[runID]
	s.runsMu.RUnlock()

	if !exists {
		return
	}

	runInfo.mu.Lock()
	// Only update status if still running (handleCondaOperation may have already updated it)
	// This prevents race conditions where both handleCondaOperation and monitorExecution
	// try to update the status, especially for createStringOutputExecution where channels
	// are already closed and both might read from them
	if runInfo.Status == RunStatusRunning {
		if exitCode != nil && *exitCode == 0 {
			runInfo.Status = RunStatusCompleted
		} else {
			runInfo.Status = RunStatusFailed
		}
		now := time.Now()
		runInfo.EndTime = &now
		runInfo.ExitCode = exitCode
	}
	// Snapshot for event emission
	status := runInfo.Status
	startTime := runInfo.StartTime
	endTime := runInfo.EndTime
	code := runInfo.ExitCode
	runInfo.mu.Unlock()

	// Best-effort: emit a terminal status event (unmatched packets are delivered to eventsHandler on client side).
	// Note: handleCondaOperation also sends status events, so this may be redundant,
	// but it ensures status is sent even if handleCondaOperation hasn't run yet
	_ = s.sendStatusSuccess(runID, status, code, startTime, endTime)
}

// writeLogEntry writes a log entry to the log stream
func (s *Server) writeLogEntry(logStream io.WriteCloser, runID, logType, data string) {
	logEntry := map[string]interface{}{
		"run_id": runID,
		"type":   logType,
		"data":   data,
		"time":   time.Now().Format(time.RFC3339Nano),
	}

	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return
	}

	jsonData = append(jsonData, '\n')
	_, _ = logStream.Write(jsonData)
}

// cleanupRun removes a run from tracking
func (s *Server) cleanupRun(runID string) {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	delete(s.runs, runID)
}

// Response helpers
func (s *Server) sendRunSuccess(runID string, stdinID, stdoutID, stderrID, logID uint32) error {
	packet, err := protocol.NewComputeRunResponsePacket(runID, true, stdinID, stdoutID, stderrID, logID, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendRunError(runID, errMsg string) error {
	packet, err := protocol.NewComputeRunResponsePacket(runID, false, 0, 0, 0, 0, errMsg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendCancelSuccess(runID string) error {
	packet, err := protocol.NewComputeCancelResponsePacket(runID, true, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendCancelError(runID, errMsg string) error {
	packet, err := protocol.NewComputeCancelResponsePacket(runID, false, errMsg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendStatusSuccess(runID string, status RunStatus, exitCode *int, startTime time.Time, endTime *time.Time) error {
	var startTimeStr string
	if !startTime.IsZero() {
		startTimeStr = startTime.Format(time.RFC3339Nano)
	}

	var endTimeStr string
	if endTime != nil {
		endTimeStr = endTime.Format(time.RFC3339Nano)
	}

	resp := protocol.ComputeStatusResponse{
		RunID:     runID,
		Status:    string(status),
		ExitCode:  exitCode,
		StartTime: startTimeStr,
		EndTime:   endTimeStr,
	}

	packet, err := protocol.NewComputeStatusResponsePacket(resp)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

// handleCondaOperation handles __conda prefixed commands on the server side
// Format: __conda <subcommand> [args...]
// Uses shared CondaHandler for unified command handling
func (s *Server) handleCondaOperation(ctx context.Context, req protocol.ComputeRunRequest, stdinID, stdoutID, stderrID, logID uint32, stdinStream, stdoutStream, stderrStream, logStream io.ReadWriteCloser) error {
	// Helper function to close streams and send error
	closeStreamsAndSendError := func(errMsg string) error {
		// Close streams and remove from multiplexer map to prevent "stream already exists" errors
		_ = s.Multiplexer().CloseStream(stdinID)
		_ = s.Multiplexer().CloseStream(stdoutID)
		_ = s.Multiplexer().CloseStream(stderrID)
		_ = s.Multiplexer().CloseStream(logID)
		return s.sendRunError(req.RunID, errMsg)
	}

	if len(req.Args) == 0 {
		return closeStreamsAndSendError("__conda requires a subcommand")
	}

	subcommand := req.Args[0]
	args := req.Args[1:]

	// Create conda operations using creator pattern
	// The creator is automatically registered by conda package's init() when conda is imported
	// When running the full test suite, conda package is imported, so init() runs and registers creator
	creator := GetCondaOperationsCreator()
	if creator == nil {
		return closeStreamsAndSendError("conda operations not available: conda package not imported")
	}

	opsInterface, err := creator("")
	if err != nil {
		return closeStreamsAndSendError(fmt.Sprintf("failed to initialize conda operations: %v", err))
	}

	// Type assert to get handler interface
	handler, ok := opsInterface.(interface {
		HandleSubcommand(ctx context.Context, subcommand string, args []string, stdin io.Reader) (streams.ExecutionStreamReader, error)
	})
	if !ok {
		// Fallback: use wrapper if direct handler not available
		ops, ok := opsInterface.(CondaOperations)
		if !ok {
			return closeStreamsAndSendError("invalid conda operations type")
		}
		handler = &condaHandlerWrapper{ops: ops}
	}

	// Create run info for tracking
	runInfo := &RunInfo{
		RunID:     req.RunID,
		Status:    RunStatusRunning,
		StartTime: time.Now(),
	}

	s.runsMu.Lock()
	s.runs[req.RunID] = runInfo
	s.runsMu.Unlock()

	// Handle subcommand using shared handler
	var resultErr error
	var exitCode int
	forwardingWg := &sync.WaitGroup{}

	go func() {
		// Use shared handler to execute subcommand
		execution, err := handler.HandleSubcommand(ctx, subcommand, args, stdinStream)
		if err != nil {
			resultErr = err
			exitCode = 1
			fmt.Fprintf(stderrStream, "Error: %v\n", err)
			// Close streams on error and remove from multiplexer map
			_ = s.Multiplexer().CloseStream(stdinID)
			_ = s.Multiplexer().CloseStream(stdoutID)
			_ = s.Multiplexer().CloseStream(stderrID)
			_ = s.Multiplexer().CloseStream(logID)

			now := time.Now()
			runInfo.mu.Lock()
			runInfo.EndTime = &now
			runInfo.Status = RunStatusFailed
			runInfo.ExitCode = &exitCode
			runInfo.mu.Unlock()
			_ = s.sendStatusSuccess(req.RunID, RunStatusFailed, &exitCode, runInfo.StartTime, &now)
			return
		}

		// Bridge execution streams to multiplexer (raw streaming, no transformation)
		// This also sets up CancelFunc in runInfo if execution supports cancellation
		// bridgeCondaExecution starts forwarding goroutines and returns immediately
		s.bridgeCondaExecution(execution, stdoutStream, stderrStream, logStream, req.RunID, runInfo, forwardingWg)

		// Wait for completion - read from channels AFTER starting bridgeCondaExecution
		// This ensures monitorExecution has a chance to start, but we read the values here
		// For createStringOutputExecution, channels are already closed with values, so we get them immediately
		doneErr := <-execution.GetDone()
		if code, ok := <-execution.GetExitCode(); ok {
			exitCode = code
		} else {
			// Channel closed without value - for createStringOutputExecution, this shouldn't happen
			// because we send 0 before closing. But if it does, exitCode remains 0 (initialized)
		}
		if doneErr != nil {
			resultErr = doneErr
		}

		// Wait for forwarding to complete before closing streams
		// This ensures all data is forwarded, especially for stringOutputExecution
		forwardingWg.Wait()

		// Now safe to close streams and update status
		// This is the authoritative status update - monitorExecution may have already
		// tried to update status, but we overwrite it here with the correct values
		now := time.Now()
		runInfo.mu.Lock()
		runInfo.EndTime = &now
		// Debug: check what values we have
		// For createStringOutputExecution, doneErr should be nil and exitCode should be 0
		if resultErr == nil && exitCode == 0 {
			runInfo.Status = RunStatusCompleted
		} else {
			// If we're here, either resultErr is not nil or exitCode is not 0
			// For createStringOutputExecution, this shouldn't happen unless channels were read incorrectly
			runInfo.Status = RunStatusFailed
		}
		runInfo.ExitCode = &exitCode
		// Snapshot for event emission
		status := runInfo.Status
		startTime := runInfo.StartTime
		endTime := runInfo.EndTime
		code := runInfo.ExitCode
		runInfo.mu.Unlock()

		// Send authoritative status update (overwrites any status from monitorExecution)
		_ = s.sendStatusSuccess(req.RunID, status, code, startTime, endTime)

		// Close streams after forwarding is complete and remove from multiplexer map
		_ = s.Multiplexer().CloseStream(stdinID)
		_ = s.Multiplexer().CloseStream(stdoutID)
		_ = s.Multiplexer().CloseStream(stderrID)
		_ = s.Multiplexer().CloseStream(logID)
	}()

	// Send success response immediately (execution happens in goroutine)
	return s.sendRunSuccess(req.RunID, stdinID, stdoutID, stderrID, logID)
}

func (s *Server) sendStatusError(runID, errMsg string) error {
	resp := protocol.ComputeStatusResponse{
		RunID:  runID,
		Status: "",
		Error:  errMsg,
	}

	packet, err := protocol.NewComputeStatusResponsePacket(resp)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}
