package protocol

import (
	"encoding/json"
	"fmt"
)

// HandshakeType represents the type of handshake message
type HandshakeType string

const (
	HandshakeTypeHello         HandshakeType = "hello"
	HandshakeTypeHelloAck      HandshakeType = "hello_ack"
	HandshakeTypeProxyOpen     HandshakeType = "proxy_open"
	HandshakeTypeProxyOpened   HandshakeType = "proxy_opened"
	HandshakeTypeProxyClose    HandshakeType = "proxy_close"
	HandshakeTypeProxyClosed   HandshakeType = "proxy_closed"
	HandshakeTypeProxyList     HandshakeType = "proxy_list"
	HandshakeTypeProxyListResp HandshakeType = "proxy_list_response"
	HandshakeTypeError         HandshakeType = "error"
	// File protocol handshake types
	HandshakeTypeFileHello        HandshakeType = "file_hello"
	HandshakeTypeFileHelloAck     HandshakeType = "file_hello_ack"
	HandshakeTypeFileList         HandshakeType = "file_list"
	HandshakeTypeFileListResponse HandshakeType = "file_list_response"
	HandshakeTypeFileGet          HandshakeType = "file_get"
	HandshakeTypeFileGetResponse  HandshakeType = "file_get_response"
	HandshakeTypeFileSend         HandshakeType = "file_send"
	HandshakeTypeFileSendResponse HandshakeType = "file_send_response"
	HandshakeTypeFileError        HandshakeType = "file_error"
	// Compute protocol handshake types
	HandshakeTypeComputeHello            HandshakeType = "compute_hello"
	HandshakeTypeComputeHelloAck         HandshakeType = "compute_hello_ack"
	HandshakeTypeComputeRun              HandshakeType = "compute_run"
	HandshakeTypeComputeRunResponse      HandshakeType = "compute_run_resp"
	HandshakeTypeComputeCancel           HandshakeType = "compute_cancel"
	HandshakeTypeComputeCancelResponse   HandshakeType = "compute_cancel_resp"
	HandshakeTypeComputeListEnvs         HandshakeType = "compute_list_envs"
	HandshakeTypeComputeListEnvsResponse HandshakeType = "compute_list_envs_resp"
	HandshakeTypeComputeLogOpen          HandshakeType = "compute_log_open"
	HandshakeTypeComputeLogClose         HandshakeType = "compute_log_close"
	HandshakeTypeComputeStatus           HandshakeType = "compute_status"
	HandshakeTypeComputeStatusResponse   HandshakeType = "compute_status_resp"
	HandshakeTypeComputeError            HandshakeType = "compute_error"
)

