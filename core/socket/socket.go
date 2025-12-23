package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"stellar/core/constant"
	p2p_constant "stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/policy"
	"stellar/p2p/protocols/compute"
	compute_service "stellar/p2p/protocols/compute/service"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"
	"strconv"
	"sync"
	"time"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/gin-gonic/gin"
)

var logger = golog.Logger("stellar-core-socket")

type APIServer struct {
	Node   *node.Node
	Proxy  *proxy.ProxyManager
	server *gin.Engine

	// Compute operation tracking
	computeRunsMu sync.RWMutex
	computeRuns   map[string]*ComputeRun
}

// ComputeRun represents a compute operation
type ComputeRun struct {
	ID       string
	DeviceID string
	Command  string
	Args     []string
	Status   string // "running", "completed", "failed", "cancelled"
	Created  time.Time
	Started  *time.Time
	Finished *time.Time
	ExitCode *int
	Error    error
	handle   *compute_service.RawExecutionHandle
	client   *compute_service.Client
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
	defer os.Remove(socketPath)

	logger.Infof("Stellar API server started on %s", socketPath)

	http.Serve(listener, s.server)

	defer listener.Close()
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
	s.Node.Policy.AddWhiteList(deviceId)
}

func (s *APIServer) RemovePolicyWhiteList(c *gin.Context) {
	deviceId := c.PostForm("deviceId")
	if err := s.Node.Policy.RemoveWhiteList(deviceId); err != nil {
		c.AbortWithError(404, err)
	}
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
	server := gin.Default()

	// Initialize compute runs map
	s.computeRunsMu.Lock()
	if s.computeRuns == nil {
		s.computeRuns = make(map[string]*ComputeRun)
	}
	s.computeRunsMu.Unlock()

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

	// Compute protocol endpoints
	server.POST("/devices/:deviceId/compute/run", s.RunCompute)
	server.GET("/devices/:deviceId/compute", s.ListComputeRuns)
	server.GET("/devices/:deviceId/compute/:runId", s.GetComputeRun)
	server.POST("/devices/:deviceId/compute/:runId/cancel", s.CancelComputeRun)
	server.DELETE("/devices/:deviceId/compute/:runId", s.DeleteComputeRun)

	// Streaming endpoints
	server.GET("/devices/:deviceId/compute/:runId/stdout", s.StreamStdout)
	server.GET("/devices/:deviceId/compute/:runId/stderr", s.StreamStderr)
	server.GET("/devices/:deviceId/compute/:runId/logs", s.StreamLogs)
	server.POST("/devices/:deviceId/compute/:runId/stdin", s.SendStdin)

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
	return s.Node.GetDevice(deviceID)
}

// Compute Protocol Endpoints

// RunCompute executes a command on a device
func (s *APIServer) RunCompute(c *gin.Context) {
	deviceID := c.Param("deviceId")

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

	device, err := s.validateDevice(deviceID)
	if err != nil {
		s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Device not found: %s", deviceID), nil)
		return
	}

	// Create compute client using existing p2p compute protocol
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := compute.DialComputeClient(ctx, s.Node, device.ID)
	if err != nil {
		logger.Warnf("Failed to dial compute client: %v", err)
		s.errorResponse(c, http.StatusInternalServerError, "Failed to connect to device", err.Error())
		return
	}
	// Note: Don't defer close - keep client alive for streaming

	// Generate run ID
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	// Execute command using existing compute service
	handle, err := client.Run(ctx, compute_service.RunRequest{
		RunID:      runID,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
	})
	if err != nil {
		client.Close()
		logger.Warnf("Failed to run compute command: %v", err)
		s.errorResponse(c, http.StatusInternalServerError, "Failed to execute command", err.Error())
		return
	}

	// Store run
	now := time.Now()
	run := &ComputeRun{
		ID:       runID,
		DeviceID: deviceID,
		Command:  req.Command,
		Args:     req.Args,
		Status:   "running",
		Created:  now,
		Started:  &now,
		handle:   handle,
		client:   client,
	}

	s.computeRunsMu.Lock()
	if s.computeRuns == nil {
		s.computeRuns = make(map[string]*ComputeRun)
	}
	s.computeRuns[runID] = run
	s.computeRunsMu.Unlock()

	// Monitor completion
	go func() {
		err := <-handle.Done
		exitCode := <-handle.ExitCode

		s.computeRunsMu.Lock()
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
		s.computeRunsMu.Unlock()
	}()

	// Return response
	c.JSON(http.StatusCreated, gin.H{
		"id":      runID,
		"status":  "running",
		"created": run.Created.Format(time.RFC3339),
		"streams": gin.H{
			"stdout": fmt.Sprintf("/devices/%s/compute/%s/stdout", deviceID, runID),
			"stderr": fmt.Sprintf("/devices/%s/compute/%s/stderr", deviceID, runID),
			"logs":   fmt.Sprintf("/devices/%s/compute/%s/logs", deviceID, runID),
		},
	})
}

