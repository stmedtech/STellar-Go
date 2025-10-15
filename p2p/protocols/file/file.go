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
	"time"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/progressbar/v3"
)

var logger = golog.Logger("stellar-p2p-protocols-file")
var DataDir string

// FileTransferTimeout is the timeout for file transfer operations
const FileTransferTimeout = 30 * time.Second

func init() {
	pwd, err := os.Getwd()
	if err != nil {
		return
	}
	DataDir = filepath.Join(pwd, "data")
}

// readWithTimeout reads from a reader with a timeout
func readWithTimeout(reader io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		data string
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := reader.Read(buf)
		if err != nil {
			ch <- result{"", err}
			return
		}
		ch <- result{string(buf[:n]), nil}
	}()

	select {
	case res := <-ch:
		return res.data, res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("read timeout after %v", timeout)
	}
}

func fileStreamHandler(s network.Stream) {
	if err := doStellarFile(s); err != nil {
		// TODO improve error handling
		logger.Warnf("file error: %v", err)
		s.ResetWithError(406)
	} else {
		s.Close()
	}
}

func BindFileStream(n *node.Node) {
	n.Host.SetStreamHandler(constant.StellarFileProtocol, n.Policy.AuthorizeStream(fileStreamHandler))
	logger.Info("File protocol is ready")
}

type FileEntry struct {
	DirectoryName string
	Filename      string
	Size          int64
	IsDir         bool
	Children      []FileEntry
}

