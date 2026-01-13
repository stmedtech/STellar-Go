package socket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"stellar/core/constant"
	"stellar/frontend"
	p2p_constant "stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/policy"
	"stellar/p2p/protocols/compute"
	compute_service "stellar/p2p/protocols/compute/service"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/gin-gonic/gin"
)

var logger = golog.Logger("stellar-core-socket")

// ComputeManager manages compute runs with buffered output storage
type ComputeManager struct {
	runsMu sync.RWMutex
	runs   map[string]*ComputeRun
}

// NewComputeManager creates a new compute manager
func NewComputeManager() *ComputeManager {
	return &ComputeManager{
		runs: make(map[string]*ComputeRun),
	}
}

// AddRun adds a compute run to the manager
func (cm *ComputeManager) AddRun(run *ComputeRun) {
	cm.runsMu.Lock()
	defer cm.runsMu.Unlock()
	cm.runs[run.ID] = run
}

// GetRun retrieves a compute run by ID
func (cm *ComputeManager) GetRun(runID string) (*ComputeRun, bool) {
	cm.runsMu.RLock()
	defer cm.runsMu.RUnlock()
	run, exists := cm.runs[runID]
	return run, exists
}

// RemoveRun removes a compute run and cleans up its resources
func (cm *ComputeManager) RemoveRun(runID string) bool {
	cm.runsMu.Lock()
	defer cm.runsMu.Unlock()
	run, exists := cm.runs[runID]
	if !exists {
		return false
	}

	// Cleanup resources
	run.Cleanup()
	delete(cm.runs, runID)
	return true
}

// ListRuns returns all runs for a given device
func (cm *ComputeManager) ListRuns(deviceID string) []*ComputeRun {
	cm.runsMu.RLock()
	defer cm.runsMu.RUnlock()

	var result []*ComputeRun
	for _, run := range cm.runs {
		if run.DeviceID == deviceID {
			result = append(result, run)
		}
	}
	return result
}

// ComputeRun represents a compute operation with buffered output
type ComputeRun struct {
	ID         string
	DeviceID   string
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
	Status     string // "running", "completed", "failed", "cancelled"
	Created    time.Time
	Started    *time.Time
	Finished   *time.Time
	ExitCode   *int
	Error      error

	// Execution handle (unified interface for both remote and local)
	execHandle ExecutionHandle
	client     *compute_service.Client // Only for remote execution cleanup

	// Buffered output (since original streams are consumed once read)
	stdoutBuf *bytes.Buffer
	stderrBuf *bytes.Buffer
	logsBuf   *bytes.Buffer

	// Synchronization for buffer access
	bufMu sync.RWMutex

	// Channel to signal when buffers are ready for reading
	bufReady chan struct{}

	// Track if streams are being actively read
	streamingMu sync.Mutex
	streaming   bool
}

// ExecutionHandle defines a unified interface for both remote and local command execution
// This abstraction allows ComputeRun to work with both execution types seamlessly
type ExecutionHandle interface {
	// Streams
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Log() io.ReadCloser // Merged log stream (stdout + stderr with timestamps)

	// Control
	Done() <-chan error
	ExitCode() <-chan int
	Cancel() error

	// Metadata
	RunID() string
}

// RemoteExecutionAdapter adapts RawExecutionHandle to ExecutionHandle interface
type RemoteExecutionAdapter struct {
	handle *compute_service.RawExecutionHandle
}

func (a *RemoteExecutionAdapter) Stdin() io.WriteCloser { return a.handle.Stdin }
func (a *RemoteExecutionAdapter) Stdout() io.ReadCloser { return a.handle.Stdout }
func (a *RemoteExecutionAdapter) Stderr() io.ReadCloser { return a.handle.Stderr }
func (a *RemoteExecutionAdapter) Log() io.ReadCloser    { return a.handle.Log }
func (a *RemoteExecutionAdapter) Done() <-chan error    { return a.handle.Done }
func (a *RemoteExecutionAdapter) ExitCode() <-chan int  { return a.handle.ExitCode }
func (a *RemoteExecutionAdapter) Cancel() error         { return a.handle.Cancel() }
func (a *RemoteExecutionAdapter) RunID() string         { return a.handle.RunID }

// LocalExecutionAdapter adapts RawExecution to ExecutionHandle interface
type LocalExecutionAdapter struct {
	execution *compute_service.RawExecution
}

func (a *LocalExecutionAdapter) Stdin() io.WriteCloser { return a.execution.Stdin }
func (a *LocalExecutionAdapter) Stdout() io.ReadCloser { return a.execution.Stdout }
func (a *LocalExecutionAdapter) Stderr() io.ReadCloser { return a.execution.Stderr }
func (a *LocalExecutionAdapter) Log() io.ReadCloser    { return a.execution.Log }
func (a *LocalExecutionAdapter) Done() <-chan error    { return a.execution.Done }
func (a *LocalExecutionAdapter) ExitCode() <-chan int  { return a.execution.ExitCode }
func (a *LocalExecutionAdapter) Cancel() error         { a.execution.Cancel(); return nil }
func (a *LocalExecutionAdapter) RunID() string         { return a.execution.RunID }

// StartBuffering starts reading from streams and buffering output
func (r *ComputeRun) StartBuffering() {
	if r.execHandle == nil {
		return
	}

	r.bufMu.Lock()
	r.stdoutBuf = &bytes.Buffer{}
	r.stderrBuf = &bytes.Buffer{}
	r.logsBuf = &bytes.Buffer{}
	r.bufReady = make(chan struct{}, 1)
	r.bufMu.Unlock()

	// Start reading streams in parallel
	go r.bufferStream(r.execHandle.Stdout(), r.stdoutBuf)
	go r.bufferStream(r.execHandle.Stderr(), r.stderrBuf)
	if logStream := r.execHandle.Log(); logStream != nil {
		go r.bufferStream(logStream, r.logsBuf)
	}

	// Signal that buffers are ready
	close(r.bufReady)
}

// bufferStream reads from a stream and writes to buffer
func (r *ComputeRun) bufferStream(stream io.ReadCloser, buf *bytes.Buffer) {
	if stream == nil {
		return
	}

	b := make([]byte, 4096)
	for {
		n, err := stream.Read(b)
		if n > 0 {
			r.bufMu.Lock()
			buf.Write(b[:n])
			r.bufMu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				logger.Warnf("Error reading stream for run %s: %v", r.ID, err)
			}
			return
		}
	}
}

