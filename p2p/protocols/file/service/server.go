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

// Server handles server-side file operations
type Server struct {
	*base_service.BaseServer
	dataDir string
}

// NewServer creates a new file server with automatic multiplexer setup
func NewServer(conn io.ReadWriteCloser, dataDir string) *Server {
	if conn == nil {
		return nil
	}

	handshake := &fileHandshakeHandler{}
	dispatcher := &filePacketDispatcher{}

	base := base_service.NewBaseServer(conn, handshake, dispatcher)
	if base == nil {
		return nil
	}

	s := &Server{
		BaseServer: base,
		dataDir:    dataDir,
	}
	dispatcher.server = s
	return s
}

// Accept performs handshake and accepts a client connection
func (s *Server) Accept() error {
	return s.BaseServer.Accept()
}

// Serve runs the control-plane event loop until the context is canceled or an error occurs.
func (s *Server) Serve(ctx context.Context) error {
	return s.BaseServer.Serve(ctx)
}

func (s *Server) dispatchControlPacket(handshake *protocol.HandshakePacket) error {
	if handshake == nil {
		return fmt.Errorf("nil handshake packet")
	}

	switch handshake.Type {
	case protocol.HandshakeTypeFileList:
		return s.handleList(handshake)
	case protocol.HandshakeTypeFileGet:
		return s.handleGet(handshake)
	case protocol.HandshakeTypeFileSend:
		return s.handleSend(handshake)
	default:
		return fmt.Errorf("unknown type: %s", handshake.Type)
	}
}

func (s *Server) handleList(handshake *protocol.HandshakePacket) error {
	var request protocol.FileListRequest
	if err := handshake.UnmarshalPayload(&request); err != nil {
		return s.sendListError(fmt.Sprintf("invalid request: %v", err))
	}

	relativePath := request.Path
	relativePath = filepath.ToSlash(relativePath)
	if relativePath != "" && relativePath[0] == '/' {
		relativePath = relativePath[1:]
	}
	dirPath := filepath.Join(s.dataDir, relativePath)

	// Check if directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return s.sendListError(fmt.Sprintf("directory not found: %s", request.Path))
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return s.sendListError(fmt.Sprintf("failed to read directory: %v", err))
	}

	files := make([]protocol.FileEntry, 0)
	for _, e := range entries {
		isDir := e.IsDir()
		size := int64(0)
		if !isDir {
			fi, fiErr := os.Stat(filepath.Join(dirPath, e.Name()))
			if fiErr != nil {
				continue
			}
			size = fi.Size()
		}

		displayPath := relativePath
		if displayPath == "" {
			displayPath = "/"
		}

		fileEntry := protocol.FileEntry{
			DirectoryName: displayPath,
			Filename:      e.Name(),
			Size:          size,
			IsDir:         isDir,
		}

		// If recursive, populate children
		if request.Recursive && isDir {
			children, err := s.listRecursive(filepath.Join(relativePath, e.Name()))
			if err == nil {
				fileEntry.Children = children
			}
		}

		files = append(files, fileEntry)
	}

	return s.sendListSuccess(files)
}

func (s *Server) listRecursive(relativePath string) ([]protocol.FileEntry, error) {
	dirPath := filepath.Join(s.dataDir, relativePath)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	files := make([]protocol.FileEntry, 0)
	for _, e := range entries {
		isDir := e.IsDir()
		size := int64(0)
		if !isDir {
			fi, fiErr := os.Stat(filepath.Join(dirPath, e.Name()))
			if fiErr != nil {
				continue
			}
			size = fi.Size()
		}

		fileEntry := protocol.FileEntry{
			DirectoryName: relativePath,
			Filename:      e.Name(),
			Size:          size,
			IsDir:         isDir,
		}

		if isDir {
			children, err := s.listRecursive(filepath.Join(relativePath, e.Name()))
			if err == nil {
				fileEntry.Children = children
			}
		}

		files = append(files, fileEntry)
	}

	return files, nil
}