// ListComputeRuns lists compute operations for a device
func (s *APIServer) ListComputeRuns(c *gin.Context) {
	deviceID := c.Param("deviceId")

	device, err := s.validateDevice(deviceID)
	if err != nil {
		s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Device not found: %s", deviceID), nil)
		return
	}
	_ = device // Use device to ensure it exists

	s.computeRunsMu.RLock()
	runs := make([]gin.H, 0)
	for _, run := range s.computeRuns {
		if run.DeviceID == deviceID {
			runData := gin.H{
				"id":      run.ID,
				"command": run.Command,
				"status":  run.Status,
				"created": run.Created.Format(time.RFC3339),
			}
			if run.ExitCode != nil {
				runData["exit_code"] = *run.ExitCode
			}
			runs = append(runs, runData)
		}
	}
	s.computeRunsMu.RUnlock()

	c.JSON(http.StatusOK, runs)
}

// GetComputeRun gets compute operation details
func (s *APIServer) GetComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	s.computeRunsMu.RLock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.RUnlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

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

	c.JSON(http.StatusOK, runData)
}

// CancelComputeRun cancels a running operation
func (s *APIServer) CancelComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	s.computeRunsMu.Lock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.Unlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	if err := run.handle.Cancel(); err != nil {
		s.errorResponse(c, http.StatusInternalServerError, "Failed to cancel", err.Error())
		return
	}

	run.Status = "cancelled"
	c.JSON(http.StatusOK, gin.H{"id": runID, "status": "cancelled"})
}

// DeleteComputeRun removes a compute operation record
func (s *APIServer) DeleteComputeRun(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	s.computeRunsMu.Lock()
	run, exists := s.computeRuns[runID]
	if exists && run.DeviceID == deviceID {
		delete(s.computeRuns, runID)
		// Clean up resources
		if run.client != nil {
			run.client.Close()
		}
	}
	s.computeRunsMu.Unlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": runID, "status": "removed"})
}

// Streaming Endpoints

// StreamStdout streams stdout from a compute operation
func (s *APIServer) StreamStdout(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	s.computeRunsMu.RLock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.RUnlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	c.Header("Content-Type", "text/plain")
	if follow {
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}
		_, err := io.Copy(c.Writer, run.handle.Stdout)
		if err != nil && err != io.EOF {
			logger.Warnf("Error streaming stdout: %v", err)
		}
		flusher.Flush()
	} else {
		data, err := io.ReadAll(run.handle.Stdout)
		if err != nil && err != io.EOF {
			s.errorResponse(c, http.StatusInternalServerError, "Failed to read stdout", err.Error())
			return
		}
		c.Data(http.StatusOK, "text/plain", data)
	}
}

// StreamStderr streams stderr from a compute operation
func (s *APIServer) StreamStderr(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	s.computeRunsMu.RLock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.RUnlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	c.Header("Content-Type", "text/plain")
	if follow {
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}
		_, err := io.Copy(c.Writer, run.handle.Stderr)
		if err != nil && err != io.EOF {
			logger.Warnf("Error streaming stderr: %v", err)
		}
		flusher.Flush()
	} else {
		data, err := io.ReadAll(run.handle.Stderr)
		if err != nil && err != io.EOF {
			s.errorResponse(c, http.StatusInternalServerError, "Failed to read stderr", err.Error())
			return
		}
		c.Data(http.StatusOK, "text/plain", data)
	}
}

// StreamLogs streams combined logs from a compute operation
func (s *APIServer) StreamLogs(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")
	follow := c.DefaultQuery("follow", "false") == "true"

	s.computeRunsMu.RLock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.RUnlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	c.Header("Content-Type", "application/json")
	if follow {
		c.Header("Transfer-Encoding", "chunked")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			s.errorResponse(c, http.StatusInternalServerError, "Streaming not supported", nil)
			return
		}
		_, err := io.Copy(c.Writer, run.handle.Log)
		if err != nil && err != io.EOF {
			logger.Warnf("Error streaming logs: %v", err)
		}
		flusher.Flush()
	} else {
		data, err := io.ReadAll(run.handle.Log)
		if err != nil && err != io.EOF {
			s.errorResponse(c, http.StatusInternalServerError, "Failed to read logs", err.Error())
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	}
}

// SendStdin sends input to a running compute operation
func (s *APIServer) SendStdin(c *gin.Context) {
	deviceID := c.Param("deviceId")
	runID := c.Param("runId")

	s.computeRunsMu.RLock()
	run, exists := s.computeRuns[runID]
	s.computeRunsMu.RUnlock()

	if !exists || run.DeviceID != deviceID {
		s.errorResponse(c, http.StatusNotFound, "Run not found", nil)
		return
	}

	if run.handle.Stdin == nil {
		s.errorResponse(c, http.StatusBadRequest, "Stdin not available for this run", nil)
		return
	}

	bytesWritten, err := io.Copy(run.handle.Stdin, c.Request.Body)
	if err != nil {
		s.errorResponse(c, http.StatusInternalServerError, "Failed to write to stdin", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"bytes_written": bytesWritten})
}
