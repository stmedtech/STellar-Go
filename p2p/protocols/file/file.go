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

type FileEntry struct {
	DirectoryName string
	Filename      string
	Size          int64
	IsDir         bool
	Children      []FileEntry
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
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			return err
		}

		relativePath, err := buf.ReadString('\n')
		if err != nil {
			return err
		}
		relativePath = strings.Trim(relativePath, "\n")
		relativePath = strings.Trim(relativePath, "/")
		relativePath = filepath.ToSlash(relativePath)
		dirPath := filepath.Join(dataDir, relativePath)

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return err
		}

		files := make([]FileEntry, 0)
		for _, e := range entries {
			isDir := e.IsDir()
			size := int64(0)
			if !isDir {
				fi, fiErr := os.Stat(filepath.Join(dirPath, e.Name()))
				if fiErr != nil {
					logger.Warnf("file ls error while reading file entry: %v", fiErr)
					continue
				}

				size = fi.Size()
			}

			files = append(files, FileEntry{
				Filename:      e.Name(),
				DirectoryName: relativePath,
				Size:          size,
				IsDir:         e.IsDir(),
			})
		}
		jsonData, jsonErr := json.Marshal(files)
		if jsonErr != nil {
			return jsonErr
		}

		_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
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
		fileName = filepath.ToSlash(fileName)
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

func List(n *node.Node, peer peer.ID, relativePath string) (files []FileEntry, err error) {
	files = make([]FileEntry, 0)
	err = nil

	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarFileProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)
	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarFileList)))
	if err != nil {
		return
	}

	data, err := buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")
	if data == constant.StellarEchoUnknownCommand {
		err = fmt.Errorf("file unknown command")
		return
	}
	if data != constant.StellarPong {
		err = fmt.Errorf("file get not receiving pong ack")
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", filepath.ToSlash(relativePath))))
	if err != nil {
		return
	}

	data, err = buf.ReadString('\n')
	if err != nil {
		return
	}
	data = strings.Trim(data, "\n")
	if data == constant.StellarEchoUnknownCommand {
		return
	}

	decodeErr := json.Unmarshal([]byte(data), &files)
	if decodeErr != nil {
		err = decodeErr
		return
	}

	return
}

func ListFullTree(n *node.Node, peer peer.ID) (files []FileEntry, err error) {
	var lsDirRecursive func(relativePath string) (fs []FileEntry, reErr error)

	lsDirRecursive = func(relativePath string) (fs []FileEntry, reErr error) {
		fs, reErr = List(n, peer, relativePath)
		if reErr != nil {
			return
		}
		logger.Infof("ls: %v", fs)
		for idx, f := range fs {
			if f.IsDir {
				children, rereErr := lsDirRecursive(filepath.ToSlash(filepath.Join(f.DirectoryName, f.Filename)))
				if rereErr != nil {
					reErr = rereErr
					return
				}
				fs[idx].Children = children
			}
		}

		return
	}
	files, err = lsDirRecursive("/")

	return
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
	if data == constant.StellarEchoUnknownCommand {
		err = fmt.Errorf("file unknown command")
		return
	}
	if data != constant.StellarPong {
		err = fmt.Errorf("file get not receiving pong ack")
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", filepath.ToSlash(fileName))))
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
	dir := filepath.Dir(filePath)
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, os.ModeDir)
		if err != nil {
			return
		}
	}

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
