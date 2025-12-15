package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeRunPacket_Marshal tests marshaling of ComputeRunRequest
func TestComputeRunPacket_Marshal(t *testing.T) {
	req := ComputeRunRequest{
		RunID:      "test-run-123",
		Command:    "ls",
		Args:       []string{"-la", "/tmp"},
		Env:        map[string]string{"PATH": "/usr/bin"},
		WorkingDir: "/home/user",
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)
	require.NotNil(t, packet)

	// Verify packet type
	assert.Equal(t, PacketTypeTunneling, packet.Type)

	// Unmarshal and verify
	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeRun, handshake.Type)

	var unmarshaledReq ComputeRunRequest
	err = handshake.UnmarshalPayload(&unmarshaledReq)
	require.NoError(t, err)
	assert.Equal(t, req.RunID, unmarshaledReq.RunID)
	assert.Equal(t, req.Command, unmarshaledReq.Command)
	assert.Equal(t, req.Args, unmarshaledReq.Args)
	assert.Equal(t, req.Env, unmarshaledReq.Env)
	assert.Equal(t, req.WorkingDir, unmarshaledReq.WorkingDir)
}

// TestComputeRunPacket_Unmarshal tests unmarshaling of ComputeRunRequest
func TestComputeRunPacket_Unmarshal(t *testing.T) {
	req := ComputeRunRequest{
		RunID:      "test-run-456",
		Command:    "echo",
		Args:       []string{"hello", "world"},
		Env:        map[string]string{"VAR1": "value1", "VAR2": "value2"},
		WorkingDir: "/tmp",
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, req, result)
}

// TestComputeRunPacket_EmptyCommand tests packet with empty command
func TestComputeRunPacket_EmptyCommand(t *testing.T) {
	req := ComputeRunRequest{
		RunID:   "test-run-empty",
		Command: "",
		Args:    []string{},
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "", result.Command)
}

// TestComputeRunPacket_WithArgs tests packet with command arguments
func TestComputeRunPacket_WithArgs(t *testing.T) {
	req := ComputeRunRequest{
		RunID:   "test-run-args",
		Command: "python",
		Args:    []string{"-c", "print('hello')"},
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, []string{"-c", "print('hello')"}, result.Args)
}

// TestComputeRunPacket_WithEnv tests packet with environment variables
func TestComputeRunPacket_WithEnv(t *testing.T) {
	req := ComputeRunRequest{
		RunID:   "test-run-env",
		Command: "env",
		Env: map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		},
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, req.Env, result.Env)
}

// TestComputeRunPacket_WithWorkingDir tests packet with working directory
func TestComputeRunPacket_WithWorkingDir(t *testing.T) {
	req := ComputeRunRequest{
		RunID:      "test-run-wd",
		Command:    "pwd",
		WorkingDir: "/home/user/project",
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "/home/user/project", result.WorkingDir)
}

// TestComputeRunResponsePacket_Marshal tests marshaling of ComputeRunResponse
func TestComputeRunResponsePacket_Marshal(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-123", true, 1, 2, 3, 4, "")
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeRunResponse, handshake.Type)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, "test-run-123", resp.RunID)
	assert.True(t, resp.Accepted)
	assert.Equal(t, uint32(1), resp.StdinID)
	assert.Equal(t, uint32(2), resp.StdoutID)
	assert.Equal(t, uint32(3), resp.StderrID)
	assert.Equal(t, uint32(4), resp.LogID)
}

// TestComputeRunResponsePacket_Unmarshal tests unmarshaling of ComputeRunResponse
func TestComputeRunResponsePacket_Unmarshal(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-456", false, 0, 5, 6, 7, "command not found")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, "test-run-456", resp.RunID)
	assert.False(t, resp.Accepted)
	assert.Equal(t, uint32(0), resp.StdinID)
	assert.Equal(t, uint32(5), resp.StdoutID)
	assert.Equal(t, uint32(6), resp.StderrID)
	assert.Equal(t, uint32(7), resp.LogID)
	assert.Equal(t, "command not found", resp.Error)
}

// TestComputeRunResponsePacket_Accepted tests accepted response
func TestComputeRunResponsePacket_Accepted(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-accepted", true, 10, 11, 12, 13, "")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Accepted)
	assert.Empty(t, resp.Error)
}

