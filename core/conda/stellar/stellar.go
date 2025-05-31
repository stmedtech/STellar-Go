package stellar

import (
	"os"
	"path/filepath"
	"runtime"
)

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
	fileErr = os.MkdirAll(appDir, os.ModePerm)
	if fileErr != nil {
		return "", fileErr
	}

	return appDir, nil
}
