package conda

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"stellar/core/constant"

	golog "github.com/ipfs/go-log/v2"
	"github.com/schollz/progressbar/v3"
)

var logger = golog.Logger("stellar-conda")

const CONDA_VERSION = "25.3.1-1"

var CondaPath string = ""

func UpdateCondaPath() bool {
	condaPath, err := CommandPath()
	if err != nil {
		logger.Warnf("error getting conda command path: %v", err)
		return false
	}

	CondaPath = condaPath
	logger.Infof("conda command path is %s", CondaPath)
	return true
}

func init() {
	UpdateCondaPath()
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

type saveOutput struct {
	savedOutput []byte
}

func (so *saveOutput) Write(p []byte) (n int, err error) {
	so.savedOutput = append(so.savedOutput, p...)
	logger.Infof("conda output: %s", string(p))
	return len(p), nil
}

func runCommand(cmd *exec.Cmd) ([]byte, error) {
	var so saveOutput

	cmd.Stdin = os.Stdin
	cmd.Stdout = &so
	cmd.Stderr = &so
	cmdErr := cmd.Run()
	if cmdErr != nil {
		return make([]byte, 0), cmdErr
	}
	return so.savedOutput, nil
}

func CommandPath() (string, error) {
	condaPath, err := FindCondaPath()
	if err != nil {
		logger.Warn(err)
		return "", err
	}

	condaVersion, err := GetCondaVersion(condaPath)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Conda Version: %v", condaVersion)
	logger.Debugln(msg)

	return condaPath, nil
}

func Download(folder, url string) (string, error) {
	resp, getErr := http.Get(url)
	if getErr != nil {
		return "", getErr
	}
	defer resp.Body.Close()

	filePath := filepath.Join(folder, filepath.Base(url))
	tempDestinationPath := filePath + ".tmp"

	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		if contentType := resp.Header.Get("Content-Type"); contentType == "application/octet-stream" {
			f, _ := os.OpenFile(tempDestinationPath, os.O_CREATE|os.O_WRONLY, 0644)
			defer f.Close()

			bar := progressbar.DefaultBytes(
				resp.ContentLength,
				"downloading",
			)
			io.Copy(io.MultiWriter(f, bar), resp.Body)

			defer os.Rename(tempDestinationPath, filePath)

			return filePath, nil
		} else {
			return "", fmt.Errorf("requested URL is not downloadable")
		}
	} else {
		return filePath, nil
	}
}

func Install(version string) error {
	appDir, fileErr := constant.StellarPath()
	if fileErr != nil {
		return fileErr
	}

	condaDownloadUrl, err := DownloadCondaInstaller(version, CONDA_VERSION)
	if err != nil {
		return err
	}

	filePath, err := Download(appDir, condaDownloadUrl)
	if err != nil {
		return err
	}
	logger.Infoln("Downloaded: " + condaDownloadUrl + " to " + filePath)

	switch os := runtime.GOOS; os {
	case "darwin":
	case "linux":
		cmd := exec.Command("/bin/sh", filePath, "-b", "-f", "-p", filepath.Join(appDir, "conda"))
		stdout, cmdErr := runCommand(cmd)
		if cmdErr != nil {
			return fmt.Errorf("installation failed due to %v", cmdErr)
		}
		logger.Info(string(stdout))
	case "windows":
		cmd := exec.Command(filePath)
		stdout, cmdErr := runCommand(cmd)
		if cmdErr != nil {
			return fmt.Errorf("installation failed due to %v", cmdErr)
		}
		logger.Info(string(stdout))
	default:
		return fmt.Errorf("installation failed due to unsupported OS: %v", os)
	}

	return nil
}
