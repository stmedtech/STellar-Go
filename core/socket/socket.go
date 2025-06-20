package socket

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"stellar/core/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/file"
	"strconv"

	golog "github.com/ipfs/go-log/v2"

	"github.com/gin-gonic/gin"
)

var logger = golog.Logger("stellar-core-socket")

type APIServer struct {
	Node   *node.Node
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
	s.server.Run(fmt.Sprintf(":%d", port))
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

func (s *APIServer) Start() {
	server := gin.Default()
	server.GET("/devices", s.GetDevices)
	server.GET("/devices/:deviceId", s.GetDevice)
	server.GET("/devices/:deviceId/tree", s.GetDeviceTree)

	server.GET("/policy", s.GetPolicy)
	server.POST("/policy", s.SetPolicy)
	server.GET("/policy/whitelist", s.GetPolicyWhiteList)
	server.POST("/policy/whitelist", s.AddPolicyWhiteList)
	server.DELETE("/policy/whitelist", s.RemovePolicyWhiteList)

	s.server = server
}
