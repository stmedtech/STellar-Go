package socket

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"stellar/core/constant"
	"stellar/core/protocols/compute"
	p2p_constant "stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/policy"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"
	"strconv"

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
}

func (s *APIServer) StartSocket() {
	s.Start()

	socketPath := filepath.Join(constant.STELLAR_PATH, "stellar.sock")
	// Remove any existing socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Fatalln("Error while listening", err)
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
	c.IndentedJSON(http.StatusOK, s.Node.Devices)
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
		DevicesCount:   len(s.Node.Devices),
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

// Compute Protocol Endpoints
func (s *APIServer) ListCondaEnvs(c *gin.Context) {
	deviceId := c.Param("deviceId")

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	envs, envsErr := compute.ListCondaPythonEnvs(s.Node, device.ID)
	if envsErr != nil {
		logger.Warn(envsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": envsErr.Error()})
		return
	}

	c.JSON(http.StatusOK, envs)
}

func (s *APIServer) PrepareCondaEnv(c *gin.Context) {
	deviceId := c.Param("deviceId")

	type PrepareRequest struct {
		Env         string `json:"env" binding:"required"`
		Version     string `json:"version" binding:"required"`
		EnvYamlPath string `json:"env_yaml_path" binding:"required"`
	}

	var req PrepareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	preparation := compute.CondaPythonPreparation{
		Env:         req.Env,
		Version:     req.Version,
		EnvYamlPath: req.EnvYamlPath,
	}

	envPath, prepErr := compute.PrepareCondaPython(s.Node, device.ID, preparation)
	if prepErr != nil {
		logger.Warn(prepErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": prepErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"envPath": envPath,
		"envName": req.Env,
	})
}

func (s *APIServer) ExecuteScript(c *gin.Context) {
	deviceId := c.Param("deviceId")

	type ExecuteRequest struct {
		Env        string `json:"env" binding:"required"`
		ScriptPath string `json:"script_path" binding:"required"`
	}

	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	execution := compute.CondaPythonScriptExecution{
		Env:        req.Env,
		ScriptPath: req.ScriptPath,
	}

	result, execErr := compute.ExecuteCondaPythonScript(s.Node, device.ID, execution)
	if execErr != nil {
		logger.Warn(execErr)
		logger.Warn(result)
		c.JSON(http.StatusInternalServerError, gin.H{"error": execErr.Error()})
		return
	}

	logger.Infof("Successfully executed script %s on device %s", req.ScriptPath, deviceId)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
		"env":     req.Env,
	})
}

func (s *APIServer) ExecuteWorkspace(c *gin.Context) {
	deviceId := c.Param("deviceId")

	type ExecuteRequest struct {
		Env           string `json:"env" binding:"required"`
		WorkspacePath string `json:"workspace_path" binding:"required"`
	}

	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	execution := compute.CondaPythonWorkspaceExecution{
		Env:           req.Env,
		WorkspacePath: req.WorkspacePath,
	}

	result, execErr := compute.ExecuteCondaPythonWorkspace(s.Node, device.ID, execution)
	if execErr != nil {
		logger.Warn(execErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": execErr.Error()})
		return
	}

	logger.Infof("Successfully executed workspace %s on device %s", req.WorkspacePath, deviceId)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
		"env":     req.Env,
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
	server.GET("/devices/:deviceId/compute/envs", s.ListCondaEnvs)
	server.POST("/devices/:deviceId/compute/prepare", s.PrepareCondaEnv)
	server.POST("/devices/:deviceId/compute/execute", s.ExecuteScript)
	server.POST("/devices/:deviceId/compute/execute-workspace", s.ExecuteWorkspace)

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
