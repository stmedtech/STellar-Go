package bootstrap

import (
	"bufio"
	"math/rand/v2"
	"os"
	"path/filepath"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var logger = golog.Logger("stellar-p2p-bootstrap")

var BOOTSTRAPPERS = []string{
	"/ip4/114.32.226.175/tcp/43210/p2p/12D3KooWJ3VruqtQC4g7wvfy7NPqtdJmrWotzki4b2J7D9tYzY9a",
}

var Bootstrappers []peer.AddrInfo

func getBootstrappers() ([]string, error) {
	bootstrappers := BOOTSTRAPPERS

	pwd, err := os.Getwd()
	if err != nil {
		return bootstrappers, err
	}
	file, err := os.Open(filepath.Join(pwd, "bootstrappers.txt"))
	if err != nil {
		return bootstrappers, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		bootstrappers = append(bootstrappers, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return bootstrappers, err
	}

	return bootstrappers, nil
}

func MakeAddrInfo(peerAddrString string) (peerInfo peer.AddrInfo, err error) {
	peerAddr, peerAddrErr := ma.NewMultiaddr(peerAddrString)
	if peerAddrErr != nil {
		err = peerAddrErr
		return
	}

	peer, peerErr := peer.AddrInfoFromP2pAddr(peerAddr)
	if peerErr != nil {
		return
	}

	peerInfo = *peer
	return
}

func init() {
	bootstrapperStrings, readErr := getBootstrappers()
	if readErr != nil {
		logger.Warn(readErr)
		return
	}
	BOOTSTRAPPERS = append(BOOTSTRAPPERS, bootstrapperStrings...)
	for i := range BOOTSTRAPPERS {
		j := rand.IntN(i + 1)
		BOOTSTRAPPERS[i], BOOTSTRAPPERS[j] = BOOTSTRAPPERS[j], BOOTSTRAPPERS[i]
	}

	// bootstrappers = dht.GetDefaultBootstrapPeerAddrInfos()

	for _, peerAddrString := range BOOTSTRAPPERS {
		bootstrap, peerErr := MakeAddrInfo(peerAddrString)
		if peerErr != nil {
			continue
		}
		Bootstrappers = append(Bootstrappers, bootstrap)
	}
}
