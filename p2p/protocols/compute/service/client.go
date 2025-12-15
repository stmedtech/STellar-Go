package service

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"stellar/p2p/protocols/common/protocol"
	base_service "stellar/p2p/protocols/common/service"
)

// Client handles client-side compute operations
type Client struct {
	*base_service.BaseClient
	events *computeEventsHandler
}

// computeEventsHandler receives unsolicited control-plane packets (i.e. packets that did not match
// a pending request) and completes runs without polling.
type computeEventsHandler struct {
	mu      sync.Mutex
	handles map[string]*RawExecutionHandle
}

func newComputeEventsHandler() *computeEventsHandler {
	return &computeEventsHandler{handles: make(map[string]*RawExecutionHandle)}
}

func (h *computeEventsHandler) register(handle *RawExecutionHandle) {
	if h == nil || handle == nil || handle.RunID == "" {
		return
	}
	h.mu.Lock()
	h.handles[handle.RunID] = handle
	h.mu.Unlock()
}

func (h *computeEventsHandler) unregister(runID string) {
	if h == nil || runID == "" {
		return
	}
	h.mu.Lock()
	delete(h.handles, runID)
	h.mu.Unlock()
}

func (h *computeEventsHandler) completeAll(err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	handles := make([]*RawExecutionHandle, 0, len(h.handles))
	for _, v := range h.handles {
		handles = append(handles, v)
	}
	h.handles = make(map[string]*RawExecutionHandle)
	h.mu.Unlock()

	for _, handle := range handles {
		handle.complete(err, -1)
	}
}

func (h *computeEventsHandler) HandleEvent(packet *protocol.HandshakePacket) {
	if h == nil || packet == nil {
		return
	}
	if packet.Type != protocol.HandshakeTypeComputeStatusResponse {
		return
	}

	var status protocol.ComputeStatusResponse
	if err := packet.UnmarshalPayload(&status); err != nil {
		return
	}
	if status.RunID == "" {
		return
	}

	h.mu.Lock()
	handle := h.handles[status.RunID]
	h.mu.Unlock()
	if handle == nil {
		return
	}

	switch status.Status {
	case string(RunStatusCompleted), string(RunStatusFailed), string(RunStatusCanceled):
		// Complete handle once.
		handle.completeFromStatus(status)
		h.unregister(status.RunID)
	default:
		// running/unknown: ignore
	}
}

// NewClient creates a new compute client with automatic multiplexer setup
func NewClient(clientID string, conn io.ReadWriteCloser) *Client {
	if conn == nil || clientID == "" {
		return nil
	}

	hello := &computeHelloHandler{}
	events := newComputeEventsHandler()
	base := base_service.NewBaseClient(clientID, conn, hello, events)
	if base == nil {
		return nil
	}

	return &Client{BaseClient: base, events: events}
}

// Connect performs handshake and establishes connection
func (c *Client) Connect() error {
	return c.BaseClient.Connect()
}

// Close closes the client and completes any active handles.
func (c *Client) Close() error {
	var err error
	if c != nil && c.BaseClient != nil {
		err = c.BaseClient.Close()
	}
	if c != nil && c.events != nil {
		c.events.completeAll(fmt.Errorf("connection closed"))
	}
	return err
}

// RawExecutionHandle manages an active command execution with stream access
type RawExecutionHandle struct {
	RunID    string
	Stdin    io.WriteCloser
	Stdout   io.ReadCloser
	Stderr   io.ReadCloser
	Log      io.ReadCloser
	Done     <-chan error
	ExitCode <-chan int
	Cancel   func() error
	doneCh   chan error
	exitCh   chan int
	once     sync.Once
}

// Run executes a raw command
func (c *Client) Run(ctx context.Context, req RunRequest) (*RawExecutionHandle, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}

	// Generate run ID if not provided
	runID := req.RunID
	if runID == "" {
		runID = generateRunID()
	}

	// Create channels + handle and register EARLY to avoid races where a terminal status event
	// arrives before Run() returns (e.g., fast commands like `echo`).
	doneCh := make(chan error, 1)
	exitCh := make(chan int, 1)
	handle := &RawExecutionHandle{
		RunID:    runID,
		Done:     doneCh,
		ExitCode: exitCh,
		doneCh:   doneCh,
		exitCh:   exitCh,
		Cancel: func() error {
			return c.Cancel(ctx, runID)
		},
	}
	if c.events != nil {
		c.events.register(handle)
	}

	// Create request packet
	computeReq := protocol.ComputeRunRequest{
		RunID:      runID,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
	}

	requestPacket, err := protocol.NewComputeRunPacket(computeReq)
	if err != nil {
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Send request and wait for response
	response, err := c.SendRequest(ctx, requestPacket, matchComputeRunResponse(runID))
	if err != nil {
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, err
	}

	var runResponse protocol.ComputeRunResponse
	if err := response.UnmarshalPayload(&runResponse); err != nil {
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !runResponse.Accepted {
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("run rejected: %s", runResponse.Error)
	}

	// Get or create streams
	stdinStream, err := c.GetOrCreateStream(runResponse.StdinID)
	if err != nil {
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("get stdin stream: %w", err)
	}

	stdoutStream, err := c.GetOrCreateStream(runResponse.StdoutID)
	if err != nil {
		stdinStream.Close()
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("get stdout stream: %w", err)
	}

	stderrStream, err := c.GetOrCreateStream(runResponse.StderrID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("get stderr stream: %w", err)
	}

	logStream, err := c.GetOrCreateStream(runResponse.LogID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		stderrStream.Close()
		if c.events != nil {
			c.events.unregister(runID)
		}
		return nil, fmt.Errorf("get log stream: %w", err)
	}

	// Attach streams to the already-registered handle.
	handle.Stdin = stdinStream
	handle.Stdout = stdoutStream
	handle.Stderr = stderrStream
	handle.Log = logStream

	// If the caller supplied a cancelable ctx, propagate cancellation to the handle.
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			handle.complete(ctx.Err(), -1)
			if c.events != nil {
				c.events.unregister(runID)
			}
		}()
	}

	return handle, nil
}

