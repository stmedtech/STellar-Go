package policy

import (
	"fmt"

	"stellar/core/config"

	golog "github.com/ipfs/go-log/v2"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var logger = golog.Logger("stellar-p2p-policy")

type ProtocolPolicy struct {
	Enable    bool
	WhiteList []string
}

// LoadFromConfig loads the whitelist directly from the config singleton
// This should be called after the policy is initialized to ensure it has the latest whitelist from config
func (p *ProtocolPolicy) LoadFromConfig() {
	cfg := config.GetInstance()
	p.WhiteList = make([]string, len(cfg.WhiteList))
	copy(p.WhiteList, cfg.WhiteList)
}

func (p *ProtocolPolicy) Authenticate(peer peer.ID) bool {
	if p.Enable {
		for _, allow := range p.WhiteList {
			if peer.String() == allow {
				return true
			}
		}

		return false
	} else {
		return true
	}
}

func (p *ProtocolPolicy) AuthorizeStream(next func(s network.Stream)) func(s network.Stream) {
	callback := func(s network.Stream) {
		peer := s.Conn().RemotePeer()
		if !p.Authenticate(peer) {
			logger.Debugf("reject unauthorized peer %v", peer)
			s.ResetWithError(403)
			return
		}

		next(s)
	}

	return callback
}

func (p *ProtocolPolicy) AddWhiteList(deviceId string) (err error) {
	m, err := ma.NewMultiaddr("/p2p/" + deviceId)
	if err != nil {
		return
	}

	_, err = peer.IDFromP2PAddr(m)
	if err != nil {
		return
	}

	index := -1
	for idx, allow := range p.WhiteList {
		if deviceId == allow {
			index = idx
			break
		}
	}

	if index != -1 {
		err = fmt.Errorf("deviceId already exist in white list")
		return
	}

	p.WhiteList = append(p.WhiteList, deviceId)

	// Auto-sync whitelist to config
	if err := p.syncWhiteListToConfig(); err != nil {
		logger.Warnf("Failed to sync whitelist to config: %v", err)
	}

	return
}

func RemoveIndex(s []string, index int) []string {
	return append(s[:index], s[index+1:]...)
}

func (p *ProtocolPolicy) RemoveWhiteList(deviceId string) (err error) {
	index := -1
	for idx, allow := range p.WhiteList {
		if deviceId == allow {
			index = idx
			break
		}
	}

	if index == -1 {
		err = fmt.Errorf("deviceId not found in white list")
		return
	}

	p.WhiteList = RemoveIndex(p.WhiteList, index)

	// Auto-sync whitelist to config
	if err := p.syncWhiteListToConfig(); err != nil {
		logger.Warnf("Failed to sync whitelist to config: %v", err)
	}

	return
}

// syncWhiteListToConfig syncs the whitelist to the config singleton
func (p *ProtocolPolicy) syncWhiteListToConfig() error {
	cfg := config.GetInstance()
	return cfg.SyncWhiteList(p.WhiteList)
}