// GetStdout returns the buffered stdout content
func (r *ComputeRun) GetStdout() []byte {
	r.bufMu.RLock()
	defer r.bufMu.RUnlock()
	if r.stdoutBuf == nil {
		return []byte{}
	}
	return r.stdoutBuf.Bytes()
}

// GetStderr returns the buffered stderr content
func (r *ComputeRun) GetStderr() []byte {
	r.bufMu.RLock()
	defer r.bufMu.RUnlock()
	if r.stderrBuf == nil {
		return []byte{}
	}
	return r.stderrBuf.Bytes()
}

// GetLogs returns the buffered logs content
// For local execution, returns merged stdout/stderr with timestamps
// For remote execution, returns the log stream content
func (r *ComputeRun) GetLogs() []byte {
	r.bufMu.RLock()
	defer r.bufMu.RUnlock()
	if r.logsBuf == nil {
		return []byte{}
	}
	return r.logsBuf.Bytes()
}

// StreamStdoutTo streams stdout to the provided writer, optionally following new output
func (r *ComputeRun) StreamStdoutTo(w io.Writer, follow bool) error {
	// Wait for buffers to be ready
	<-r.bufReady

	if !follow {
		// Return existing buffer content
		r.bufMu.RLock()
		if r.stdoutBuf != nil {
			_, err := w.Write(r.stdoutBuf.Bytes())
			r.bufMu.RUnlock()
			return err
		}
		r.bufMu.RUnlock()
		return nil
	}

	// For follow mode, we need to read from the original stream
	// Since it's already being buffered, we'll read from buffer and then continue
	r.streamingMu.Lock()
	r.streaming = true
	r.streamingMu.Unlock()

	// Write existing buffer
	r.bufMu.RLock()
	if r.stdoutBuf != nil && r.stdoutBuf.Len() > 0 {
		w.Write(r.stdoutBuf.Bytes())
	}
	r.bufMu.RUnlock()

	// For follow mode with already-buffered streams, we return what we have
	// In a real implementation, you might want to tee the stream
	return nil
}

// StreamStderrTo streams stderr to the provided writer, optionally following new output
func (r *ComputeRun) StreamStderrTo(w io.Writer, follow bool) error {
	<-r.bufReady

	if !follow {
		r.bufMu.RLock()
		if r.stderrBuf != nil {
			_, err := w.Write(r.stderrBuf.Bytes())
			r.bufMu.RUnlock()
			return err
		}
		r.bufMu.RUnlock()
		return nil
	}

	r.streamingMu.Lock()
	r.streaming = true
	r.streamingMu.Unlock()

	r.bufMu.RLock()
	if r.stderrBuf != nil && r.stderrBuf.Len() > 0 {
		w.Write(r.stderrBuf.Bytes())
	}
	r.bufMu.RUnlock()

	return nil
}

// Cleanup cleans up resources associated with the run
func (r *ComputeRun) Cleanup() {
	// Close client connection for remote execution
	if r.client != nil {
		r.client.Close()
		r.client = nil
	}

	// Close execution handle streams using interface
	if r.execHandle != nil {
		if stdin := r.execHandle.Stdin(); stdin != nil {
			stdin.Close()
		}
		if stdout := r.execHandle.Stdout(); stdout != nil {
			stdout.Close()
		}
		if stderr := r.execHandle.Stderr(); stderr != nil {
			stderr.Close()
		}
		r.execHandle = nil
	}

	r.bufMu.Lock()
	r.stdoutBuf = nil
	r.stderrBuf = nil
	r.logsBuf = nil
	r.bufMu.Unlock()
}

type APIServer struct {
	Node   *node.Node
	Proxy  *proxy.ProxyManager
	server *gin.Engine

	// Compute manager
	computeManager *ComputeManager
}

func (s *APIServer) StartSocket() {
	s.Start()

	socketPath := filepath.Join(constant.STELLAR_PATH, "stellar.sock")
	// Remove any existing socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Warnf("Error while listening", err)
		return
	}

	// Ensure socket file is always cleaned up, no matter what happens
	// This includes normal exit, panic, crash, or signal termination
	cleanupSocket := func() {
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			logger.Warnf("Failed to remove socket file %s: %v", socketPath, err)
		}
	}

	// Defer cleanup - will run on normal return
	defer cleanupSocket()

	// Handle panics to ensure cleanup even on panic
	defer func() {
		if r := recover(); r != nil {
			cleanupSocket()
			if listener != nil {
				listener.Close()
			}
			panic(r) // Re-panic after cleanup
		}
	}()

	// Handle termination signals to ensure graceful shutdown and cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		logger.Infof("Received termination signal, shutting down socket server...")
		// Clean up socket file explicitly (os.Exit bypasses defers)
		cleanupSocket()
		// Close listener to cause http.Serve to return
		if listener != nil {
			listener.Close()
		}
		// Exit the program
		os.Exit(0)
	}()

	logger.Infof("Stellar API server started on %s", socketPath)

	// Serve will block until listener is closed
	// When listener closes (via signal or error), defer will clean up socket file
	http.Serve(listener, s.server)
}

func (s *APIServer) StartServer(port uint64) {
	s.Start()
	s.server.Run(fmt.Sprintf("0.0.0.0:%d", port))
}

func (s *APIServer) GetDevices(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, s.Node.Devices())
}

func (s *APIServer) GetDevice(c *gin.Context) {
	deviceId := c.Param("deviceId")
	if device, err := s.Node.GetDevice(deviceId); err != nil {
		logger.Warn(err)
		c.JSON(http.StatusInternalServerError, err)
	} else {
		c.IndentedJSON(http.StatusOK, device)
	}
}

func (s *APIServer) GetDeviceTree(c *gin.Context) {
	deviceId := c.Param("deviceId")
	if device, err := s.Node.GetDevice(deviceId); err != nil {
		logger.Warn(err)
		c.JSON(http.StatusInternalServerError, err)
	} else {
		files, lsErr := file.ListFullTree(s.Node, device.ID)
		if lsErr != nil {
			return
		}

		c.IndentedJSON(http.StatusOK, files)
	}
}