// TestComputeRunResponsePacket_Rejected tests rejected response
func TestComputeRunResponsePacket_Rejected(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-rejected", false, 0, 0, 0, 0, "invalid command")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.False(t, resp.Accepted)
	assert.Equal(t, "invalid command", resp.Error)
}

// TestComputeRunResponsePacket_StreamIDs tests stream IDs in response
func TestComputeRunResponsePacket_StreamIDs(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-streams", true, 100, 200, 300, 400, "")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, uint32(100), resp.StdinID)
	assert.Equal(t, uint32(200), resp.StdoutID)
	assert.Equal(t, uint32(300), resp.StderrID)
	assert.Equal(t, uint32(400), resp.LogID)
}

// TestComputeCancelPacket_Marshal tests marshaling of ComputeCancelRequest
func TestComputeCancelPacket_Marshal(t *testing.T) {
	req := ComputeCancelRequest{
		RunID: "test-run-cancel",
	}

	packet, err := NewComputeCancelPacket(req)
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeCancel, handshake.Type)

	var result ComputeCancelRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "test-run-cancel", result.RunID)
}

// TestComputeCancelPacket_Unmarshal tests unmarshaling of ComputeCancelRequest
func TestComputeCancelPacket_Unmarshal(t *testing.T) {
	req := ComputeCancelRequest{
		RunID: "test-run-cancel-2",
	}

	packet, err := NewComputeCancelPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeCancelRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, req, result)
}

// TestComputeCancelPacket_EmptyRunID tests packet with empty run ID
func TestComputeCancelPacket_EmptyRunID(t *testing.T) {
	req := ComputeCancelRequest{
		RunID: "",
	}

	packet, err := NewComputeCancelPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeCancelRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Empty(t, result.RunID)
}

// TestComputeCancelResponsePacket_Marshal tests marshaling of ComputeCancelResponse
func TestComputeCancelResponsePacket_Marshal(t *testing.T) {
	packet, err := NewComputeCancelResponsePacket("test-run-cancel", true, "")
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeCancelResponse, handshake.Type)

	var resp ComputeCancelResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, "test-run-cancel", resp.RunID)
	assert.True(t, resp.Success)
}

// TestComputeCancelResponsePacket_Unmarshal tests unmarshaling of ComputeCancelResponse
func TestComputeCancelResponsePacket_Unmarshal(t *testing.T) {
	packet, err := NewComputeCancelResponsePacket("test-run-cancel-2", false, "run not found")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeCancelResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, "test-run-cancel-2", resp.RunID)
	assert.False(t, resp.Success)
	assert.Equal(t, "run not found", resp.Error)
}

// TestComputeCancelResponsePacket_Success tests successful cancel response
func TestComputeCancelResponsePacket_Success(t *testing.T) {
	packet, err := NewComputeCancelResponsePacket("test-run-success", true, "")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeCancelResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)
}

// TestComputeCancelResponsePacket_Failure tests failed cancel response
func TestComputeCancelResponsePacket_Failure(t *testing.T) {
	packet, err := NewComputeCancelResponsePacket("test-run-failure", false, "run already completed")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeCancelResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "run already completed", resp.Error)
}

// TestComputeStatusPacket_Marshal tests marshaling of ComputeStatusRequest
func TestComputeStatusPacket_Marshal(t *testing.T) {
	req := ComputeStatusRequest{
		RunID: "test-run-status",
	}

	packet, err := NewComputeStatusPacket(req)
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeStatus, handshake.Type)

	var result ComputeStatusRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "test-run-status", result.RunID)
}

// TestComputeStatusPacket_Unmarshal tests unmarshaling of ComputeStatusRequest
func TestComputeStatusPacket_Unmarshal(t *testing.T) {
	req := ComputeStatusRequest{
		RunID: "test-run-status-2",
	}

	packet, err := NewComputeStatusPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeStatusRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, req, result)
}

// TestComputeStatusResponsePacket_Marshal tests marshaling of ComputeStatusResponse
func TestComputeStatusResponsePacket_Marshal(t *testing.T) {
	exitCode := 0
	resp := ComputeStatusResponse{
		RunID:     "test-run-status",
		Status:    "completed",
		ExitCode:  &exitCode,
		StartTime: "2025-01-01T12:00:00Z",
		EndTime:   "2025-01-01T12:00:05Z",
	}

	packet, err := NewComputeStatusResponsePacket(resp)
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeStatusResponse, handshake.Type)

	var result ComputeStatusResponse
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, resp.RunID, result.RunID)
	assert.Equal(t, resp.Status, result.Status)
	assert.NotNil(t, result.ExitCode)
	assert.Equal(t, 0, *result.ExitCode)
}