func (s *Server) handleGet(handshake *protocol.HandshakePacket) error {
	var request protocol.FileGetRequest
	if err := handshake.UnmarshalPayload(&request); err != nil {
		return s.sendGetError(fmt.Sprintf("invalid request: %v", err))
	}

	fileName := filepath.ToSlash(request.FileName)
	filePath := filepath.Join(s.dataDir, fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return s.sendGetError(fmt.Sprintf("file not found: %s", request.FileName))
	}

	// Get file info
	fi, err := os.Stat(filePath)
	if err != nil {
		return s.sendGetError(fmt.Sprintf("failed to stat file: %v", err))
	}

	// Calculate checksum
	checksum, err := s.calculateChecksum(filePath)
	if err != nil {
		return s.sendGetError(fmt.Sprintf("failed to calculate checksum: %v", err))
	}

	// Allocate data stream
	streamID := s.NextStreamID()
	stream, err := s.Multiplexer().OpenStreamWithID(streamID)
	if err != nil {
		return s.sendGetError(fmt.Sprintf("failed to open data stream: %v", err))
	}

	fileInfo := &protocol.FileInfo{
		Filename: fileName,
		Size:     fi.Size(),
		Checksum: checksum,
	}

	// Send success response with stream ID and file info
	if err := s.sendGetSuccess(streamID, fileInfo); err != nil {
		stream.Close()
		return err
	}

	// Start file transfer in goroutine
	go s.transferFile(stream, filePath, fi.Size())

	return nil
}

func (s *Server) handleSend(handshake *protocol.HandshakePacket) error {
	var request protocol.FileSendRequest
	if err := handshake.UnmarshalPayload(&request); err != nil {
		return s.sendSendError(fmt.Sprintf("invalid request: %v", err))
	}

	fileName := filepath.ToSlash(request.FileName)
	filePath := filepath.Join(s.dataDir, fileName)

	// Ensure directory exists
	dir := filepath.Dir(filePath)

	// Check if directory exists and is writable before trying to create it
	// This catches cases where the parent directory is not writable
	// We need to check if we can create the directory, not just if it exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return s.sendSendError(fmt.Sprintf("failed to create directory: %v", err))
	}

	// Check if directory is writable by attempting to create a test file
	// This ensures we can actually write the file before accepting the transfer
	// This check is critical because os.MkdirAll can succeed even if the parent
	// directory is not writable (if the directory already exists)
	testFile := filepath.Join(dir, ".stellar_write_test")
	testF, err := os.Create(testFile)
	if err != nil {
		return s.sendSendError(fmt.Sprintf("directory not writable: %v", err))
	}
	testF.Close()
	if err := os.Remove(testFile); err != nil {
		// Log but don't fail - cleanup error is not critical
		// The important check (writability) already passed
	}

	// Allocate data stream
	streamID := s.NextStreamID()
	stream, err := s.Multiplexer().OpenStreamWithID(streamID)
	if err != nil {
		return s.sendSendError(fmt.Sprintf("failed to open data stream: %v", err))
	}

	// Send success response with stream ID
	if err := s.sendSendSuccess(streamID); err != nil {
		stream.Close()
		return err
	}

	// Start file receive in goroutine
	go s.receiveFile(stream, filePath)

	return nil
}

func (s *Server) transferFile(stream io.ReadWriteCloser, filePath string, expectedSize int64) {
	defer stream.Close()

	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Use io.Copy with context cancellation
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(stream, file)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			return
		}
	case <-ctx.Done():
		return
	}
}

