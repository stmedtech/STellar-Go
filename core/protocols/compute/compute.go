package compute

import (
	"bufio"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"stellar/core/conda"
	"stellar/core/util"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/file"
	"strings"

	"github.com/google/uuid"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

var logger = golog.Logger("stellar-core-protocols-compute")

type CondaPythonPreparation struct {
	Env         string
	Version     string
	EnvYamlPath string
}

func (f *CondaPythonPreparation) Prepare() (envPath string, err error) {
	envYamlPath := filepath.ToSlash(f.EnvYamlPath)
	envYamlPath = filepath.Join(file.DataDir, envYamlPath)
	defer os.Remove(envYamlPath)

	var tempDir string
	tempDir, err = createTempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDir)

	newPath := filepath.Join(tempDir, filepath.Base(envYamlPath))

	_, err = util.CopyFile(envYamlPath, newPath)
	if err != nil {
		return
	}
	envYamlPath = newPath

	if envPath, err = conda.CreateEnv(conda.CondaPath, f.Env, f.Version); err != nil {
		return
	}

	err = conda.UpdateEnv(conda.CondaPath, f.Env, envYamlPath)
	if err != nil {
		return
	}

	logger.Infof("successfully prepared conda python env %s[%s]", f.Env, f.Version)

	return
}

type CondaPythonScriptExecution struct {
	Env        string
	ScriptPath string
}

func (f *CondaPythonScriptExecution) Execute() (result string, err error) {
	scriptPath := filepath.ToSlash(f.ScriptPath)
	scriptPath = filepath.Join(file.DataDir, scriptPath)
	defer os.Remove(scriptPath)

	var tempDir string
	tempDir, err = createTempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDir)

	newPath := filepath.Join(tempDir, filepath.Base(scriptPath))

	_, err = util.CopyFile(scriptPath, newPath)
	if err != nil {
		return
	}
	scriptPath = newPath

	if result, err = conda.RunCommand(conda.CondaPath, f.Env, "python", scriptPath); err != nil {
		return
	}

	return
}

func computeStreamHandler(s network.Stream) {
	if err := doStellarCompute(s); err != nil {
		// TODO improve error handling
		logger.Warnf("compute error: %v", err)
		s.ResetWithError(406)
	} else {
		s.Close()
	}
}

func BindComputeStream(n *node.Node) {
	conda.UpdateCondaPath()

	n.Host.SetStreamHandler(constant.StellarComputeProtocol, n.Policy.AuthorizeStream(computeStreamHandler))
	logger.Info("Compute protocol is ready")
}

func createTempDir() (tempDir string, err error) {
	tempDir, err = os.MkdirTemp("", "stellar-compute-temp-dir-*")
	if err != nil {
		err = fmt.Errorf("error creating temporary directory: %v", err)
		return
	}

	return
}

func sendTempFile(n *node.Node, peer peer.ID, filePath string) (savePath string, err error) {
	s, err := os.Stat(filePath)
	if err != nil {
		return
	}
	if s.IsDir() {
		err = fmt.Errorf("file path %s is not a valid file", filePath)
		return
	}

	savePath = fmt.Sprintf("%s%s", uuid.New().String(), filepath.Ext(filePath))

	err = file.Upload(n, peer, filePath, savePath)
	if err != nil {
		return
	}

	return
}

func doStellarCompute(s network.Stream) (err error) {
	buf := bufio.NewReader(s)

	str, err := buf.ReadString('\n')
	if err != nil {
		return
	}
	str = strings.Trim(str, "\n")

	switch str {
	case constant.StellarComputePrepareCondaPython:
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			return
		}

		var data string
		data, err = buf.ReadString('\n')
		if err != nil {
			return
		}
		data = strings.Trim(data, "\n")

		var payload CondaPythonPreparation
		err = json.Unmarshal([]byte(data), &payload)
		if err != nil {
			return
		}

		var envPath string
		envPath, err = payload.Prepare()
		if err != nil {
			return
		}

		_, err = s.Write([]byte(fmt.Sprintf("%v\n", envPath)))
		if err != nil {
			return
		}

		return
	case constant.StellarComputeListCondaPythonEnvs:
		var envs map[string]string
		envs, err = conda.EnvList(conda.CondaPath)
		if err != nil {
			return
		}

		jsonData, jsonErr := json.Marshal(envs)
		if jsonErr != nil {
			err = jsonErr
			return
		}

		_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
		if err != nil {
			return
		}

		return
	case constant.StellarComputeExecuteScript:
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			return
		}

		var data string
		data, err = buf.ReadString('\n')
		if err != nil {
			return
		}
		data = strings.Trim(data, "\n")

		var payload CondaPythonScriptExecution
		err = json.Unmarshal([]byte(data), &payload)
		if err != nil {
			return
		}

		var result string
		result, err = payload.Execute()
		if err != nil {
			return
		}

		b64Result := b64.StdEncoding.EncodeToString([]byte(result))
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", b64Result)))
		if err != nil {
			return
		}

		return
	default:
		_, err = s.Write([]byte(constant.StellarComputeUnknownCommand))
		return
	}
}

func PrepareCondaPython(n *node.Node, peer peer.ID, form CondaPythonPreparation) (envPath string, err error) {
	envYamlPath, err := sendTempFile(n, peer, form.EnvYamlPath)
	if err != nil {
		return
	}
	form.EnvYamlPath = envYamlPath
	logger.Infof("compute conda python env file %s sent", form.EnvYamlPath)

	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarComputeProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarComputePrepareCondaPython)))
	if err != nil {
		return
	}

	data, err := buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")
	if data == constant.StellarEchoUnknownCommand {
		err = fmt.Errorf("compute unknown command")
		return
	}
	if data != constant.StellarPong {
		err = fmt.Errorf("compute get not receiving pong ack")
		return
	}

	jsonData, jsonErr := json.Marshal(form)
	if jsonErr != nil {
		err = jsonErr
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
	if err != nil {
		return
	}

	envPath, err = buf.ReadString('\n')
	if err != nil {
		return
	}

	return
}

func ListCondaPythonEnvs(n *node.Node, peer peer.ID) (envs map[string]string, err error) {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarComputeProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarComputeListCondaPythonEnvs)))
	if err != nil {
		return
	}

	var data string
	data, err = buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")

	err = json.Unmarshal([]byte(data), &envs)
	if err != nil {
		return
	}

	return
}

func ExecuteCondaPythonScript(n *node.Node, peer peer.ID, form CondaPythonScriptExecution) (result string, err error) {
	scriptPath, err := sendTempFile(n, peer, form.ScriptPath)
	if err != nil {
		return
	}
	form.ScriptPath = scriptPath
	logger.Infof("execute conda python script file %s sent", form.ScriptPath)

	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarComputeProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarComputeExecuteScript)))
	if err != nil {
		return
	}

	data, err := buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")
	if data == constant.StellarEchoUnknownCommand {
		err = fmt.Errorf("compute unknown command")
		return
	}
	if data != constant.StellarPong {
		err = fmt.Errorf("compute get not receiving pong ack")
		return
	}

	jsonData, jsonErr := json.Marshal(form)
	if jsonErr != nil {
		err = jsonErr
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
	if err != nil {
		return
	}

	b64Result, err := buf.ReadString('\n')
	if err != nil {
		return
	}

	b64ResultDec, err := b64.StdEncoding.DecodeString(b64Result)
	if err != nil {
		return
	}
	result = string(b64ResultDec)

	return
}
