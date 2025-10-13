package main

import (
	"flag"
	"os"
	"stellar/p2p/identity"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
)

func keyCommand() {
	var logger = golog.Logger("stellar")

	keyCmd := flag.NewFlagSet("key", flag.ExitOnError)
	seed := keyCmd.Int64("seed", 0, "set random seed for private key generation")
	b64privkey := keyCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")

	keyCmd.Parse(os.Args[2:])

	if *b64privkey == "" {
		privKey, privKeyErr := identity.GeneratePrivateKey(*seed)
		if privKeyErr != nil {
			panic(privKeyErr)
		}

		encodedData, encodeErr := identity.EncodePrivateKey(privKey)
		if encodeErr != nil {
			panic(encodeErr)
		}
		logger.Infof("Generated encoded key: %v", encodedData)
	} else {
		privKey, privKeyErr := identity.DecodePrivateKey(*b64privkey)
		if privKeyErr != nil {
			logger.Fatalln(privKeyErr)
		}
		id, idErr := peer.IDFromPublicKey(privKey.GetPublic())
		if idErr != nil {
			logger.Fatalln(idErr)
		}
		logger.Infof("Decoded ID from private key: %v", id.String())
	}
}
