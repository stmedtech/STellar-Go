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
}

// NewClient creates a new compute client with automatic multiplexer setup
func NewClient(clientID string, conn io.ReadWriteCloser) *Client {
	if conn == nil || clientID == "" {
		return nil
	}

	hello := &computeHelloHandler{}
	base := base_service.NewBaseClient(clientID, conn, hello, nil)
	if base == nil {
		return nil
	}

	return &Client{BaseClient: base}
}

// Connect performs handshake and establishes connection
func (c *Client) Connect() error {
	return c.BaseClient.Connect()
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
	mu       sync.RWMutex
	closed   bool
	doneCh   chan error
	exitCh   chan int
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
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Send request and wait for response
	response, err := c.SendRequest(ctx, requestPacket, matchComputeRunResponse(runID))
	if err != nil {
		return nil, err
	}

	var runResponse protocol.ComputeRunResponse
	if err := response.UnmarshalPayload(&runResponse); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !runResponse.Accepted {
		return nil, fmt.Errorf("run rejected: %s", runResponse.Error)
	}

	// Get or create streams
	stdinStream, err := c.GetOrCreateStream(runResponse.StdinID)
	if err != nil {
		return nil, fmt.Errorf("get stdin stream: %w", err)
	}

	stdoutStream, err := c.GetOrCreateStream(runResponse.StdoutID)
	if err != nil {
		stdinStream.Close()
		return nil, fmt.Errorf("get stdout stream: %w", err)
	}

	stderrStream, err := c.GetOrCreateStream(runResponse.StderrID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		return nil, fmt.Errorf("get stderr stream: %w", err)
	}

	logStream, err := c.GetOrCreateStream(runResponse.LogID)
	if err != nil {
		stdinStream.Close()
		stdoutStream.Close()
		stderrStream.Close()
		return nil, fmt.Errorf("get log stream: %w", err)
	}

	// Create channels for completion tracking
	doneCh := make(chan error, 1)
	exitCh := make(chan int, 1)

	// Create handle
	handle := &RawExecutionHandle{
		RunID:    runID,
		Stdin:    stdinStream,
		Stdout:   stdoutStream,
		Stderr:   stderrStream,
		Log:      logStream,
		Done:     doneCh,
		ExitCode: exitCh,
		doneCh:   doneCh,
		exitCh:   exitCh,
		Cancel: func() error {
			return c.Cancel(ctx, runID)
		},
	}

	// Start monitoring goroutine
	go handle.monitor(ctx, c)

	return handle, nil
}

// monitor monitors the execution status and updates channels
func (h *RawExecutionHandle) monitor(ctx context.Context, client *Client) {
	defer close(h.doneCh)
	defer close(h.exitCh)

	// Poll for status updates
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus string
	for {
		select {
		case <-ctx.Done():
			h.doneCh <- ctx.Err()
			return
		case <-ticker.C:
			status, err := client.Status(ctx, h.RunID)
			if err != nil {
				// Continue polling on error
				continue
			}

			if status.Status != lastStatus {
				lastStatus = status.Status

				if status.Status == string(RunStatusCompleted) || status.Status == string(RunStatusFailed) {
					if status.ExitCode != nil {
						h.exitCh <- *status.ExitCode
					} else {
						h.exitCh <- -1
					}
					if status.Status == string(RunStatusFailed) {
						h.doneCh <- fmt.Errorf("execution failed")
					} else {
						h.doneCh <- nil
					}
					return
				}

				if status.Status == string(RunStatusCanceled) {
					h.exitCh <- -1
					h.doneCh <- fmt.Errorf("execution canceled")
					return
				}
			}
		}
	}
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