func (s *APIServer) GetPolicy(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, s.Node.Policy)
}

func (s *APIServer) SetPolicy(c *gin.Context) {
	enableStr := c.DefaultPostForm("enable", "true")
	enable, err := strconv.ParseBool(enableStr)
	if err != nil {
		c.AbortWithError(406, err)
		return
	}
	s.Node.Policy.Enable = enable
}

func (s *APIServer) GetPolicyWhiteList(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, s.Node.Policy.WhiteList)
}

func (s *APIServer) AddPolicyWhiteList(c *gin.Context) {
	deviceId := c.PostForm("deviceId")
	if err := s.Node.Policy.AddWhiteList(deviceId); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *APIServer) RemovePolicyWhiteList(c *gin.Context) {
	// For DELETE requests, prefer query parameter (more standard)
	// But also support form data in body for compatibility
	deviceId := c.Query("deviceId")
	if deviceId == "" {
		// Try reading from body if Content-Type is form-urlencoded
		contentType := c.GetHeader("Content-Type")
		if contentType == "application/x-www-form-urlencoded" {
			body, err := io.ReadAll(c.Request.Body)
			if err == nil {
				values, err := url.ParseQuery(string(body))
				if err == nil {
					deviceId = values.Get("deviceId")
				}
				c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
			}
		}
		// Fallback to PostForm for POST requests
		if deviceId == "" {
			deviceId = c.PostForm("deviceId")
		}
	}
	if deviceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deviceId is required"})
		return
	}
	if err := s.Node.Policy.RemoveWhiteList(deviceId); err != nil {
		// Return 404 if device not found, but also log the error for debugging
		logger.Warnf("Failed to remove device from whitelist: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Health check endpoint
func (s *APIServer) GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// Node info endpoint
func (s *APIServer) GetNodeInfo(c *gin.Context) {
	type NodeInfoResponse struct {
		NodeID         string
		Addresses      []multiaddr.Multiaddr
		Bootstrapper   bool
		RelayNode      bool
		ReferenceToken string
		DevicesCount   int
		Policy         *policy.ProtocolPolicy
	}

	c.JSON(http.StatusOK, NodeInfoResponse{
		NodeID:         s.Node.ID().String(),
		Addresses:      s.Node.Host.Addrs(),
		Bootstrapper:   s.Node.Bootstrapper,
		RelayNode:      s.Node.RelayNode,
		ReferenceToken: s.Node.ReferenceToken,
		DevicesCount:   len(s.Node.Devices()),
		Policy:         s.Node.Policy,
	})
}

// Connect to peer endpoint
func (s *APIServer) ConnectToPeer(c *gin.Context) {
	type ConnectRequest struct {
		PeerInfo string `json:"peer_info" binding:"required"`
	}

	var req ConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	peerInfo, err := peer.AddrInfoFromString(req.PeerInfo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid peer info format"})
		return
	}

	device, connectErr := s.Node.ConnectDevice(*peerInfo)
	if connectErr != nil {
		logger.Warn(connectErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": connectErr.Error()})
		return
	}

	c.JSON(http.StatusOK, device)
}

// Echo Protocol Endpoints
func (s *APIServer) PingDevice(c *gin.Context) {
	deviceId := c.Param("deviceId")
	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	pingErr := s.Node.Ping(device.ID)

	if pingErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": pingErr.Error()})
		return
	}

	c.JSON(http.StatusOK, device)
}

func (s *APIServer) GetDeviceInfo(c *gin.Context) {
	deviceId := c.Param("deviceId")
	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	deviceInfoStr, echoErr := s.Node.GetEcho(device.ID, p2p_constant.StellarEchoDeviceInfo)
	if echoErr != nil {
		logger.Warn(echoErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": echoErr.Error()})
		return
	}

	decodeErr := json.Unmarshal([]byte(deviceInfoStr), &device)
	if decodeErr != nil {
		logger.Warn(decodeErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": decodeErr.Error()})
		return
	}

	c.JSON(http.StatusOK, device)
}

// File Protocol Endpoints
func (s *APIServer) ListFiles(c *gin.Context) {
	deviceId := c.Param("deviceId")
	path := c.DefaultQuery("path", "/")

	if deviceId == "" {
		logger.Warn("ListFiles called with empty device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device ID is required"})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		logger.Warnf("Device not found: %s, error: %v", deviceId, err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceId)})
		return
	}

	logger.Infof("Listing files for device %s at path: %s", deviceId, path)
	files, lsErr := file.List(s.Node, device.ID, path)
	if lsErr != nil {
		logger.Errorf("Failed to list files for device %s at path %s: %v", deviceId, path, lsErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":     fmt.Sprintf("Failed to list files: %v", lsErr),
			"device_id": deviceId,
			"path":      path,
		})
		return
	}

	logger.Infof("Successfully listed %d files for device %s", len(files), deviceId)
	c.JSON(http.StatusOK, files)
}

func (s *APIServer) DownloadFile(c *gin.Context) {
	deviceId := c.Param("deviceId")
	remotePath := c.Query("remotePath")
	destPath := c.Query("destPath")

	if deviceId == "" {
		logger.Warn("DownloadFile called with empty device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device ID is required"})
		return
	}

	if remotePath == "" || destPath == "" {
		logger.Warnf("DownloadFile called with missing parameters - remotePath: %s, destPath: %s", remotePath, destPath)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "remotePath and destPath are required",
			"remotePath": remotePath,
			"destPath":   destPath,
		})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		logger.Warnf("Device not found for download: %s, error: %v", deviceId, err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceId)})
		return
	}

	// Extract filename from remotePath and pass full destPath
	fileName := filepath.Base(remotePath)

	logger.Infof("Downloading file %s from device %s to %s", fileName, deviceId, destPath)
	filePath, downloadErr := file.Download(s.Node, device.ID, fileName, destPath)
	if downloadErr != nil {
		logger.Errorf("Failed to download file %s from device %s: %v", fileName, deviceId, downloadErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       fmt.Sprintf("Download failed: %v", downloadErr),
			"device_id":   deviceId,
			"remote_path": remotePath,
			"dest_path":   destPath,
		})
		return
	}

	logger.Infof("Successfully downloaded file %s to %s", fileName, filePath)
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"filePath":   filePath,
		"remotePath": remotePath,
		"fileName":   fileName,
	})
}

