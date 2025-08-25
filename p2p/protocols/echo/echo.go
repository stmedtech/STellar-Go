package echo

import (
	"bufio"
	"encoding/json"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/util"
	"strings"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
)

var logger = golog.Logger("stellar-p2p-protocols-echo")

func BindEchoStream(n *node.Node) {
	n.Host.SetStreamHandler(constant.StellarEchoProtocol, func(s network.Stream) {
		if err := doStellarEcho(n, s); err != nil {
			// TODO improve error handling
			logger.Warnf("echo error: %v", err)
			s.ResetWithError(406)
		} else {
			s.Close()
		}
	})
	logger.Info("Echo protocol is ready")
}

func doStellarEcho(n *node.Node, s network.Stream) error {
	buf := bufio.NewReader(s)

	str, err := buf.ReadString('\n')
	if err != nil {
		return err
	}
	str = strings.Trim(str, "\n")

	response := ""
	switch str {
	case constant.StellarPing:
		response = constant.StellarPong
	case constant.StellarEchoDeviceInfo:
		sysInfo, sysInfoErr := util.GetSystemInformation()
		if sysInfoErr != nil {
			return sysInfoErr
		}
		device := node.Device{
			ID:             n.ID(),
			ReferenceToken: n.ReferenceToken,
			SysInfo:        sysInfo,
		}
		jsonData, jsonErr := json.Marshal(device)
		if jsonErr != nil {
			return jsonErr
		}
		response = string(jsonData)
	default:
		response = constant.StellarEchoUnknownCommand
	}

	_, err = s.Write([]byte(response))
	return err
}
