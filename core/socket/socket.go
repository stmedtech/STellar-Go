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

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	files, lsErr := file.List(s.Node, device.ID, path)
	if lsErr != nil {
		logger.Warn(lsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": lsErr.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

func (s *APIServer) DownloadFile(c *gin.Context) {
	deviceId := c.Param("deviceId")
	remotePath := c.Query("remote_path")
	destPath := c.Query("dest_path")

	if remotePath == "" || destPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "remote_path and dest_path are required"})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	filePath, downloadErr := file.Download(s.Node, device.ID, remotePath, destPath)
	if downloadErr != nil {
		logger.Warn(downloadErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": downloadErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"file_path":   filePath,
		"remote_path": remotePath,
	})
}

func (s *APIServer) UploadFile(c *gin.Context) {
	deviceId := c.Param("deviceId")
	localPath := c.PostForm("local_path")
	remotePath := c.PostForm("remote_path")

	if localPath == "" || remotePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local_path and remote_path are required"})
		return
	}

	device, err := s.Node.GetDevice(deviceId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	uploadErr := file.Upload(s.Node, device.ID, localPath, remotePath)
	if uploadErr != nil {
		logger.Warn(uploadErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": uploadErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"local_path":  localPath,
		"remote_path": remotePath,
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
		"success":  true,
		"env_path": envPath,
		"env_name": req.Env,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": execErr.Error()})
		return
	}

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

	// Find the device's proxy manager (assuming it's available through the device)
	if s.Node.Devices == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No proxy manager available"})
		return
	}

	destAddr := fmt.Sprintf("%s:%d", req.RemoteHost, req.RemotePort)

	proxy, err := s.Proxy.Proxy(device.ID, req.LocalPort, destAddr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Can not create proxy"})
		return
	}

	// Note: This would need to be implemented properly with proxy manager access
	// For now, return a placeholder response
	c.JSON(http.StatusOK, proxy)
}

func (s *APIServer) ListProxies(c *gin.Context) {
	// Note: This would need proper proxy manager integration
	// For now, return empty list
	c.JSON(http.StatusOK, s.Proxy.Proxies())
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
