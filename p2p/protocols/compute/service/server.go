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
// This is NOT backward-compatible with any legacy compute behavior: we always force-stop
// running processes to keep tests and protocol behavior deterministic.
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
		stdinStream.Close()
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open stdout stream: %v", err))
	}

	stderrStream, err := s.Multiplexer().OpenStreamWithID(stderrID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open stderr stream: %v", err))
	}

	logStream, err := s.Multiplexer().OpenStreamWithID(logID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		stderrStream.Close()
		return s.sendRunError(req.RunID, fmt.Sprintf("failed to open log stream: %v", err))
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
		stdinStream.Close()
		stdoutStream.Close()
		stderrStream.Close()
		logStream.Close()
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

	// Cancel execution
	runInfo.CancelFunc()
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

// forwardStdout forwards stdout from execution to stream and logs
func (s *Server) forwardStdout(execution *RawExecution, stream io.WriteCloser, logStream io.WriteCloser, runID string) {
	defer stream.Close()

	buf := make([]byte, 4096)
	for {
		n, err := execution.Stdout.Read(buf)
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
func (s *Server) forwardStderr(execution *RawExecution, stream io.WriteCloser, logStream io.WriteCloser, runID string) {
	defer stream.Close()

	buf := make([]byte, 4096)
	for {
		n, err := execution.Stderr.Read(buf)
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
