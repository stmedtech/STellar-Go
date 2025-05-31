package file

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"stellar/p2p/constant"
	"stellar/p2p/node"
	"strings"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/progressbar/v3"
)

var logger = golog.Logger("stellar-p2p-protocols-file")
var dataDir string

func init() {
	pwd, err := os.Getwd()
	if err != nil {
		return
	}
	dataDir = filepath.Join(pwd, "data")
}

func BindFileStream(n *node.Node) {
	n.Host.SetStreamHandler(constant.StellarFileProtocol, func(s network.Stream) {
		// TODO security
		if err := doStellarFile(n, s); err != nil {
			logger.Warnf("file error: %v", err)
			s.Reset()
		} else {
			s.Close()
		}
	})
}

type FileInfo struct {
	Filename string
	Size     int64
	Checksum string
}

func FileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = io.Copy(hash, file)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func doStellarFile(n *node.Node, s network.Stream) error {
	buf := bufio.NewReader(s)

	str, err := buf.ReadString('\n')
	if err != nil {
		return err
	}
	str = strings.Trim(str, "\n")

	response := ""
	switch str {
	case constant.StellarFileList:
		files := make([]string, 0)

		entries, err := os.ReadDir(dataDir)
		if err != nil {
			return err
		}

		for _, e := range entries {
			files = append(files, e.Name())
		}
		jsonData, jsonErr := json.Marshal(files)
		if jsonErr != nil {
			return jsonErr
		}
		response = string(jsonData)
		_, err = s.Write([]byte(response))
		return err
	case constant.StellarFileGet:
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			return err
		}

		fileName, err := buf.ReadString('\n')
		if err != nil {
			return err
		}
		fileName = strings.Trim(fileName, "\n")
		fileName = filepath.Join(dataDir, fileName)

		fi, err := os.Stat(fileName)
		if err != nil {
			return err
		}

		checksum, checksumErr := FileChecksum(fileName)
		if checksumErr != nil {
			return checksumErr
		}

		finfo := FileInfo{
			Filename: fileName,
			Size:     fi.Size(),
			Checksum: checksum,
		}
		jsonData, jsonErr := json.Marshal(finfo)
		if jsonErr != nil {
			return jsonErr
		}

		_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
		if err != nil {
			return err
		}

		data, err := buf.ReadString('\n')
		if err != nil {
			return err
		}
		data = strings.Trim(data, "\n")
		if string(data) != constant.StellarPong {
			return fmt.Errorf("file get not receiving pong ack")
		}

		file, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer file.Close()

		logger.Debugf("start copying file %v", fileName)
		_, err = io.Copy(s, file)
		if err != nil {
			return err
		}
		return nil
	default:
		response = constant.StellarFileUnknownCommand
		_, err = s.Write([]byte(response))
		return err
	}
}

func List(n *node.Node, peer peer.ID) ([]string, error) {
	files := make([]string, 0)

	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarFileProtocol)
	if err != nil {
		return files, err
	}
	defer s.Close()

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarFileList)))
	if err != nil {
		return files, err
	}

	data, err := io.ReadAll(s)
	if err != nil {
		return files, err
	}
	if string(data) == constant.StellarEchoUnknownCommand {
		return files, err
	}

	decodeErr := json.Unmarshal(data, &files)
	if decodeErr != nil {
		return files, decodeErr
	}

	return files, nil
}

func Download(n *node.Node, peer peer.ID, fileName string) (filePath string, err error) {
	filePath = ""
	err = nil

	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarFileProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)
	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarFileGet)))
	if err != nil {
		return
	}

	data, err := buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")
	if string(data) == constant.StellarEchoUnknownCommand {
		return
	}
	if string(data) != constant.StellarPong {
		err = fmt.Errorf("file get not receiving pong ack")
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", fileName)))
	if err != nil {
		return
	}

	data, err = buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")

	var finfo FileInfo
	decodeErr := json.Unmarshal([]byte(data), &finfo)
	if decodeErr != nil {
		err = decodeErr
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
	if err != nil {
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
	if err != nil {
		return
	}

	filePath = filepath.Join(dataDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	logger.Debugf("start copying file to %v", filePath)
	bar := progressbar.DefaultBytes(
		finfo.Size,
		"downloading",
	)
	_, err = io.Copy(io.MultiWriter(file, bar), s)
	if err != nil {
		return
	}

	checksum, checksumErr := FileChecksum(filePath)
	if checksumErr != nil {
		err = checksumErr
		return
	}
	if checksum != finfo.Checksum {
		err = fmt.Errorf("downloaded file checksum %v mismatch with original %v", checksum, finfo.Checksum)
		return
	}

	return
}