func (h *RawExecutionHandle) completeFromStatus(status protocol.ComputeStatusResponse) {
	exit := -1
	if status.ExitCode != nil {
		exit = *status.ExitCode
	}

	switch status.Status {
	case string(RunStatusCompleted):
		h.complete(nil, exit)
	case string(RunStatusCanceled):
		h.complete(fmt.Errorf("execution canceled"), -1)
	case string(RunStatusFailed):
		h.complete(fmt.Errorf("execution failed"), exit)
	default:
		// ignore
	}
}

func (h *RawExecutionHandle) complete(err error, exitCode int) {
	if h == nil {
		return
	}
	h.once.Do(func() {
		// Best-effort send; channels are buffered.
		select {
		case h.exitCh <- exitCode:
		default:
		}
		select {
		case h.doneCh <- err:
		default:
		}
		close(h.exitCh)
		close(h.doneCh)
	})
}

// Cancel cancels a running command execution
func (c *Client) Cancel(ctx context.Context, runID string) error {
	if err := c.EnsureConnected(); err != nil {
		return err
	}

	if runID == "" {
		return fmt.Errorf("run_id is required")
	}

	req := protocol.ComputeCancelRequest{
		RunID: runID,
	}

	requestPacket, err := protocol.NewComputeCancelPacket(req)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	response, err := c.SendRequest(ctx, requestPacket, matchComputeCancelResponse(runID))
	if err != nil {
		return err
	}

	var cancelResponse protocol.ComputeCancelResponse
	if err := response.UnmarshalPayload(&cancelResponse); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if !cancelResponse.Success {
		return fmt.Errorf("cancel failed: %s", cancelResponse.Error)
	}

	return nil
}

// Status returns the status of a command execution
func (c *Client) Status(ctx context.Context, runID string) (*StatusResponse, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}

	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	req := protocol.ComputeStatusRequest{
		RunID: runID,
	}

	requestPacket, err := protocol.NewComputeStatusPacket(req)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	response, err := c.SendRequest(ctx, requestPacket, matchComputeStatusResponse(runID))
	if err != nil {
		return nil, err
	}

	var statusResponse protocol.ComputeStatusResponse
	if err := response.UnmarshalPayload(&statusResponse); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if statusResponse.Error != "" {
		return nil, fmt.Errorf("status query failed: %s", statusResponse.Error)
	}

	result := &StatusResponse{
		RunID:    statusResponse.RunID,
		Status:   statusResponse.Status,
		ExitCode: statusResponse.ExitCode,
	}

	if statusResponse.StartTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, statusResponse.StartTime); err == nil {
			result.StartTime = &t
		}
	}

	if statusResponse.EndTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, statusResponse.EndTime); err == nil {
			result.EndTime = &t
		}
	}

	return result, nil
}

// RunRequest contains parameters for executing a command
type RunRequest struct {
	RunID      string
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
}

// StatusResponse contains the status of a command execution
type StatusResponse struct {
	RunID     string
	Status    string
	ExitCode  *int
	StartTime *time.Time
	EndTime   *time.Time
}

// computeHelloHandler implements client-side hello for compute protocol
type computeHelloHandler struct{}

func (h *computeHelloHandler) CreateHelloPacket(version, clientID string) (*protocol.Packet, error) {
	return protocol.NewComputeHelloPacket(version, clientID)
}

func (h *computeHelloHandler) ExpectedAckType() protocol.HandshakeType {
	return protocol.HandshakeTypeComputeHelloAck
}

func (h *computeHelloHandler) ErrorHandshakeType() protocol.HandshakeType {
	return protocol.HandshakeTypeComputeError
}

// Matcher functions for request/response matching

func matchComputeRunResponse(runID string) base_service.Matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		if packet.Type != protocol.HandshakeTypeComputeRunResponse {
			return false, nil
		}
		var resp protocol.ComputeRunResponse
		if err := packet.UnmarshalPayload(&resp); err != nil {
			return false, err
		}
		return resp.RunID == runID, nil
	}
}

func matchComputeCancelResponse(runID string) base_service.Matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		if packet.Type != protocol.HandshakeTypeComputeCancelResponse {
			return false, nil
		}
		var resp protocol.ComputeCancelResponse
		if err := packet.UnmarshalPayload(&resp); err != nil {
			return false, err
		}
		return resp.RunID == runID, nil
	}
}

func matchComputeStatusResponse(runID string) base_service.Matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		if packet.Type != protocol.HandshakeTypeComputeStatusResponse {
			return false, nil
		}
		var resp protocol.ComputeStatusResponse
		if err := packet.UnmarshalPayload(&resp); err != nil {
			return false, err
		}
		return resp.RunID == runID, nil
	}
}
