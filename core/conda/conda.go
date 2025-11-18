package conda

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"stellar/core/constant"
	"strings"

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

func CondaDownloadPath() (string, error) {
	appDir, fileErr := constant.StellarPath()
	if fileErr != nil {
		return "", fileErr
	}

	condaPath := filepath.Join(appDir, "conda")
	if fileErr = os.MkdirAll(condaPath, os.ModePerm); fileErr != nil {
		return "", fileErr
	}

	return condaPath, nil
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func getPath() (string, error) {
	if os := runtime.GOOS; os == "windows" {
		if condaPath := "C:\\ProgramData\\miniconda3\\Scripts\\conda.exe"; commandExists(condaPath) {
			return condaPath, nil
		}
		if condaPath := "C:\\ProgramData\\miniconda3\\_conda.exe"; commandExists(condaPath) {
			return condaPath, nil
		}
	}

	if condaDir, fileErr := CondaDownloadPath(); fileErr != nil {
		return "", fileErr
	} else {
		if condaPath := filepath.Join(condaDir, "_conda"); commandExists(condaPath) {
			return filepath.Join(condaDir, "bin", "conda"), nil
		}

		if condaPath := "conda"; commandExists(condaPath) {
			return condaPath, nil
		}
	}

	return "", fmt.Errorf("conda executable not found")
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

func Version(condaPath string) (string, error) {
	err := fmt.Errorf("version not supported")

	cmd := exec.Command(condaPath, "--version")
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return "", cmdErr
	}

	re, regErr := regexp.Compile(`\d+.\d+.\d+`)
	if regErr != nil {
		return "", regErr
	}

	if version := re.FindString(string(stdout)); version == "" {
		return "", err
	} else {
		return version, nil
	}
}

func EnvList(condaPath string) (map[string]string, error) {
	envs := make(map[string]string)

	cmd := exec.Command(condaPath, "env", "list")
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return envs, cmdErr
	}

	re, regErr := regexp.Compile(`\S+ {3,}[* ]*\S+`)
	if regErr != nil {
		return envs, regErr
	}

	envLines := re.FindAllString(string(stdout), -1)
	if len(envLines) == 0 {
		return envs, fmt.Errorf("parsing error")
	}

	for _, envLine := range envLines {
		s := strings.Split(envLine, " ")
		if len(s) >= 2 && s[0] != "" && s[len(s)-1] != "" {
			// ignore base env
			if s[0] != "base" {
				envs[s[0]] = s[len(s)-1]
			}
		}
	}

	return envs, nil
}

func Env(condaPath, env string) (string, error) {
	envs, envsErr := EnvList(condaPath)
	if envsErr != nil {
		return "", envsErr
	}
	if val, ok := envs[env]; ok {
		return val, nil
	}
	return "", fmt.Errorf("environment not found")
}

func CommandPath() (string, error) {
	condaPath, err := getPath()
	if err != nil {
		logger.Warn(err)
		return "", err
	}

	condaVersion, err := Version(condaPath)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Conda Version: %v", condaVersion)
	logger.Debugln(msg)

	return condaPath, nil
}

func CreateEnv(condaPath, env, version string) (string, error) {
	err := fmt.Errorf("environment creation failed")

	if envPath, envErr := Env(condaPath, env); envErr == nil {
		msg := fmt.Sprintf("environment %v exist", env)
		logger.Warnln(msg)
		return envPath, nil
	}

	cmd := exec.Command(condaPath, "create", "--name", env, fmt.Sprintf("python=%v", version), "-y")
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return "", cmdErr
	}

	re, regErr := regexp.Compile(fmt.Sprintf("conda activate %v", env))
	if regErr != nil {
		return "", regErr
	}

	if ok := re.FindString(string(stdout)); ok == "" {
		return "", err
	}
	logger.Debugln(string(stdout))

	envPath, envErr := Env(condaPath, env)
	if envErr != nil {
		return "", envErr
	}

	return envPath, nil
}

func RemoveEnv(condaPath, env string) error {
	err := fmt.Errorf("environment deletion failed")

	if _, envErr := Env(condaPath, env); envErr != nil {
		return fmt.Errorf("environment not exist")
	}

	cmd := exec.Command(condaPath, "env", "remove", "--name", env, "-y")
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return cmdErr
	}

	re, regErr := regexp.Compile("Executing transaction: done")
	if regErr != nil {
		return regErr
	}

	if ok := re.FindString(string(stdout)); ok == "" {
		return err
	}
	logger.Debugln(string(stdout))

	return nil
}

func UpdateEnv(condaPath, env, yamlPath string) error {
	err := fmt.Errorf("environment update failed")

	if _, envErr := Env(condaPath, env); envErr != nil {
		return fmt.Errorf("environment not exist")
	}

	cmd := exec.Command(condaPath, "env", "update", "--name", env, "--file", yamlPath)
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return cmdErr
	}

	re, regErr := regexp.Compile(fmt.Sprintf("conda activate %v", env))
	if regErr != nil {
		return regErr
	}

	if ok := re.FindString(string(stdout)); ok == "" {
		return err
	}

	return nil
}

func RunCommand(condaPath, env string, arg ...string) (string, error) {
	if _, envErr := Env(condaPath, env); envErr != nil {
		return "", fmt.Errorf("environment not exist")
	}

	args := append([]string{"run", "--name", env}, arg...)

	cmd := exec.Command(condaPath, args...)
	stdout, cmdErr := runCommand(cmd)
	if cmdErr != nil {
		return "", cmdErr
	}

	return string(stdout), nil
}

func EnvInstallPackage(condaPath, env, packageName string) error {
	stdout, cmdErr := RunCommand(condaPath, env, "pip", "install", packageName)

	re, regErr := regexp.Compile("Requirement already satisfied")
	if regErr != nil {
		return regErr
	}
	if ok := re.FindString(string(stdout)); ok == "" {
		if cmdErr != nil {
			return cmdErr
		}
	} else {
		logger.Warnln("[warning] package already installed")
	}

	return nil
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

func DownloadUrl(pythonVersion, condaVersion string) (string, error) {
	url := fmt.Sprintf("https://repo.anaconda.com/miniconda/Miniconda3-%s_%s-", pythonVersion, condaVersion)

	switch os := runtime.GOOS; os {
	case "darwin":
		switch arch := runtime.GOARCH; arch {
		case "amd64":
			url += "MacOSX-x86_64.sh"
		case "arm":
		case "arm64":
			url += "MacOSX-arm64.sh"
		default:
			return "", fmt.Errorf("get download url failed due to unsupported architecture: %v", arch)
		}
	case "linux":
		switch arch := runtime.GOARCH; arch {
		case "amd64":
			url += "Linux-x86_64.sh"
		case "arm":
		case "arm64":
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

func Install(version string) error {
	appDir, fileErr := constant.StellarPath()
	if fileErr != nil {
		return fileErr
	}

	condaDownloadUrl, err := DownloadUrl(version, CONDA_VERSION)
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