func (e *FileEntry) FullName() string {
	return filepath.ToSlash(filepath.Join(e.DirectoryName, e.Filename))
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

func doStellarFile(s network.Stream) error {
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
			logger.Errorf("Failed to write pong response: %v", err)
			return err
		}

		relativePath, err := buf.ReadString('\n')
		if err != nil {
			logger.Errorf("Failed to read relative path: %v", err)
			return err
		}
		relativePath = strings.Trim(relativePath, "\n")
		relativePath = strings.Trim(relativePath, "/")
		relativePath = filepath.ToSlash(relativePath)
		dirPath := filepath.Join(DataDir, relativePath)

		// Check if directory exists
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			logger.Errorf("Directory does not exist: %s", dirPath)
			return fmt.Errorf("directory not found: %s", relativePath)
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			logger.Errorf("Failed to read directory %s: %v", dirPath, err)
			return fmt.Errorf("failed to read directory: %v", err)
		}

		files := make([]FileEntry, 0)
		for _, e := range entries {
			isDir := e.IsDir()
			size := int64(0)
			if !isDir {
				fi, fiErr := os.Stat(filepath.Join(dirPath, e.Name()))
				if fiErr != nil {
					logger.Warnf("file ls error while reading file entry %s: %v", e.Name(), fiErr)
					continue
				}

				size = fi.Size()
			}

			// Fix directory name to properly reflect the path structure
			displayPath := relativePath
			if displayPath == "" {
				displayPath = "/"
			}

			files = append(files, FileEntry{
				Filename:      e.Name(),
				DirectoryName: displayPath,
				Size:          size,
				IsDir:         e.IsDir(),
			})
		}

		jsonData, jsonErr := json.Marshal(files)
		if jsonErr != nil {
			logger.Errorf("Failed to marshal file list to JSON: %v", jsonErr)
			return fmt.Errorf("failed to serialize file list: %v", jsonErr)
		}

		_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
		if err != nil {
			logger.Errorf("Failed to write file list response: %v", err)
		}
		return err
	case constant.StellarFileGet:
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			logger.Errorf("Failed to write pong response for file get: %v", err)
			return err
		}

		fileName, err := buf.ReadString('\n')
		if err != nil {
			logger.Errorf("Failed to read file name: %v", err)
			return err
		}
		fileName = strings.Trim(fileName, "\n")
		fileName = filepath.ToSlash(fileName)
		filePath := filepath.Join(DataDir, fileName)

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			logger.Errorf("File does not exist: %s", filePath)
			return fmt.Errorf("file not found: %s", fileName)
		}

		logger.Infof("Sending file: %s", filePath)
		err = upload(s, filePath)
		if err != nil {
			logger.Errorf("Failed to upload file %s: %v", filePath, err)
			return fmt.Errorf("failed to send file: %v", err)
		}
		return nil
	case constant.StellarFileSend:
		_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		if err != nil {
			logger.Errorf("Failed to write pong response for file send: %v", err)
			return err
		}

		fileName, err := buf.ReadString('\n')
		if err != nil {
			logger.Errorf("Failed to read file name for upload: %v", err)
			return err
		}
		fileName = strings.Trim(fileName, "\n")
		fileName = filepath.ToSlash(fileName)
		filePath := filepath.Join(DataDir, fileName)

		// Ensure directory exists
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Errorf("Failed to create directory %s: %v", dir, err)
			return fmt.Errorf("failed to create directory: %v", err)
		}

		logger.Infof("Receiving file: %s", filePath)
		_, err = download(s, filePath)
		if err != nil {
			logger.Errorf("Failed to download file to %s: %v", filePath, err)
			return fmt.Errorf("failed to receive file: %v", err)
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

	// Set a timeout for reading the initial acknowledgment
	done := make(chan error, 1)
	var data string
	go func() {
		var readErr error
		data, readErr = buf.ReadString('\n')
		done <- readErr
	}()

	select {
	case err = <-done:
		if err != nil {
			return
		}
	case <-time.After(FileTransferTimeout):
		return nil, fmt.Errorf("timeout waiting for initial acknowledgment")
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

func upload(s io.ReadWriteCloser, filePath string) (err error) {
	buf := bufio.NewReader(s)

	logger.Infof("Starting file upload: %s", filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		logger.Errorf("Failed to stat file %s: %v", filePath, err)
		return
	}

	logger.Infof("File size: %d bytes", fi.Size())

	checksum, checksumErr := FileChecksum(filePath)
	if checksumErr != nil {
		logger.Errorf("Failed to calculate checksum for %s: %v", filePath, checksumErr)
		err = checksumErr
		return
	}

	logger.Infof("File checksum: %s", checksum)

	finfo := FileInfo{
		Filename: filePath,
		Size:     fi.Size(),
		Checksum: checksum,
	}
	jsonData, jsonErr := json.Marshal(finfo)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal file info: %v", jsonErr)
		err = jsonErr
		return
	}

	logger.Infof("Sending file info: %s", string(jsonData))
	_, err = s.Write([]byte(fmt.Sprintf("%v\n", string(jsonData))))
	if err != nil {
		logger.Errorf("Failed to write file info: %v", err)
		return
	}

	// Flush the stream to ensure data is sent immediately
	if flusher, ok := s.(interface{ Flush() error }); ok {
		if flushErr := flusher.Flush(); flushErr != nil {
			logger.Errorf("Failed to flush stream: %v", flushErr)
			return
		}
	}

	logger.Infof("Waiting for pong acknowledgment...")

	// Set a timeout for reading the pong acknowledgment
	done := make(chan error, 1)
	var data string
	go func() {
		var readErr error
		data, readErr = buf.ReadString('\n')
		done <- readErr
	}()

	select {
	case err = <-done:
		if err != nil {
			logger.Errorf("Failed to read pong acknowledgment: %v", err)
			return
		}
	case <-time.After(FileTransferTimeout):
		logger.Errorf("Timeout waiting for pong acknowledgment after %v", FileTransferTimeout)
		return fmt.Errorf("timeout waiting for pong acknowledgment")
	}
	data = strings.Trim(data, "\n")
	logger.Infof("Received response: %s", data)
	if string(data) != constant.StellarPong {
		err = fmt.Errorf("file send not receiving pong ack, got: %s", data)
		logger.Errorf("Expected pong, got: %s", data)
		return
	}

	logger.Infof("Opening file for reading: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		logger.Errorf("Failed to open file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	logger.Infof("Starting file copy, size: %d bytes", finfo.Size)
	bar := progressbar.DefaultBytes(
		finfo.Size,
		"uploading",
	)
	bytesCopied, err := io.CopyN(io.MultiWriter(s, bar), file, finfo.Size)
	if err != nil {
		logger.Errorf("Failed to copy file data: %v", err)
		return
	}
	logger.Infof("Copied %d bytes", bytesCopied)

	logger.Infof("Waiting for final pong acknowledgment...")
	data, err = buf.ReadString('\n')
	if err != nil {
		logger.Errorf("Failed to read final pong: %v", err)
		return
	}
	data = strings.Trim(data, "\n")
	logger.Infof("Received final response: %s", data)
	if data != constant.StellarPong {
		err = fmt.Errorf("file send not receiving final pong ack, got: %s", data)
		logger.Errorf("Expected final pong, got: %s", data)
		return
	}

	logger.Infof("File upload completed successfully: %s", filePath)
	return nil
}

func download(s io.ReadWriteCloser, fileName string) (filePath string, err error) {
	buf := bufio.NewReader(s)

	logger.Infof("Starting file download: %s", fileName)

	logger.Infof("Waiting for file info...")

	// Set a timeout for reading the file info
	done := make(chan error, 1)
	var data string
	go func() {
		var readErr error
		data, readErr = buf.ReadString('\n')
		done <- readErr
	}()

	select {
	case err = <-done:
		if err != nil {
			logger.Errorf("Failed to read file info: %v", err)
			return
		}
	case <-time.After(FileTransferTimeout):
		logger.Errorf("Timeout waiting for file info after %v", FileTransferTimeout)
		return "", fmt.Errorf("timeout waiting for file info")
	}
	data = strings.Trim(data, "\n")
	logger.Infof("Received file info: %s", data)

	var finfo FileInfo
	decodeErr := json.Unmarshal([]byte(data), &finfo)
	if decodeErr != nil {
		logger.Errorf("Failed to unmarshal file info: %v", decodeErr)
		err = decodeErr
		return
	}

	logger.Infof("File info - Name: %s, Size: %d, Checksum: %s", finfo.Filename, finfo.Size, finfo.Checksum)

	logger.Infof("Sending pong acknowledgment...")

	// Set a timeout for sending the pong acknowledgment
	pongDone := make(chan error, 1)
	go func() {
		_, writeErr := s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
		pongDone <- writeErr
	}()

	select {
	case err := <-pongDone:
		if err != nil {
			logger.Errorf("Failed to send pong: %v", err)
			return "", err
		}
	case <-time.After(FileTransferTimeout):
		logger.Errorf("Timeout sending pong acknowledgment after %v", FileTransferTimeout)
		return "", fmt.Errorf("timeout sending pong acknowledgment")
	}

	// Flush the stream to ensure pong is sent immediately
	if flusher, ok := s.(interface{ Flush() error }); ok {
		if flushErr := flusher.Flush(); flushErr != nil {
			logger.Errorf("Failed to flush pong response: %v", flushErr)
			return
		}
	}

	filePath = fileName
	dir := filepath.Dir(filePath)
	logger.Infof("Creating directory: %s", dir)
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			logger.Errorf("Failed to create directory %s: %v", dir, err)
			return
		}
	}

	logger.Infof("Creating file: %s", filePath)
	file, err := os.Create(filePath)
	if err != nil {
		logger.Errorf("Failed to create file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	logger.Infof("Starting file copy, expected size: %d bytes", finfo.Size)
	bar := progressbar.DefaultBytes(
		finfo.Size,
		"downloading",
	)
	bytesCopied, err := io.CopyN(io.MultiWriter(file, bar), s, finfo.Size)
	if err != nil {
		logger.Errorf("Failed to copy file data: %v", err)
		return
	}
	logger.Infof("Copied %d bytes to %s", bytesCopied, filePath)

	logger.Infof("Verifying file checksum...")
	checksum, checksumErr := FileChecksum(filePath)
	if checksumErr != nil {
		logger.Errorf("Failed to calculate checksum for downloaded file: %v", checksumErr)
		err = checksumErr
		return
	}
	logger.Infof("Downloaded file checksum: %s, expected: %s", checksum, finfo.Checksum)
	if checksum != finfo.Checksum {
		err = fmt.Errorf("downloaded file checksum %v mismatch with original %v", checksum, finfo.Checksum)
		logger.Errorf("Checksum mismatch: got %s, expected %s", checksum, finfo.Checksum)
		return
	}
	logger.Infof("Checksum verification successful for file %s", filePath)

	logger.Infof("Sending final pong acknowledgment...")
	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarPong)))
	if err != nil {
		logger.Errorf("Failed to send final pong: %v", err)
		return
	}

	logger.Infof("File download completed successfully: %s", filePath)

	return
}