// TestComputeStatusResponsePacket_Unmarshal tests unmarshaling of ComputeStatusResponse
func TestComputeStatusResponsePacket_Unmarshal(t *testing.T) {
	exitCode := 1
	resp := ComputeStatusResponse{
		RunID:    "test-run-status-2",
		Status:   "failed",
		ExitCode: &exitCode,
		Error:    "command failed",
	}

	packet, err := NewComputeStatusResponsePacket(resp)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeStatusResponse
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, resp.RunID, result.RunID)
	assert.Equal(t, resp.Status, result.Status)
	assert.NotNil(t, result.ExitCode)
	assert.Equal(t, 1, *result.ExitCode)
	assert.Equal(t, "command failed", result.Error)
}

// TestComputeStatusResponsePacket_AllStatuses tests all possible status values
func TestComputeStatusResponsePacket_AllStatuses(t *testing.T) {
	statuses := []string{"running", "completed", "canceled", "failed"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			resp := ComputeStatusResponse{
				RunID:  "test-run-" + status,
				Status: status,
			}

			packet, err := NewComputeStatusResponsePacket(resp)
			require.NoError(t, err)

			handshake, err := UnmarshalHandshakePacket(packet)
			require.NoError(t, err)

			var result ComputeStatusResponse
			err = handshake.UnmarshalPayload(&result)
			require.NoError(t, err)
			assert.Equal(t, status, result.Status)
		})
	}
}

// TestComputeStatusResponsePacket_WithExitCode tests status response with exit code
func TestComputeStatusResponsePacket_WithExitCode(t *testing.T) {
	exitCode := 42
	resp := ComputeStatusResponse{
		RunID:    "test-run-exit",
		Status:   "completed",
		ExitCode: &exitCode,
	}

	packet, err := NewComputeStatusResponsePacket(resp)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeStatusResponse
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.NotNil(t, result.ExitCode)
	assert.Equal(t, 42, *result.ExitCode)
}

// TestComputeErrorPacket_Marshal tests marshaling of ComputeErrorPayload
func TestComputeErrorPacket_Marshal(t *testing.T) {
	packet, err := NewComputeErrorPacket("invalid_request", "command is required")
	require.NoError(t, err)
	require.NotNil(t, packet)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)
	assert.Equal(t, HandshakeTypeComputeError, handshake.Type)

	var payload ComputeErrorPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request", payload.Code)
	assert.Equal(t, "command is required", payload.Detail)
}

// TestComputeErrorPacket_Unmarshal tests unmarshaling of ComputeErrorPayload
func TestComputeErrorPacket_Unmarshal(t *testing.T) {
	packet, err := NewComputeErrorPacket("execution_error", "process failed to start")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var payload ComputeErrorPayload
	err = handshake.UnmarshalPayload(&payload)
	require.NoError(t, err)
	assert.Equal(t, "execution_error", payload.Code)
	assert.Equal(t, "process failed to start", payload.Detail)
}

// TestComputePackets_InvalidJSON tests handling of invalid JSON
func TestComputePackets_InvalidJSON(t *testing.T) {
	// Create a packet with invalid JSON in payload
	invalidJSON := []byte(`{"type":"compute_run","payload":"invalid json}`)
	packet := &Packet{
		Type:    PacketTypeTunneling,
		Content: invalidJSON,
	}

	_, err := UnmarshalHandshakePacket(packet)
	assert.Error(t, err)
}

// TestComputePackets_MissingFields tests handling of missing required fields
func TestComputePackets_MissingFields(t *testing.T) {
	// Test with missing run_id
	req := ComputeRunRequest{
		Command: "ls",
		// RunID is missing
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err) // Packet creation should succeed

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Empty(t, result.RunID) // Should handle missing field gracefully
}