func (s *APIServer) UploadFile(c *gin.Context) {
	deviceId := c.Param("deviceId")
	localPath := c.PostForm("localPath")
	remotePath := c.PostForm("remotePath")

	if deviceId == "" {
		logger.Warn("UploadFile called with empty device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device ID is required"})
		return
	}

	if localPath == "" || remotePath == "" {
		logger.Warnf("UploadFile called with missing parameters - localPath: %s, remotePath: %s", localPath, remotePath)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "localPath and remotePath are required",
			"localPath":  localPath,
			"remotePath": remotePath,
		})
		return
	}

	// Check if local file exists
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		logger.Warnf("Local file does not exist: %s", localPath)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     fmt.Sprintf("Local file not found: %s", localPath),
			"localPath": localPath,
		})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		logger.Warnf("Device not found for upload: %s, error: %v", deviceId, err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceId)})
		return
	}

	logger.Infof("Uploading file %s to device %s as %s", localPath, deviceId, remotePath)
	uploadErr := file.Upload(s.Node, device.ID, localPath, remotePath)
	if uploadErr != nil {
		logger.Errorf("Failed to upload file %s to device %s: %v", localPath, deviceId, uploadErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      fmt.Sprintf("Upload failed: %v", uploadErr),
			"device_id":  deviceId,
			"localPath":  localPath,
			"remotePath": remotePath,
		})
		return
	}

	logger.Infof("Successfully uploaded file %s to device %s", localPath, deviceId)
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"localPath":  localPath,
		"remotePath": remotePath,
		"device_id":  deviceId,
	})
}

// UploadFileRaw handles raw file upload from browser (multipart/form-data)
func (s *APIServer) UploadFileRaw(c *gin.Context) {
	deviceId := c.Param("deviceId")
	remotePath := c.PostForm("remotePath")

	if deviceId == "" {
		logger.Warn("UploadFileRaw called with empty device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device ID is required"})
		return
	}

	if remotePath == "" {
		logger.Warn("UploadFileRaw called with empty remote path")
		c.JSON(http.StatusBadRequest, gin.H{"error": "remotePath is required"})
		return
	}

	// Get the uploaded file
	fileHeader, err := c.FormFile("file")
	if err != nil {
		logger.Warnf("UploadFileRaw: failed to get file from form: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("File is required: %v", err)})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		logger.Warnf("Device not found for upload: %s, error: %v", deviceId, err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceId)})
		return
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "stellar-upload-*")
	if err != nil {
		logger.Errorf("Failed to create temp file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temporary file"})
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		if err := os.Remove(tmpPath); err != nil {
			logger.Warnf("Failed to remove temp file %s: %v", tmpPath, err)
		}
	}()

	// Save uploaded file to temp location
	uploadedFile, err := fileHeader.Open()
	if err != nil {
		logger.Errorf("Failed to open uploaded file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process uploaded file"})
		return
	}
	defer uploadedFile.Close()

	if _, err := io.Copy(tmpFile, uploadedFile); err != nil {
		logger.Errorf("Failed to copy uploaded file to temp: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}
	tmpFile.Close()

	logger.Infof("Uploading file %s (size: %d) to device %s as %s", fileHeader.Filename, fileHeader.Size, deviceId, remotePath)
	uploadErr := file.Upload(s.Node, device.ID, tmpPath, remotePath)
	if uploadErr != nil {
		logger.Errorf("Failed to upload file to device %s: %v", deviceId, uploadErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      fmt.Sprintf("Upload failed: %v", uploadErr),
			"device_id":  deviceId,
			"remotePath": remotePath,
		})
		return
	}

	logger.Infof("Successfully uploaded file %s to device %s", fileHeader.Filename, deviceId)
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"filename":   fileHeader.Filename,
		"size":       fileHeader.Size,
		"remotePath": remotePath,
		"device_id":  deviceId,
	})
}

// DownloadFileRaw handles raw file download from device to browser
func (s *APIServer) DownloadFileRaw(c *gin.Context) {
	deviceId := c.Param("deviceId")
	remotePath := c.Query("remotePath")

	if deviceId == "" {
		logger.Warn("DownloadFileRaw called with empty device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device ID is required"})
		return
	}

	if remotePath == "" {
		logger.Warn("DownloadFileRaw called with empty remote path")
		c.JSON(http.StatusBadRequest, gin.H{"error": "remotePath is required"})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		logger.Warnf("Device not found for download: %s, error: %v", deviceId, err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceId)})
		return
	}

	// Convert remotePath to relative path (remove leading slash if present)
	// The file.Download expects a relative path from the dataDir root
	relativePath := strings.TrimPrefix(remotePath, "/")
	if relativePath == "" {
		relativePath = remotePath
	}

	// Extract filename for Content-Disposition header
	fileName := filepath.Base(remotePath)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "download"
	}

	// Create temporary file for download
	tmpFile, err := os.CreateTemp("", "stellar-download-*")
	if err != nil {
		logger.Errorf("Failed to create temp file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temporary file"})
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			logger.Warnf("Failed to remove temp file %s: %v", tmpPath, err)
		}
	}()

	logger.Infof("Downloading file %s (path: %s) from device %s", fileName, relativePath, deviceId)
	filePath, downloadErr := file.Download(s.Node, device.ID, relativePath, tmpPath)
	if downloadErr != nil {
		logger.Errorf("Failed to download file %s from device %s: %v", fileName, deviceId, downloadErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       fmt.Sprintf("Download failed: %v", downloadErr),
			"device_id":   deviceId,
			"remote_path": remotePath,
		})
		return
	}

	// Open the downloaded file and stream it to client
	downloadedFile, err := os.Open(filePath)
	if err != nil {
		logger.Errorf("Failed to open downloaded file %s: %v", filePath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read downloaded file"})
		return
	}
	defer downloadedFile.Close()

	fileInfo, err := downloadedFile.Stat()
	if err != nil {
		logger.Errorf("Failed to stat downloaded file %s: %v", filePath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get file information"})
		return
	}

	// Set headers for file download
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Use Gin's DataFromReader for proper streaming
	// This ensures the file is streamed correctly in both Docker and native environments
	c.DataFromReader(http.StatusOK, fileInfo.Size(), "application/octet-stream", downloadedFile, map[string]string{
		"Content-Disposition": fmt.Sprintf("attachment; filename=%q", fileName),
	})

	logger.Infof("Successfully downloaded and streamed file %s from device %s", fileName, deviceId)
}

// Proxy Protocol Endpoints
func (s *APIServer) CreateProxy(c *gin.Context) {
	type ProxyRequest struct {
		DeviceID   string `json:"device_id" binding:"required"`
		LocalPort  uint64 `json:"local_port" binding:"required"`
		RemoteHost string `json:"remote_host" binding:"required"`
		RemotePort uint64 `json:"remote_port" binding:"required"`
	}

	var req ProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := s.Node.GetDevice(req.DeviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	destAddr := fmt.Sprintf("%s:%d", req.RemoteHost, req.RemotePort)

	proxy, err := s.Proxy.Proxy(device.ID, req.LocalPort, destAddr)
	if err != nil {
		logger.Warnf("Failed to create proxy: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Cannot create proxy: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"local_port":  proxy.Port,
		"remote_addr": destAddr,
		"device_id":   req.DeviceID,
	})
}

func (s *APIServer) ListProxies(c *gin.Context) {
	proxies := s.Proxy.Proxies()

	// Convert to API response format
	proxyList := make([]gin.H, 0, len(proxies))
	for _, p := range proxies {
		proxyList = append(proxyList, gin.H{
			"local_port":  p.Port,
			"remote_addr": p.DestAddr,
			"device_id":   p.Dest.String(),
		})
	}

	c.JSON(http.StatusOK, proxyList)
}

func (s *APIServer) CloseProxy(c *gin.Context) {
	portStr := c.Param("port")
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid port number"})
		return
	}

	s.Proxy.Close(port)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"port":    port,
	})
}

