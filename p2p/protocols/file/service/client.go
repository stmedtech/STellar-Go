package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"stellar/p2p/protocols/common/protocol"
	base_service "stellar/p2p/protocols/common/service"
)

// Client handles client-side file operations
type Client struct {
	*base_service.BaseClient
}

// NewClient creates a new file client with automatic multiplexer setup
func NewClient(clientID string, conn io.ReadWriteCloser) *Client {
	if conn == nil || clientID == "" {
		return nil
	}

	hello := &fileHelloHandler{}
	base := base_service.NewBaseClient(clientID, conn, hello, nil)
	if base == nil {
		return nil
	}

	return &Client{BaseClient: base}
}

// Connect performs handshake and establishes connection
func (c *Client) Connect() error {
	return c.BaseClient.Connect()
}

// List requests a file/directory listing
func (c *Client) List(path string, recursive bool) ([]protocol.FileEntry, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}

	requestPacket, err := protocol.NewFileListPacket(path, recursive)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	response, err := c.SendRequest(context.Background(), requestPacket, matchFileListResponse())
	if err != nil {
		return nil, err
	}

	var listResponse protocol.FileListResponse
	if err := response.UnmarshalPayload(&listResponse); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !listResponse.Success {
		return nil, fmt.Errorf("list failed: %s", listResponse.Error)
	}

	return listResponse.Files, nil
}

// Get requests a file download
func (c *Client) Get(fileName string, destPath string) error {
	if err := c.EnsureConnected(); err != nil {
		return err
	}

	requestPacket, err := protocol.NewFileGetPacket(fileName)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	response, err := c.SendRequest(context.Background(), requestPacket, matchFileGetResponse())
	if err != nil {
		return err
	}

	var getResponse protocol.FileGetResponse
	if err := response.UnmarshalPayload(&getResponse); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if !getResponse.Success {
		return fmt.Errorf("get failed: %s", getResponse.Error)
	}

	if getResponse.FileInfo == nil {
		return fmt.Errorf("missing file info in response")
	}

	// Get or create data stream
	stream, err := c.GetOrCreateStream(getResponse.StreamID)
	if err != nil {
		return fmt.Errorf("get stream: %w", err)
	}
	defer stream.Close()

	// Ensure destination directory exists
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create file
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	// Copy file data
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := io.CopyN(file, stream, getResponse.FileInfo.Size)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			os.Remove(destPath)
			return fmt.Errorf("copy file: %w", err)
		}
	case <-ctx.Done():
		os.Remove(destPath)
		return fmt.Errorf("timeout")
	}

	// Verify checksum
	checksum, err := calculateChecksum(destPath)
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("calculate checksum: %w", err)
	}

	if checksum != getResponse.FileInfo.Checksum {
		os.Remove(destPath)
		return fmt.Errorf("checksum mismatch: got %s, expected %s", checksum, getResponse.FileInfo.Checksum)
	}

	return nil
}

// Send requests a file upload
func (c *Client) Send(filePath string, remoteFileName string) error {
	if err := c.EnsureConnected(); err != nil {
		return err
	}

	// Get file info first
	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	checksum, err := calculateChecksum(filePath)
	if err != nil {
		return fmt.Errorf("calculate checksum: %w", err)
	}

	fileInfo := &protocol.FileInfo{
		Filename: remoteFileName,
		Size:     fi.Size(),
		Checksum: checksum,
	}

	// Send request
	requestPacket, err := protocol.NewFileSendPacket(remoteFileName)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	response, err := c.SendRequest(context.Background(), requestPacket, matchFileSendResponse())
	if err != nil {
		return err
	}

	var sendResponse protocol.FileSendResponse
	if err := response.UnmarshalPayload(&sendResponse); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if !sendResponse.Success {
		return fmt.Errorf("send failed: %s", sendResponse.Error)
	}

	// Get or create data stream
	stream, err := c.GetOrCreateStream(sendResponse.StreamID)
	if err != nil {
		return fmt.Errorf("get stream: %w", err)
	}
	defer stream.Close()

	// Send file info first (JSON, newline-terminated)
	fileInfoJSON, err := json.Marshal(fileInfo)
	if err != nil {
		return fmt.Errorf("marshal file info: %w", err)
	}

	if _, err := stream.Write(append(fileInfoJSON, '\n')); err != nil {
		return fmt.Errorf("write file info: %w", err)
	}

	// Open file and copy data
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	done := make(chan error, 1)
	var bytesWritten int64
	go func() {
		var err error
		bytesWritten, err = io.Copy(stream, file)
		if err != nil {
			done <- fmt.Errorf("io.Copy error after %d bytes: %w", bytesWritten, err)
			return
		}
		// Verify we wrote all data
		fileInfo, statErr := os.Stat(filePath)
		if statErr == nil && bytesWritten != fileInfo.Size() {
			done <- fmt.Errorf("partial write: wrote %d bytes, expected %d", bytesWritten, fileInfo.Size())
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return fmt.Errorf("timeout after writing %d bytes", bytesWritten)
	}

	// CRITICAL FIX: Wait a moment before closing the stream to ensure all data
	// has been flushed to the network. This is especially important for libp2p
	// streams where closing the stream resets it, causing "stream reset (remote)"
	// errors if the server is still reading.
	// The issue occurs because io.Copy may return before all data is actually
	// transmitted over the network (it's only written to the socket buffer).
	// On Linux, TCP socket buffers can be large, and data may not be immediately
	// transmitted. By waiting briefly, we allow the data to be transmitted before
	// sending the EOF marker. The delay is proportional to the amount of data written
	// to account for larger files taking longer to transmit.
	// For files around 229KB, a 200ms delay should be sufficient for most network
	// conditions, but we scale it based on the amount of data.
	delay := time.Duration(bytesWritten/1024) * time.Millisecond // 1ms per KB
	if delay < 100*time.Millisecond {
		delay = 100 * time.Millisecond
	}
	if delay > 500*time.Millisecond {
		delay = 500 * time.Millisecond
	}
	time.Sleep(delay)

	return nil
}

// Helper types and functions

type matcher = base_service.Matcher

func matchFileListResponse() matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		return packet.Type == protocol.HandshakeTypeFileListResponse, nil
	}
}

func matchFileGetResponse() matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		return packet.Type == protocol.HandshakeTypeFileGetResponse, nil
	}
}

func matchFileSendResponse() matcher {
	return func(packet *protocol.HandshakePacket) (bool, error) {
		return packet.Type == protocol.HandshakeTypeFileSendResponse, nil
	}
}

func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
