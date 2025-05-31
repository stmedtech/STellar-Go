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

const StellarProxyProtocol protocol.ID = "/stellar-proxy/1.0.0"

const StellarFileProtocol protocol.ID = "/stellar-file/1.0.0"