func (s *APIServer) Start() {
	// Configure GIN to use os.Stderr (which is redirected to both terminal and file in main.go)
	// This ensures all GIN logs (including [GIN-debug] and [ERROR]) are captured in the log file
	// IMPORTANT: Set these BEFORE calling gin.Default() to ensure GIN uses the redirected stderr
	gin.DefaultWriter = os.Stderr
	gin.DefaultErrorWriter = os.Stderr

	server := gin.Default()

	// Initialize compute manager
	if s.computeManager == nil {
		s.computeManager = NewComputeManager()
	}

	// Register frontend routes FIRST (before API routes)
	// This ensures the / route is available
	// Fallback HTML content when frontend is not available
	fallbackHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Stellar - Frontend Not Available</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
        }
        .container {
            background: white;
            padding: 2rem;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
            max-width: 500px;
        }
        h1 {
            margin-top: 0;
            color: #667eea;
        }
        p {
            color: #666;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Frontend Module Not Available</h1>
        <p>The frontend module was not built or embedded in this build.</p>
        <p>API endpoints are still available. You can access the API at:</p>
        <ul style="text-align: left; display: inline-block;">
            <li><code>/health</code> - Health check</li>
            <li><code>/node</code> - Node information</li>
            <li><code>/devices</code> - Device management</li>
        </ul>
    </div>
</body>
</html>`

	// Helper function to serve index.html or fallback
	serveIndexOrFallback := func(c *gin.Context, staticFS fs.FS) {
		if staticFS != nil {
			indexFile, err := fs.ReadFile(staticFS, "index.html")
			if err == nil && len(indexFile) > 0 {
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexFile)
				return
			}
			// Fallback: try reading directly
			indexFile, err = frontend.StaticFiles.ReadFile("dist/index.html")
			if err == nil && len(indexFile) > 0 {
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexFile)
				return
			}
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fallbackHTML))
	}

	// Helper function to get content type from file path
	getContentType := func(filePath string) string {
		if strings.HasSuffix(filePath, ".js") {
			return "application/javascript"
		} else if strings.HasSuffix(filePath, ".css") {
			return "text/css"
		} else if strings.HasSuffix(filePath, ".json") {
			return "application/json"
		} else if strings.HasSuffix(filePath, ".html") {
			return "text/html; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".png") {
			return "image/png"
		} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
			return "image/jpeg"
		} else if strings.HasSuffix(filePath, ".svg") {
			return "image/svg+xml"
		} else if strings.HasSuffix(filePath, ".ico") {
			return "image/x-icon"
		}
		return "application/octet-stream"
	}

	// Try to load static files
	var staticFS fs.FS
	staticFS, err := fs.Sub(frontend.StaticFiles, "dist")
	if err == nil {
		// Verify files are actually embedded
		_, testErr := fs.ReadFile(staticFS, "index.html")
		if testErr == nil {
			logger.Infof("Frontend static files embedded successfully")
		} else {
			logger.Warnf("Frontend files embedded but index.html not accessible: %v", testErr)
			staticFS = nil
		}
	} else {
		logger.Warnf("Failed to create static filesystem: %v - frontend will not be available", err)
		staticFS = nil
	}

	// ============================================================================
	// STEP 1: Register ALL API routes FIRST (highest priority)
	// ============================================================================

	// Health and node endpoints
	server.GET("/health", s.GetHealth)
	server.GET("/node", s.GetNodeInfo)
	server.POST("/connect", s.ConnectToPeer)

	// Device management endpoints
	server.GET("/devices", s.GetDevices)
	server.GET("/devices/:deviceId", s.GetDevice)
	server.GET("/devices/:deviceId/tree", s.GetDeviceTree)

	// Echo protocol endpoints
	server.POST("/devices/:deviceId/ping", s.PingDevice)
	server.GET("/devices/:deviceId/info", s.GetDeviceInfo)

	// File protocol endpoints
	server.GET("/devices/:deviceId/files", s.ListFiles)
	server.GET("/devices/:deviceId/files/download", s.DownloadFile)
	server.POST("/devices/:deviceId/files/upload", s.UploadFile)
	// Raw file upload/download endpoints (for browser file uploads)
	server.POST("/devices/:deviceId/files/upload/raw", s.UploadFileRaw)
	server.GET("/devices/:deviceId/files/download/raw", s.DownloadFileRaw)

	// Compute protocol endpoints
	server.POST("/devices/:deviceId/compute/execute", s.ExecuteCommand)
	server.GET("/devices/:deviceId/compute/runs", s.ListComputeRuns)
	server.GET("/devices/:deviceId/compute/runs/:runId", s.GetComputeRun)
	server.POST("/devices/:deviceId/compute/runs/:runId/cancel", s.CancelComputeRun)
	server.DELETE("/devices/:deviceId/compute/runs/:runId", s.DeleteComputeRun)

	// Output streaming endpoints
	server.GET("/devices/:deviceId/compute/runs/:runId/stdout", s.StreamStdout)
	server.GET("/devices/:deviceId/compute/runs/:runId/stderr", s.StreamStderr)
	server.GET("/devices/:deviceId/compute/runs/:runId/logs", s.StreamLogs)

	// Interactive input endpoint (for sending stdin to running commands)
	server.POST("/devices/:deviceId/compute/runs/:runId/stdin", s.SendStdin)

	// Proxy protocol endpoints
	server.POST("/proxy", s.CreateProxy)
	server.GET("/proxy", s.ListProxies)
	server.DELETE("/proxy/:port", s.CloseProxy)

	// Policy management endpoints
	server.GET("/policy", s.GetPolicy)
	server.POST("/policy", s.SetPolicy)
	server.GET("/policy/whitelist", s.GetPolicyWhiteList)
	server.POST("/policy/whitelist", s.AddPolicyWhiteList)
	server.DELETE("/policy/whitelist", s.RemovePolicyWhiteList)

	// ============================================================================
	// STEP 2: Handle root path exactly - serve index.html
	// ============================================================================
	server.GET("/", func(c *gin.Context) {
		serveIndexOrFallback(c, staticFS)
	})

	// ============================================================================
	// STEP 3: Handle all other paths (NoRoute)
	// Priority: Try staticFS first, then index.html for SPA, then 404
	// ============================================================================
	server.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip if it's already handled by API routes (shouldn't happen, but safety check)
		if strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/node") ||
			strings.HasPrefix(path, "/devices") ||
			strings.HasPrefix(path, "/proxy") ||
			strings.HasPrefix(path, "/policy") ||
			strings.HasPrefix(path, "/connect") {
			c.Status(http.StatusNotFound)
			return
		}

		// Try to find file in staticFS (supports assets in root path)
		if staticFS != nil {
			// Remove leading slash for filesystem path
			fsPath := strings.TrimPrefix(path, "/")
			if fsPath == "" {
				fsPath = "index.html"
			}

			fileContent, err := fs.ReadFile(staticFS, fsPath)
			if err == nil && len(fileContent) > 0 {
				// Found file in staticFS, serve it
				contentType := getContentType(fsPath)
				c.Data(http.StatusOK, contentType, fileContent)
				return
			}
		}

		// File not found in staticFS, serve index.html for SPA routing
		// This handles client-side routes like /app/dashboard, /app/devices, etc.
		serveIndexOrFallback(c, staticFS)
	})

	s.server = server
}

// GetRouter returns the gin.Engine router for testing purposes
func (s *APIServer) GetRouter() *gin.Engine {
	return s.server
}

// errorResponse is a DRY helper for error responses
func (s *APIServer) errorResponse(c *gin.Context, statusCode int, message string, details interface{}) {
	response := gin.H{"error": message}
	if details != nil {
		response["details"] = details
	}
	c.JSON(statusCode, response)
}

// validateDevice is a DRY helper for device validation
func (s *APIServer) validateDevice(deviceID string) (node.Device, error) {
	if deviceID == "" {
		return node.Device{}, fmt.Errorf("device ID is required")
	}
	if s.Node == nil {
		return node.Device{}, fmt.Errorf("node is not initialized")
	}

	// Allow host node deviceId (bypass validation for local execution)
	hostNodeID := s.Node.Host.ID().String()
	if deviceID == hostNodeID {
		// Return a dummy device for host node
		peerID, err := peer.Decode(deviceID)
		if err != nil {
			return node.Device{}, fmt.Errorf("invalid peer ID: %w", err)
		}
		return node.Device{ID: peerID, Status: node.DeviceStatusHealthy}, nil
	}

	return s.Node.GetDevice(deviceID)
}

// Compute Protocol Endpoints

// ExecuteCommand executes a shell command on a device (unified for both local and remote)
func (s *APIServer) ExecuteCommand(c *gin.Context) {
	deviceID := c.Param("deviceId")
	hostNodeID := s.Node.Host.ID().String()
	isLocal := deviceID == hostNodeID

	// Validate device (allows host node deviceId for local execution)
	if !isLocal {
		_, err := s.validateDevice(deviceID)
		if err != nil {
			s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Device not found: %s", deviceID), nil)
			return
		}
	}

	var req struct {
		Command    string            `json:"command" binding:"required"`
		Args       []string          `json:"args,omitempty"`
		Env        map[string]string `json:"env,omitempty"`
		WorkingDir string            `json:"working_dir,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// Execute command (unified for local and remote)
	execHandle, client, err := s.executeCommand(deviceID, isLocal, req.Command, req.Args, req.Env, req.WorkingDir)
	if err != nil {
		if client != nil {
			client.Close()
		}
		s.errorResponse(c, http.StatusInternalServerError, "Failed to execute command", err.Error())
		return
	}

	// Generate run ID
	runID := execHandle.RunID()

	// Create and store run using unified interface
	now := time.Now()
	run := &ComputeRun{
		ID:         runID,
		DeviceID:   deviceID,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		Status:     "running",
		Created:    now,
		Started:    &now,
		execHandle: execHandle,
		client:     client, // Only set for remote execution
	}

	// Start buffering output immediately
	run.StartBuffering()

	// Add to manager
	s.computeManager.AddRun(run)

	// Monitor completion (unified using interface)
	s.monitorRunCompletion(run)

	// Return response with updated endpoint structure
	c.JSON(http.StatusCreated, gin.H{
		"id":      runID,
		"status":  "running",
		"command": req.Command,
		"args":    req.Args,
		"created": run.Created.Format(time.RFC3339),
		"started": run.Started.Format(time.RFC3339),
		"endpoints": gin.H{
			"stdout": fmt.Sprintf("/devices/%s/compute/runs/%s/stdout", deviceID, runID),
			"stderr": fmt.Sprintf("/devices/%s/compute/runs/%s/stderr", deviceID, runID),
			"logs":   fmt.Sprintf("/devices/%s/compute/runs/%s/logs", deviceID, runID),
			"stdin":  fmt.Sprintf("/devices/%s/compute/runs/%s/stdin", deviceID, runID),
		},
	})
}