func Upload(n *node.Node, peer peer.ID, filePath string, saveFilePath string) (err error) {
	s, err := n.Host.NewStream(n.CTX, peer, constant.StellarFileProtocol)
	if err != nil {
		return
	}
	defer s.Close()

	buf := bufio.NewReader(s)

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", constant.StellarFileSend)))
	if err != nil {
		return
	}

	// Set a timeout for reading the initial acknowledgment
	done := make(chan error, 1)
	var data string
	go func() {
		var readErr error
		data, readErr = buf.ReadString('\n')
		done <- readErr
	}()

	select {
	case err = <-done:
		if err != nil {
			return
		}
	case <-time.After(FileTransferTimeout):
		return fmt.Errorf("timeout waiting for initial acknowledgment")
	}
	data = strings.Trim(data, "\n")
	if data == constant.StellarEchoUnknownCommand {
		err = fmt.Errorf("file unknown command")
		return
	}
	if data != constant.StellarPong {
		err = fmt.Errorf("file send not receiving pong ack")
		return
	}

	_, err = s.Write([]byte(fmt.Sprintf("%v\n", filepath.ToSlash(saveFilePath))))
	if err != nil {
		return
	}

	err = upload(s, filePath)
	return
}

func Download(n *node.Node, peer peer.ID, fileName string, destPath string) (filePath string, err error) {
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

	// Set a timeout for reading the initial acknowledgment
	done := make(chan error, 1)
	var data string
	go func() {
		var readErr error
		data, readErr = buf.ReadString('\n')
		done <- readErr
	}()

	select {
	case err = <-done:
		if err != nil {
			return
		}
	case <-time.After(FileTransferTimeout):
		return "", fmt.Errorf("timeout waiting for initial acknowledgment")
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

	// Send the remote filename to request
	_, err = s.Write([]byte(fmt.Sprintf("%v\n", filepath.ToSlash(fileName))))
	if err != nil {
		return
	}

	// Save to the specified destination path
	filePath, err = download(s, destPath)
	return
}