// TestComputePackets_EmptyArgs tests packet with empty args slice
func TestComputePackets_EmptyArgs(t *testing.T) {
	req := ComputeRunRequest{
		RunID:   "test-run-empty-args",
		Command: "echo",
		Args:    []string{},
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Empty(t, result.Args)
}

// TestComputePackets_EmptyEnv tests packet with empty env map
func TestComputePackets_EmptyEnv(t *testing.T) {
	req := ComputeRunRequest{
		RunID:   "test-run-empty-env",
		Command: "env",
		Env:     map[string]string{},
	}

	packet, err := NewComputeRunPacket(req)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeRunRequest
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Empty(t, result.Env)
}

// TestComputePackets_NilExitCode tests status response with nil exit code
func TestComputePackets_NilExitCode(t *testing.T) {
	resp := ComputeStatusResponse{
		RunID:    "test-run-nil-exit",
		Status:   "running",
		ExitCode: nil, // No exit code yet
	}

	packet, err := NewComputeStatusResponsePacket(resp)
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var result ComputeStatusResponse
	err = handshake.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Nil(t, result.ExitCode)
}

// TestComputePackets_ZeroStreamIDs tests response with zero stream IDs
func TestComputePackets_ZeroStreamIDs(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-zero", false, 0, 0, 0, 0, "error")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), resp.StdinID)
	assert.Equal(t, uint32(0), resp.StdoutID)
	assert.Equal(t, uint32(0), resp.StderrID)
	assert.Equal(t, uint32(0), resp.LogID)
}

// TestComputePackets_LargeStreamIDs tests response with large stream IDs
func TestComputePackets_LargeStreamIDs(t *testing.T) {
	packet, err := NewComputeRunResponsePacket("test-run-large", true, 4294967295, 4294967294, 4294967293, 4294967292, "")
	require.NoError(t, err)

	handshake, err := UnmarshalHandshakePacket(packet)
	require.NoError(t, err)

	var resp ComputeRunResponse
	err = handshake.UnmarshalPayload(&resp)
	require.NoError(t, err)
	assert.Equal(t, uint32(4294967295), resp.StdinID)
	assert.Equal(t, uint32(4294967294), resp.StdoutID)
	assert.Equal(t, uint32(4294967293), resp.StderrID)
	assert.Equal(t, uint32(4294967292), resp.LogID)
}

// TestComputePackets_JSONRoundTrip tests complete JSON round-trip for all packet types
func TestComputePackets_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		create func() (*Packet, error)
		verify func(*testing.T, *HandshakePacket)
	}{
		{
			name: "RunRequest",
			create: func() (*Packet, error) {
				return NewComputeRunPacket(ComputeRunRequest{
					RunID:   "roundtrip-1",
					Command: "test",
					Args:    []string{"arg1", "arg2"},
				})
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeRun, h.Type)
			},
		},
		{
			name: "RunResponse",
			create: func() (*Packet, error) {
				return NewComputeRunResponsePacket("roundtrip-2", true, 1, 2, 3, 4, "")
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeRunResponse, h.Type)
			},
		},
		{
			name: "CancelRequest",
			create: func() (*Packet, error) {
				return NewComputeCancelPacket(ComputeCancelRequest{RunID: "roundtrip-3"})
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeCancel, h.Type)
			},
		},
		{
			name: "CancelResponse",
			create: func() (*Packet, error) {
				return NewComputeCancelResponsePacket("roundtrip-4", true, "")
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeCancelResponse, h.Type)
			},
		},
		{
			name: "StatusRequest",
			create: func() (*Packet, error) {
				return NewComputeStatusPacket(ComputeStatusRequest{RunID: "roundtrip-5"})
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeStatus, h.Type)
			},
		},
		{
			name: "StatusResponse",
			create: func() (*Packet, error) {
				return NewComputeStatusResponsePacket(ComputeStatusResponse{
					RunID:  "roundtrip-6",
					Status: "completed",
				})
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeStatusResponse, h.Type)
			},
		},
		{
			name: "Error",
			create: func() (*Packet, error) {
				return NewComputeErrorPacket("test_error", "test detail")
			},
			verify: func(t *testing.T, h *HandshakePacket) {
				assert.Equal(t, HandshakeTypeComputeError, h.Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := tt.create()
			require.NoError(t, err)

			// Marshal to JSON
			jsonData, err := json.Marshal(packet)
			require.NoError(t, err)

			// Unmarshal from JSON
			var unmarshaledPacket Packet
			err = json.Unmarshal(jsonData, &unmarshaledPacket)
			require.NoError(t, err)

			// Verify handshake
			handshake, err := UnmarshalHandshakePacket(&unmarshaledPacket)
			require.NoError(t, err)
			tt.verify(t, handshake)
		})
	}
}