func (s *Server) receiveFile(stream io.ReadWriteCloser, filePath string) {
	// CRITICAL: Don't close the stream immediately on return - the client might still be writing.
	// Instead, we'll close it only after we're sure the transfer is complete or failed.
	// This prevents "stream reset (remote)" errors when the client is still writing data.
	// For libp2p streams, closing the stream resets it, which causes errors on the client side.
	defer stream.Close()

	// Read file info from data stream (JSON, newline-terminated)
	// Use a buffered reader to read until newline
	type result struct {
		fileInfo protocol.FileInfo
		err      error
	}

	infoChan := make(chan result, 1)
	go func() {
		// Read until newline to get file info JSON
		var fileInfoJSON []byte
		buf := make([]byte, 1)
		for {
			n, err := stream.Read(buf)
			if err != nil {
				// Only report error if we haven't read any data yet
				// If we've read some data but hit EOF before newline, that's an error
				if err == io.EOF && len(fileInfoJSON) == 0 {
					// EOF before any data - this is a real error
					infoChan <- result{err: fmt.Errorf("EOF before file info: %w", err)}
					return
				} else if err != nil && err != io.EOF {
					// Non-EOF error is always a problem
					infoChan <- result{err: fmt.Errorf("read error: %w", err)}
					return
				} else if err == io.EOF {
					// EOF after reading some data but no newline - incomplete file info
					infoChan <- result{err: fmt.Errorf("EOF before newline in file info (read %d bytes)", len(fileInfoJSON))}
					return
				}
			}
			if n == 0 {
				continue
			}
			if buf[0] == '\n' {
				break
			}
			fileInfoJSON = append(fileInfoJSON, buf[0])
		}

		var fileInfo protocol.FileInfo
		if err := json.Unmarshal(fileInfoJSON, &fileInfo); err != nil {
			infoChan <- result{err: fmt.Errorf("unmarshal file info: %w", err)}
			return
		}
		infoChan <- result{fileInfo: fileInfo, err: nil}
	}()

	// Wait for file info with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var fileInfo protocol.FileInfo
	select {
	case res := <-infoChan:
		if res.err != nil {
			return
		}
		fileInfo = res.fileInfo
	case <-ctx.Done():
		return
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	// Copy file data
	transferCtx, transferCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer transferCancel()

	done := make(chan error, 1)
	go func() {
		bytesRead, err := io.CopyN(file, stream, fileInfo.Size)
		// io.CopyN returns EOF if it reads fewer bytes than requested
		// This is an error condition - we need all the data
		if err != nil {
			if err == io.EOF && bytesRead != fileInfo.Size {
				done <- fmt.Errorf("unexpected EOF: got %d bytes, expected %d", bytesRead, fileInfo.Size)
			} else {
				done <- fmt.Errorf("copy failed after %d bytes (expected %d): %w", bytesRead, fileInfo.Size, err)
			}
			return
		}
		if bytesRead != fileInfo.Size {
			done <- fmt.Errorf("partial read: got %d bytes, expected %d", bytesRead, fileInfo.Size)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			// Any error (including EOF with partial read) means the transfer failed
			// CRITICAL: Don't close the stream immediately - the client might still be writing
			// Wait a bit to allow the client to finish writing before closing the stream
			// This prevents "stream reset (remote)" errors when using libp2p streams
			time.Sleep(200 * time.Millisecond)
			os.Remove(filePath)
			return
		}
	case <-transferCtx.Done():
		// Timeout - wait a bit before closing to allow client to finish
		time.Sleep(200 * time.Millisecond)
		os.Remove(filePath)
		return
	}

	// Verify checksum
	checksum, err := s.calculateChecksum(filePath)
	if err != nil {
		os.Remove(filePath)
		return
	}

	if checksum != fileInfo.Checksum {
		// Remove file on checksum mismatch
		os.Remove(filePath)
		return
	}
}

func (s *Server) calculateChecksum(filePath string) (string, error) {
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

func (s *Server) sendListSuccess(files []protocol.FileEntry) error {
	packet, err := protocol.NewFileListResponsePacket(true, files, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendListError(errMsg string) error {
	packet, err := protocol.NewFileListResponsePacket(false, nil, errMsg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendGetSuccess(streamID uint32, fileInfo *protocol.FileInfo) error {
	packet, err := protocol.NewFileGetResponsePacket(true, streamID, fileInfo, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendGetError(errMsg string) error {
	packet, err := protocol.NewFileGetResponsePacket(false, 0, nil, errMsg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendSendSuccess(streamID uint32) error {
	packet, err := protocol.NewFileSendResponsePacket(true, streamID, "")
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

func (s *Server) sendSendError(errMsg string) error {
	packet, err := protocol.NewFileSendResponsePacket(false, 0, errMsg)
	if err != nil {
		return err
	}
	return s.ControlConn().WritePacket(packet)
}

// BaseServer provides Close/handshake helpers; no extra methods needed here.
