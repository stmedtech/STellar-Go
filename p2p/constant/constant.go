package constant

import (
	"github.com/libp2p/go-libp2p/core/protocol"
)

const StellarRendezvous string = "/stellar/rendezvous/1.0.0"

const StellarEchoProtocol protocol.ID = "/stellar-echo/1.0.0"

const (
	StellarEchoUnknownCommand string = "unkown"
	StellarPing               string = "ping"
	StellarPong               string = "pong"
	StellarEchoDeviceInfo     string = "deviceInfo"
)

const StellarProxyProtocol protocol.ID = "/stellar-proxy/1.1.0"

const StellarFileProtocol protocol.ID = "/stellar-file/1.1.0"

const (
	StellarFileUnknownCommand string = "unkown"
	StellarFileList           string = "ls"
	StellarFileGet            string = "get"
	StellarFileSend           string = "send"
)

const StellarComputeProtocol protocol.ID = "/stellar-compute/1.0.1"

const (
	StellarComputeUnknownCommand      string = "unkown"
	StellarComputePrepareCondaPython  string = "prepCondaPython"
	StellarComputeListCondaPythonEnvs string = "listCondaPythonEnvs"
	StellarComputeExecuteScript       string = "executeScript"
	StellarComputeExecuteWorkspace    string = "executeWorkspace"
)
