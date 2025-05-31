package identity

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	mrand "math/rand"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func DecodePrivateKey(b64RawBytes string) (crypto.PrivKey, error) {
	decodedData, err := base64.StdEncoding.DecodeString(b64RawBytes)
	if err != nil {
		return nil, err
	}

	privKey, privKeyErr := crypto.UnmarshalEd25519PrivateKey(decodedData)
	if privKeyErr != nil {
		return nil, privKeyErr
	}
	return privKey, nil
}

func GeneratePrivateKey(randseed int64) (crypto.PrivKey, error) {
	var r io.Reader
	if randseed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	privKey, _, privKeyErr := crypto.GenerateEd25519Key(r)
	if privKeyErr != nil {
		return nil, privKeyErr
	}

	return privKey, nil
}

func EncodePrivateKey(privKey crypto.PrivKey) (string, error) {
	data, rawErr := privKey.Raw()
	if rawErr != nil {
		return "", rawErr
	}
	encodedData := base64.StdEncoding.EncodeToString(data)

	return encodedData, nil
}
