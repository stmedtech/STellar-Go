package constant

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
)

func TestStellarRendezvous(t *testing.T) {
	assert.Equal(t, "/stellar/rendezvous/1.0.0", StellarRendezvous)
}

func TestStellarEchoProtocol(t *testing.T) {
	assert.Equal(t, protocol.ID("/stellar-echo/1.0.0"), StellarEchoProtocol)
}

func TestStellarEchoCommands(t *testing.T) {
	assert.Equal(t, "unkown", StellarEchoUnknownCommand)
	assert.Equal(t, "ping", StellarPing)
	assert.Equal(t, "pong", StellarPong)
	assert.Equal(t, "deviceInfo", StellarEchoDeviceInfo)
}

func TestStellarProxyProtocol(t *testing.T) {
	assert.Equal(t, protocol.ID("/stellar-proxy/1.0.0"), StellarProxyProtocol)
}

func TestStellarFileProtocol(t *testing.T) {
	assert.Equal(t, protocol.ID("/stellar-file/1.0.1"), StellarFileProtocol)
}

func TestStellarFileCommands(t *testing.T) {
	assert.Equal(t, "unkown", StellarFileUnknownCommand)
	assert.Equal(t, "ls", StellarFileList)
	assert.Equal(t, "get", StellarFileGet)
	assert.Equal(t, "send", StellarFileSend)
}

func TestStellarComputeProtocol(t *testing.T) {
	assert.Equal(t, protocol.ID("/stellar-compute/1.0.0"), StellarComputeProtocol)
}

func TestStellarComputeCommands(t *testing.T) {
	assert.Equal(t, "unkown", StellarComputeUnknownCommand)
	assert.Equal(t, "prepCondaPython", StellarComputePrepareCondaPython)
	assert.Equal(t, "listCondaPythonEnvs", StellarComputeListCondaPythonEnvs)
	assert.Equal(t, "executeScript", StellarComputeExecuteScript)
}

func TestProtocolIDTypes(t *testing.T) {
	// Verify that protocol constants are of the correct type
	var _ protocol.ID = StellarEchoProtocol
	var _ protocol.ID = StellarProxyProtocol
	var _ protocol.ID = StellarFileProtocol
	var _ protocol.ID = StellarComputeProtocol
}

func TestStringConstants(t *testing.T) {
	// Verify that string constants are not empty
	assert.NotEmpty(t, StellarRendezvous)
	assert.NotEmpty(t, StellarEchoUnknownCommand)
	assert.NotEmpty(t, StellarPing)
	assert.NotEmpty(t, StellarPong)
	assert.NotEmpty(t, StellarEchoDeviceInfo)
	assert.NotEmpty(t, StellarFileUnknownCommand)
	assert.NotEmpty(t, StellarFileList)
	assert.NotEmpty(t, StellarFileGet)
	assert.NotEmpty(t, StellarFileSend)
	assert.NotEmpty(t, StellarComputeUnknownCommand)
	assert.NotEmpty(t, StellarComputePrepareCondaPython)
	assert.NotEmpty(t, StellarComputeListCondaPythonEnvs)
	assert.NotEmpty(t, StellarComputeExecuteScript)
}