// HandshakePacket represents a handshake message encapsulated within a Packet
type HandshakePacket struct {
	Type    HandshakeType   `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HelloPayload is the payload for the initial hello handshake
type HelloPayload struct {
	Version  string `json:"version"`
	ClientID string `json:"client_id"`
}

// ProxyOpenRequest is the payload for requesting a new proxy connection
type ProxyOpenRequest struct {
	ProxyID    string `json:"proxy_id"`
	RemoteAddr string `json:"remote_addr"`
	Protocol   string `json:"protocol"` // "tcp" or "udp"
}

// ProxyCloseRequest is the payload for closing an existing proxy connection
type ProxyCloseRequest struct {
	ProxyID string `json:"proxy_id"`
}

// ProxyClosedResponse is the payload returned after attempting to close a proxy
type ProxyClosedResponse struct {
	ProxyID string `json:"proxy_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ProxyListRequest is the payload for requesting a list of active proxies
type ProxyListRequest struct{}

// ProxyOpenResponse is the response to proxy open request
type ProxyOpenResponse struct {
	ProxyID  string `json:"proxy_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	StreamID uint32 `json:"stream_id,omitempty"` // For nested packet stream
}

// ProxyListResponse contains list of active proxies
type ProxyListResponse struct {
	Proxies []ProxyInfo `json:"proxies"`
}

// ProxyInfo contains information about an active proxy
type ProxyInfo struct {
	ProxyID    string `json:"proxy_id"`
	RemoteAddr string `json:"remote_addr"`
	Protocol   string `json:"protocol"`
	Status     string `json:"status"` // "active", "closing", "closed"
}

// ErrorPayload contains error information
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Compute protocol payloads
type ComputeHelloPayload struct {
	ClientID string `json:"client_id"`
	Version  string `json:"version"`
}

type ComputeHelloAckPayload struct {
	ServerID string `json:"server_id"`
	Version  string `json:"version"`
}

type ComputeRunRequest struct {
	RunID      string            `json:"run_id"`      // Unique execution ID
	Command    string            `json:"command"`     // Command to execute
	Args       []string          `json:"args"`        // Command arguments
	Env        map[string]string `json:"env"`         // Environment variables (optional)
	WorkingDir string            `json:"working_dir"` // Working directory (optional)
}

type ComputeRunResponse struct {
	RunID    string `json:"run_id"`
	Accepted bool   `json:"accepted"`
	StdinID  uint32 `json:"stdin_id"`  // Stream ID for stdin (0 = no stdin)
	StdoutID uint32 `json:"stdout_id"` // Stream ID for stdout
	StderrID uint32 `json:"stderr_id"` // Stream ID for stderr
	LogID    uint32 `json:"log_id"`    // Stream ID for audit logs
	Error    string `json:"error,omitempty"`
}

type ComputeCancelRequest struct {
	RunID string `json:"run_id"`
}

type ComputeCancelResponse struct {
	RunID   string `json:"run_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type ComputeListEnvsRequest struct {
	// Empty - no parameters needed
}

type ComputeListEnvsResponse struct {
	Envs  map[string]string `json:"envs,omitempty"` // env name -> path
	Error string            `json:"error,omitempty"`
}

type ComputeLogOpen struct {
	RunID    string `json:"run_id"`
	StreamID uint32 `json:"stream_id"`
}

type ComputeLogClose struct {
	RunID string `json:"run_id"`
}

type ComputeStatusRequest struct {
	RunID string `json:"run_id"`
}

type ComputeStatusResponse struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"` // "running", "completed", "canceled", "failed"
	ExitCode  *int   `json:"exit_code,omitempty"`
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ComputeErrorPayload struct {
	RunID  string `json:"run_id,omitempty"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// FileHelloPayload is the payload for the initial file protocol hello handshake
type FileHelloPayload struct {
	Version  string   `json:"version"`
	ClientID string   `json:"client_id"`
	Features []string `json:"features"` // e.g., "list", "get", "send", "checksum", "recursive"
}

// FileListRequest is the payload for requesting a file/directory listing
type FileListRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// FileListResponse is the response to a file list request
type FileListResponse struct {
	Success bool        `json:"success"`
	Files   []FileEntry `json:"files,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// FileEntry represents a file or directory entry (matches the existing FileEntry struct)
type FileEntry struct {
	DirectoryName string      `json:"directory_name"`
	Filename      string      `json:"filename"`
	Size          int64       `json:"size"`
	IsDir         bool        `json:"is_dir"`
	Children      []FileEntry `json:"children,omitempty"`
}

// FileGetRequest is the payload for requesting a file download
type FileGetRequest struct {
	FileName string `json:"file_name"`
}

// FileGetResponse is the response to a file get request
type FileGetResponse struct {
	Success  bool      `json:"success"`
	StreamID uint32    `json:"stream_id,omitempty"` // Data stream ID for file transfer
	FileInfo *FileInfo `json:"file_info,omitempty"` // File metadata (size, checksum)
	Error    string    `json:"error,omitempty"`
}

// FileInfo contains file metadata
type FileInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// FileSendRequest is the payload for requesting a file upload
type FileSendRequest struct {
	FileName string `json:"file_name"`
}

// FileSendResponse is the response to a file send request
type FileSendResponse struct {
	Success  bool   `json:"success"`
	StreamID uint32 `json:"stream_id,omitempty"` // Data stream ID for file transfer
	Error    string `json:"error,omitempty"`
}

// NewHandshakePacket creates a new handshake packet with the given type and payload
func NewHandshakePacket(handshakeType HandshakeType, payload interface{}) (*Packet, error) {
	var payloadJSON json.RawMessage
	if payload != nil {
		var err error
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal handshake payload: %w", err)
		}
	}

	handshake := HandshakePacket{
		Type:    handshakeType,
		Payload: payloadJSON,
	}

	handshakeJSON, err := json.Marshal(handshake)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handshake: %w", err)
	}

	return NewTunnelingPacket(handshakeJSON)
}

