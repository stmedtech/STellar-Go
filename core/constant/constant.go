package constant

import (
	"os"
	"path/filepath"
	"runtime"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-core-constant")

var STELLAR_PATH string = ""

const StellarAppID = "com.stmedicaltechnologyinc.stellar"

func StellarPath() (string, error) {
	homeDir, fileErr := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		homeDir = filepath.Join(homeDir, "AppData/Roaming")
	case "darwin":
		homeDir = filepath.Join(homeDir, "Library/Application Support")
	case "linux":
		homeDir = filepath.Join(homeDir, ".local/share")
	}
	if fileErr != nil {
		return "", fileErr
	}

	appDir := filepath.Join(homeDir, "Stellar")

	return appDir, nil
}

func init() {
	path, pathErr := StellarPath()
	if pathErr != nil {
		logger.Fatalln(pathErr)
		return
	}

	STELLAR_PATH = path
}
