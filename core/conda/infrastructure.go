package conda

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"stellar/core/constant"
)

// FindCondaPath finds the conda executable path
// It searches in the following order:
// 1. Windows: C:\ProgramData\miniconda3\Scripts\conda.exe
// 2. Windows: C:\ProgramData\miniconda3\_conda.exe
// 3. Local install directory (from CondaDownloadPath)
// 4. System PATH
func FindCondaPath() (string, error) {
	// Helper function to check if command exists
	cmdExists := func(cmd string) bool {
		_, err := exec.LookPath(cmd)
		return err == nil
	}

	if runtime.GOOS == "windows" {
		if condaPath := "C:\\ProgramData\\miniconda3\\Scripts\\conda.exe"; cmdExists(condaPath) {
			return condaPath, nil
		}
		if condaPath := "C:\\ProgramData\\miniconda3\\_conda.exe"; cmdExists(condaPath) {
			return condaPath, nil
		}
	}

	if condaDir, fileErr := GetCondaDownloadPath(); fileErr != nil {
		return "", fileErr
	} else {
		if condaPath := filepath.Join(condaDir, "_conda"); cmdExists(condaPath) {
			return filepath.Join(condaDir, "bin", "conda"), nil
		}

		if condaPath := "conda"; cmdExists(condaPath) {
			return condaPath, nil
		}
	}

	return "", fmt.Errorf("conda executable not found")
}

// GetCondaVersion gets the conda version by running conda --version
// NOTE: This function uses exec.Command directly. It will be refactored in Phase 3
// to use the Executor interface for better testability.
func GetCondaVersion(condaPath string) (string, error) {
	err := fmt.Errorf("version not supported")

	cmd := exec.Command(condaPath, "--version")
	output, cmdErr := cmd.Output()
	if cmdErr != nil {
		return "", fmt.Errorf("failed to run conda --version: %w", cmdErr)
	}

	re, regErr := regexp.Compile(`\d+\.\d+\.\d+`)
	if regErr != nil {
		return "", fmt.Errorf("failed to compile version regex: %w", regErr)
	}

	if version := re.FindString(string(output)); version == "" {
		return "", err
	} else {
		return version, nil
	}
}

// GetCondaDownloadPath returns the path where conda installers are downloaded
func GetCondaDownloadPath() (string, error) {
	appDir, fileErr := constant.StellarPath()
	if fileErr != nil {
		return "", fileErr
	}

	condaPath := filepath.Join(appDir, "conda")
	if fileErr = os.MkdirAll(condaPath, os.ModePerm); fileErr != nil {
		return "", fmt.Errorf("failed to create conda directory: %w", fileErr)
	}

	return condaPath, nil
}

// DownloadCondaInstaller generates the download URL for a conda installer
// based on Python version, conda version, OS, and architecture
func DownloadCondaInstaller(pythonVersion, condaVersion string) (string, error) {
	url := fmt.Sprintf("https://repo.anaconda.com/miniconda/Miniconda3-%s_%s-", pythonVersion, condaVersion)

	switch os := runtime.GOOS; os {
	case "darwin":
		switch arch := runtime.GOARCH; arch {
		case "amd64":
			url += "MacOSX-x86_64.sh"
		case "arm", "arm64":
			url += "MacOSX-arm64.sh"
		default:
			return "", fmt.Errorf("get download url failed due to unsupported architecture: %v", arch)
		}
	case "linux":
		switch arch := runtime.GOARCH; arch {
		case "amd64":
			url += "Linux-x86_64.sh"
		case "arm", "arm64":
			url += "Linux-aarch64.sh"
		default:
			return "", fmt.Errorf("get download url failed due to unsupported architecture: %v", arch)
		}
	case "windows":
		switch arch := runtime.GOARCH; arch {
		case "amd64":
			url += "Windows-x86_64.exe"
		default:
			return "", fmt.Errorf("get download url failed due to unsupported architecture: %v", arch)
		}
	default:
		return "", fmt.Errorf("get download url failed due to unsupported OS: %v", os)
	}
	return url, nil
}