// UnmarshalHandshakePacket unmarshals a packet into a handshake packet
func UnmarshalHandshakePacket(packet *Packet) (*HandshakePacket, error) {
	if packet == nil {
		return nil, fmt.Errorf("packet is nil")
	}

	if packet.Type != PacketTypeTunneling {
		return nil, fmt.Errorf("packet is not a tunneling packet, got type: %s", packet.Type)
	}

	// First, unmarshal the tunneling content to get the actual data
	tunnelingContent, err := packet.UnmarshalTunnelingContent()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tunneling content: %w", err)
	}

	// The tunneling content's Data field contains the handshake JSON
	var handshake HandshakePacket
	if err := json.Unmarshal(tunnelingContent.Data, &handshake); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake packet content: %w", err)
	}

	return &handshake, nil
}

// UnmarshalPayload extracts the payload from a handshake packet
func (h *HandshakePacket) UnmarshalPayload(v interface{}) error {
	if h == nil {
		return fmt.Errorf("handshake packet is nil")
	}
	if len(h.Payload) == 0 {
		return nil // Empty payload is valid
	}
	if v == nil {
		return fmt.Errorf("target is nil")
	}
	if err := json.Unmarshal(h.Payload, v); err != nil {
		return fmt.Errorf("failed to unmarshal handshake payload: %w", err)
	}
	return nil
}

// MarshalPayload marshals the payload of a handshake packet
func (h *HandshakePacket) MarshalPayload(v interface{}) error {
	if h == nil {
		return fmt.Errorf("handshake packet is nil")
	}
	var err error
	h.Payload, err = json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake payload: %w", err)
	}
	return nil
}

// Convenience functions for creating common handshake packets

// NewHelloPacket creates a hello packet
func NewHelloPacket(version, clientID string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeHello, HelloPayload{
		Version:  version,
		ClientID: clientID,
	})
}

// NewHelloAckPacket creates a hello_ack packet
func NewHelloAckPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeHelloAck, nil)
}

// NewProxyOpenPacket creates a proxy_open packet
func NewProxyOpenPacket(proxyID, remoteAddr, protocol string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyOpen, ProxyOpenRequest{
		ProxyID:    proxyID,
		RemoteAddr: remoteAddr,
		Protocol:   protocol,
	})
}

// NewProxyOpenedPacket creates a proxy_opened packet
func NewProxyOpenedPacket(proxyID string, success bool, streamID uint32, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyOpened, ProxyOpenResponse{
		ProxyID:  proxyID,
		Success:  success,
		StreamID: streamID,
		Error:    errMsg,
	})
}

// NewProxyClosePacket creates a proxy_close packet
func NewProxyClosePacket(proxyID string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyClose, ProxyCloseRequest{
		ProxyID: proxyID,
	})
}

// NewProxyClosedPacket creates a proxy_closed packet
func NewProxyClosedPacket(proxyID string, success bool, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyClosed, ProxyClosedResponse{
		ProxyID: proxyID,
		Success: success,
		Error:   errMsg,
	})
}

// NewProxyListPacket creates a proxy_list packet
func NewProxyListPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyList, ProxyListRequest{})
}

// NewProxyListResponsePacket creates a proxy_list_response packet
func NewProxyListResponsePacket(proxies []ProxyInfo) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeProxyListResp, ProxyListResponse{
		Proxies: proxies,
	})
}

// NewErrorPacket creates an error packet
func NewErrorPacket(code, message string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeError, ErrorPayload{
		Code:    code,
		Message: message,
	})
}

// File protocol convenience functions

// NewFileHelloPacket creates a file_hello packet
func NewFileHelloPacket(version, clientID string, features []string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileHello, FileHelloPayload{
		Version:  version,
		ClientID: clientID,
		Features: features,
	})
}