// executeCommand executes a command and returns an ExecutionHandle (unified for local and remote)
// Returns: (execHandle, client, error)
// client is only set for remote execution and should be closed by caller on error
func (s *APIServer) executeCommand(deviceID string, isLocal bool, command string, args []string, env map[string]string, workingDir string) (ExecutionHandle, *compute_service.Client, error) {
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	if isLocal {
		// Execute locally
		preparedEnv := compute_service.PrepareExecutionEnvironment(env)
		executor := compute_service.NewRawExecutor()
		execReq := compute_service.RawExecutionRequest{
			Command:    command,
			Args:       args,
			Env:        preparedEnv,
			WorkingDir: workingDir,
			Stdin:      nil,
		}

		execution, err := executor.ExecuteRaw(context.Background(), execReq)
		if err != nil {
			return nil, nil, err
		}

		execution.RunID = runID

		// Create log stream for merged stdout/stderr (similar to server.go)
		// Use buffer pipes to split streams so both direct access and log merging work
		// CRITICAL: Create pipes and start forwarders immediately after execution is created
		// This ensures readers are ready before the command can produce output
		stdoutBufR, stdoutBufW := io.Pipe()
		stderrBufR, stderrBufW := io.Pipe()
		logR, logW := io.Pipe()

		// Start forwarding goroutines (similar to server.go forwardStdout/forwardStderr)
		// These read from execution streams and write to both buffer pipes and log stream
		// CRITICAL: Start immediately to ensure readers are ready
		var forwardWg sync.WaitGroup
		forwardWg.Add(2)

		go func() {
			defer forwardWg.Done()
			defer stdoutBufW.Close()
			forwardToLogAndBuffer(execution.Stdout, logW, stdoutBufW, runID, "stdout")
		}()
		go func() {
			defer forwardWg.Done()
			defer stderrBufW.Close()
			forwardToLogAndBuffer(execution.Stderr, logW, stderrBufW, runID, "stderr")
		}()

		// Give forwarders a moment to start and be ready to read
		// This ensures they're actively waiting on Read() calls
		time.Sleep(1 * time.Millisecond)

		// Close log writer when both forwarding goroutines complete
		go func() {
			forwardWg.Wait()
			_ = logW.Close()
		}()

		// Create new RawExecution with buffered streams and log stream
		executionWithLog := &compute_service.RawExecution{
			RunID:    execution.RunID,
			Stdin:    execution.Stdin,
			Stdout:   stdoutBufR, // Buffered copy for direct access
			Stderr:   stderrBufR, // Buffered copy for direct access
			Log:      logR,       // Merged log stream
			Done:     execution.Done,
			ExitCode: execution.ExitCode,
			Cancel: func() {
				execution.Cancel()
				_ = stdoutBufW.Close()
				_ = stderrBufW.Close()
				_ = logW.Close()
			},
		}

		return &LocalExecutionAdapter{execution: executionWithLog}, nil, nil
	}

	// Execute remotely
	device, err := s.validateDevice(deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("device not found: %s", deviceID)
	}

	// Create compute client using existing p2p compute protocol
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := compute.DialComputeClient(dialCtx, s.Node, device.ID)
	dialCancel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to device: %w", err)
	}

	// Execute command using background context
	handle, err := client.Run(context.Background(), compute_service.RunRequest{
		RunID:      runID,
		Command:    command,
		Args:       args,
		Env:        env,
		WorkingDir: workingDir,
	})
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("failed to run compute command: %w", err)
	}

	return &RemoteExecutionAdapter{handle: handle}, client, nil
}

