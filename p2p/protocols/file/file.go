package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"stellar/p2p/constant"
	"stellar/p2p/node"
	"stellar/p2p/protocols/common/protocol"
	"stellar/p2p/protocols/file/service"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
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

// fileStreamHandler handles incoming libp2p streams for the file protocol.
// This is the transport layer adapter that bridges libp2p streams to the service layer.
func fileStreamHandler(s network.Stream) {
	defer s.Close()

	srv := service.NewServer(s, DataDir)
	if srv == nil {
		logger.Warn("Failed to create file server")
		return
	}

	if err := srv.Accept(); err != nil {
		logger.Warnf("File handshake failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.Serve(ctx)
	}()

	select {
	case <-serveDone:
		srv.Close()
	case <-ctx.Done():
		srv.Close()
	}
}

func BindFileStream(n *node.Node) {
	n.Host.SetStreamHandler(
		constant.StellarFileProtocol,
		n.Policy.AuthorizeStream(fileStreamHandler),
	)
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

// protocolFileEntryToFileEntry converts protocol.FileEntry to FileEntry
func protocolFileEntryToFileEntry(pf protocol.FileEntry) FileEntry {
	fe := FileEntry{
		DirectoryName: pf.DirectoryName,
		Filename:      pf.Filename,
		Size:          pf.Size,
		IsDir:         pf.IsDir,
	}

	if len(pf.Children) > 0 {
		fe.Children = make([]FileEntry, len(pf.Children))
		for i, child := range pf.Children {
			fe.Children[i] = protocolFileEntryToFileEntry(child)
		}
	}

	return fe
}

func List(n *node.Node, peer peer.ID, relativePath string) (files []FileEntry, err error) {
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarFileProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarFileProtocol)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", peer, err)
	}
	defer s.Close()

	client := service.NewClient(n.Host.ID().String(), s)
	if client == nil {
		return nil, fmt.Errorf("failed to create file client")
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	protocolFiles, err := client.List(relativePath, false)
	if err != nil {
		return nil, err
	}

	// Convert protocol.FileEntry to FileEntry
	files = make([]FileEntry, len(protocolFiles))
	for i, pf := range protocolFiles {
		files[i] = protocolFileEntryToFileEntry(pf)
	}

	return files, nil
}

func ListFullTree(n *node.Node, peer peer.ID) (files []FileEntry, err error) {
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarFileProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarFileProtocol)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", peer, err)
	}
	defer s.Close()

	client := service.NewClient(n.Host.ID().String(), s)
	if client == nil {
		return nil, fmt.Errorf("failed to create file client")
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	protocolFiles, err := client.List("/", true)
	if err != nil {
		return nil, err
	}

	// Convert protocol.FileEntry to FileEntry
	files = make([]FileEntry, len(protocolFiles))
	for i, pf := range protocolFiles {
		files[i] = protocolFileEntryToFileEntry(pf)
	}

	return files, nil
}

// upload and download are deprecated - use service layer instead
// Kept for reference but no longer used

func Upload(n *node.Node, peer peer.ID, filePath string, saveFilePath string) (err error) {
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarFileProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarFileProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	client := service.NewClient(n.Host.ID().String(), s)
	if client == nil {
		return fmt.Errorf("failed to create file client")
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	return client.Send(filePath, saveFilePath)
}

func Download(n *node.Node, peer peer.ID, fileName string, destPath string) (filePath string, err error) {
	allowCtx := network.WithAllowLimitedConn(n.CTX, string(constant.StellarFileProtocol))
	s, err := n.Host.NewStream(allowCtx, peer, constant.StellarFileProtocol)
	if err != nil {
		return "", err
	}
	defer s.Close()

	client := service.NewClient(n.Host.ID().String(), s)
	if client == nil {
		return "", fmt.Errorf("failed to create file client")
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}

	if err := client.Get(fileName, destPath); err != nil {
		return "", err
	}

	return destPath, nil
}