// NewFileHelloAckPacket creates a file_hello_ack packet
func NewFileHelloAckPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileHelloAck, nil)
}

// NewFileListPacket creates a file_list packet
func NewFileListPacket(path string, recursive bool) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileList, FileListRequest{
		Path:      path,
		Recursive: recursive,
	})
}

// NewFileListResponsePacket creates a file_list_response packet
func NewFileListResponsePacket(success bool, files []FileEntry, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileListResponse, FileListResponse{
		Success: success,
		Files:   files,
		Error:   errMsg,
	})
}

// NewFileGetPacket creates a file_get packet
func NewFileGetPacket(fileName string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileGet, FileGetRequest{
		FileName: fileName,
	})
}

// NewFileGetResponsePacket creates a file_get_response packet
func NewFileGetResponsePacket(success bool, streamID uint32, fileInfo *FileInfo, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileGetResponse, FileGetResponse{
		Success:  success,
		StreamID: streamID,
		FileInfo: fileInfo,
		Error:    errMsg,
	})
}

// NewFileSendPacket creates a file_send packet
func NewFileSendPacket(fileName string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileSend, FileSendRequest{
		FileName: fileName,
	})
}

// NewFileSendResponsePacket creates a file_send_response packet
func NewFileSendResponsePacket(success bool, streamID uint32, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileSendResponse, FileSendResponse{
		Success:  success,
		StreamID: streamID,
		Error:    errMsg,
	})
}

// NewFileErrorPacket creates a file_error packet
func NewFileErrorPacket(code, message string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeFileError, ErrorPayload{
		Code:    code,
		Message: message,
	})
}

// Compute protocol packet creation functions

// NewComputeHelloPacket creates a compute_hello packet
func NewComputeHelloPacket(version, clientID string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeHello, ComputeHelloPayload{
		Version:  version,
		ClientID: clientID,
	})
}

// NewComputeHelloAckPacket creates a compute_hello_ack packet
func NewComputeHelloAckPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeHelloAck, ComputeHelloAckPayload{})
}

// NewComputeRunPacket creates a compute_run packet
func NewComputeRunPacket(req ComputeRunRequest) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeRun, req)
}

// NewComputeRunResponsePacket creates a compute_run_response packet
func NewComputeRunResponsePacket(runID string, accepted bool, stdinID, stdoutID, stderrID, logID uint32, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeRunResponse, ComputeRunResponse{
		RunID:    runID,
		Accepted: accepted,
		StdinID:  stdinID,
		StdoutID: stdoutID,
		StderrID: stderrID,
		LogID:    logID,
		Error:    errMsg,
	})
}

// NewComputeCancelPacket creates a compute_cancel packet
func NewComputeCancelPacket(req ComputeCancelRequest) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeCancel, req)
}

// NewComputeCancelResponsePacket creates a compute_cancel_response packet
func NewComputeCancelResponsePacket(runID string, success bool, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeCancelResponse, ComputeCancelResponse{
		RunID:   runID,
		Success: success,
		Error:   errMsg,
	})
}

// NewComputeListEnvsPacket creates a compute_list_envs packet
func NewComputeListEnvsPacket() (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeListEnvs, ComputeListEnvsRequest{})
}

// NewComputeListEnvsResponsePacket creates a compute_list_envs_resp packet
func NewComputeListEnvsResponsePacket(envs map[string]string, errMsg string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeListEnvsResponse, ComputeListEnvsResponse{
		Envs:  envs,
		Error: errMsg,
	})
}

// NewComputeStatusPacket creates a compute_status packet
func NewComputeStatusPacket(req ComputeStatusRequest) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeStatus, req)
}

// NewComputeStatusResponsePacket creates a compute_status_response packet
func NewComputeStatusResponsePacket(resp ComputeStatusResponse) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeStatusResponse, resp)
}

// NewComputeErrorPacket creates a compute_error packet
func NewComputeErrorPacket(code, detail string) (*Packet, error) {
	return NewHandshakePacket(HandshakeTypeComputeError, ComputeErrorPayload{
		Code:   code,
		Detail: detail,
	})
}