// monitorRunCompletion monitors a run's completion and updates status (DRY helper)
func (s *APIServer) monitorRunCompletion(run *ComputeRun) {
	go func() {
		err := <-run.execHandle.Done()
		exitCode := <-run.execHandle.ExitCode()

		run.Finished = new(time.Time)
		*run.Finished = time.Now()
		run.ExitCode = &exitCode
		if err != nil {
			run.Status = "failed"
			run.Error = err
		} else if exitCode == 0 {
			run.Status = "completed"
		} else {
			run.Status = "failed"
		}
	}()
}

// ListComputeRuns lists all compute operations for a device
// Allows access to command history even if device is no longer in active list
func (s *APIServer) ListComputeRuns(c *gin.Context) {
	deviceID := c.Param("deviceId")

	// Check if there are any runs for this device first
	// This allows access to command history even if device is disconnected
	runs := s.computeManager.ListRuns(deviceID)

	// If no runs exist, validate device to provide better error message
	if len(runs) == 0 {
		_, err := s.validateDevice(deviceID)
		if err != nil {
			s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Device not found: %s", deviceID), nil)
			return
		}
	}

	runList := make([]gin.H, 0, len(runs))

	for _, run := range runs {
		runData := gin.H{
			"id":      run.ID,
			"command": run.Command,
			"args":    run.Args,
			"status":  run.Status,
			"created": run.Created.Format(time.RFC3339),
		}
		if run.Started != nil {
			runData["started"] = run.Started.Format(time.RFC3339)
		}
		if run.Finished != nil {
			runData["finished"] = run.Finished.Format(time.RFC3339)
		}
		if run.ExitCode != nil {
			runData["exit_code"] = *run.ExitCode
		}
		if run.Error != nil {
			runData["error"] = run.Error.Error()
		}
		runList = append(runList, runData)
	}

	c.JSON(http.StatusOK, runList)
}

// GetComputeRun gets detailed information about a specific compute run
// Allows access to command history even if device is no longer in active list
func (s *APIServer) GetComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	// Check if run exists first - allows access even if device is disconnected
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// No device validation needed - if run exists, allow access to history

	runData := gin.H{
		"id":          run.ID,
		"device_id":   run.DeviceID,
		"command":     run.Command,
		"args":        run.Args,
		"env":         run.Env,
		"working_dir": run.WorkingDir,
		"status":      run.Status,
		"created":     run.Created.Format(time.RFC3339),
	}
	if run.Started != nil {
		runData["started"] = run.Started.Format(time.RFC3339)
	}
	if run.Finished != nil {
		runData["finished"] = run.Finished.Format(time.RFC3339)
	}
	if run.ExitCode != nil {
		runData["exit_code"] = *run.ExitCode
	}
	if run.Error != nil {
		runData["error"] = run.Error.Error()
	}

	// Include output sizes
	runData["output_sizes"] = gin.H{
		"stdout_bytes": len(run.GetStdout()),
		"stderr_bytes": len(run.GetStderr()),
		"logs_bytes":   len(run.GetLogs()),
	}

	c.JSON(http.StatusOK, runData)
}

// forwardToLogAndBuffer forwards data from source stream to both log stream and buffer pipe
// Similar to server.go's forwardStdout/forwardStderr pattern
// This allows both merged logs (with timestamps) and direct stdout/stderr access
func forwardToLogAndBuffer(source io.ReadCloser, logW io.WriteCloser, bufW io.WriteCloser, runID, logType string) {
	buf := make([]byte, 4096)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			data := buf[:n]
			// Write to buffer pipe for direct access
			if _, bufErr := bufW.Write(data); bufErr != nil {
				// Buffer pipe closed, stop writing to it but continue logging
			}
			// Write to log stream with timestamp
			writeLogEntry(logW, runID, logType, string(data))
		}
		if err != nil {
			if err == io.EOF {
				// Normal end of stream
				return
			}
			// Log error
			writeLogEntry(logW, runID, "error", fmt.Sprintf("%s read error: %v", logType, err))
			return
		}
		// If n == 0 and err == nil, continue reading (shouldn't happen but be safe)
	}
}

// writeLogEntry writes a log entry to the log stream (DRY: same as server.go)
func writeLogEntry(logStream io.WriteCloser, runID, logType, data string) {
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

// CancelComputeRun cancels a running command execution
// Note: Only works if device is still connected and run is still active
func (s *APIServer) CancelComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	// Check if run exists first
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// Note: Cancel requires active execution handle, so device must be connected

	if run.Status != "running" {
		s.errorResponse(c, http.StatusBadRequest, fmt.Sprintf("Run is not running (status: %s)", run.Status), nil)
		return
	}

	if run.execHandle == nil {
		s.errorResponse(c, http.StatusBadRequest, "No execution handle available", nil)
		return
	}

	if err := run.execHandle.Cancel(); err != nil {
		s.errorResponse(c, http.StatusInternalServerError, "Failed to cancel", err.Error())
		return
	}

	run.Status = "cancelled"
	c.JSON(http.StatusOK, gin.H{
		"id":     runID,
		"status": "cancelled",
	})
}

// DeleteComputeRun removes a compute run and cleans up all associated resources (including buffers)
// Allows deletion of command history even if device is no longer in active list
func (s *APIServer) DeleteComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	// Check if run exists first - allows deletion even if device is disconnected
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// No device validation needed - if run exists, allow deletion of history

	// Remove from manager (this also calls Cleanup)
	removed := s.computeManager.RemoveRun(runID)
	if !removed {
		s.errorResponse(c, http.StatusInternalServerError, "Failed to remove run", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":     runID,
		"status": "removed",
	})
}

// Streaming Endpoints

// StreamStdout streams stdout from a compute operation using buffered output
// Allows access to command history even if device is no longer in active list
func (s *APIServer) StreamStdout(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	// Check if run exists first - allows access even if device is disconnected
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// No device validation needed - if run exists, allow access to history

	c.Header("Content-Type", "text/plain; charset=utf-8")

	// Get buffered stdout
	stdoutData := run.GetStdout()

	if follow {
		// For follow mode, write existing buffer and then continue streaming
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}

		// Write existing buffer
		if len(stdoutData) > 0 {
			c.Writer.Write(stdoutData)
			flusher.Flush()
		}

		// Note: Since we're buffering, follow mode will only show what's already buffered
		// For true follow mode with live updates, you'd need to tee the stream
		// For now, we return the buffered content
	} else {
		// Return all buffered content
		c.Data(http.StatusOK, "text/plain; charset=utf-8", stdoutData)
	}
}

// StreamStderr streams stderr from a compute operation using buffered output
// Allows access to command history even if device is no longer in active list
func (s *APIServer) StreamStderr(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	// Check if run exists first - allows access even if device is disconnected
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// No device validation needed - if run exists, allow access to history

	c.Header("Content-Type", "text/plain; charset=utf-8")

	stderrData := run.GetStderr()

	if follow {
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}

		if len(stderrData) > 0 {
			c.Writer.Write(stderrData)
			flusher.Flush()
		}
	} else {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", stderrData)
	}
}

// StreamLogs streams combined logs from a compute operation using buffered output
// Allows access to command history even if device is no longer in active list
func (s *APIServer) StreamLogs(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	// Check if run exists first - allows access even if device is disconnected
	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}
	// No device validation needed - if run exists, allow access to history

	c.Header("Content-Type", "application/json; charset=utf-8")

	logsData := run.GetLogs()

	if follow {
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}

		if len(logsData) > 0 {
			c.Writer.Write(logsData)
			flusher.Flush()
		}
	} else {
		c.Data(http.StatusOK, "application/json; charset=utf-8", logsData)
	}
}

// SendStdin sends input to a running command's stdin
func (s *APIServer) SendStdin(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	run, exists := s.computeManager.GetRun(runID)
	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	if run.Status != "running" {
		s.errorResponse(c, http.StatusBadRequest, fmt.Sprintf("Run is not running (status: %s)", run.Status), nil)
		return
	}

	// Read input from request body
	input, err := io.ReadAll(c.Request.Body)
	if err != nil {
		s.errorResponse(c, http.StatusBadRequest, "Failed to read input", err.Error())
		return
	}

	// Write to stdin using unified interface
	if len(input) > 0 {
		if run.execHandle == nil {
			s.errorResponse(c, http.StatusBadRequest, "No execution handle available", nil)
			return
		}

		stdin := run.execHandle.Stdin()
		if stdin == nil {
			s.errorResponse(c, http.StatusBadRequest, "Stdin not available for this run", nil)
			return
		}

		if _, err := stdin.Write(input); err != nil {
			s.errorResponse(c, http.StatusInternalServerError, "Failed to write to stdin", err.Error())
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"bytes_written": len(input),
		"status":        "sent",
	})
}
