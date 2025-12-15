package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

func setupTCPConnection(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	var serverConn net.Conn
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		var err error
		serverConn, err = listener.Accept()
		require.NoError(t, err)
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	<-acceptDone
	require.NotNil(t, serverConn)
	return clientConn, serverConn
}

type runningServer struct {
	server    *Server
	client    *Client
	cancel    context.CancelFunc
	serveDone chan error
	serverDir string
}

func startServerAndClient(t *testing.T) *runningServer {
	clientConn, serverConn := setupTCPConnection(t)

	serverDir := t.TempDir()
	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	server := NewServer(serverConn, serverDir)
	require.NotNil(t, server)

	// Accept in background
	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

	// Give server a moment to start Accept
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, client.Connect())
	require.NoError(t, <-acceptErr)

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		_ = server.Close()
		select {
		case <-serveDone:
		case <-time.After(time.Second):
			t.Log("server serve did not exit before timeout")
		}
	})

	return &runningServer{
		server:    server,
		client:    client,
		cancel:    cancel,
		serveDone: serveDone,
		serverDir: serverDir,
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func waitForFileWithContent(t *testing.T, path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("file %s not written before timeout", path)
}

func fileChecksum(t *testing.T, path string) string {
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	return hex.EncodeToString(h.Sum(nil))
}

// Tests

func TestFileHandshake(t *testing.T) {
	rs := startServerAndClient(t)
	assert.True(t, rs.server.IsHandshakeDone())
	assert.Equal(t, "test-client", rs.server.ClientID())
}

func TestFileList(t *testing.T) {
	rs := startServerAndClient(t)

	writeFile(t, rs.serverDir, "a.txt", "hello")
	writeFile(t, rs.serverDir, "nested/b.txt", "world")

	entries, err := rs.client.List("/", false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 2)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Filename] = true
	}
	assert.True(t, names["a.txt"])
	assert.True(t, names["nested"])
}

func TestFileListRecursive(t *testing.T) {
	rs := startServerAndClient(t)

	writeFile(t, rs.serverDir, "dir1/dir2/c.txt", "x")

	entries, err := rs.client.List("/", true)
	require.NoError(t, err)

	found := false
	for _, e := range entries {
		if e.Filename == "dir1" && e.IsDir && len(e.Children) > 0 {
			found = true
		}
	}
	assert.True(t, found, "expected recursive children")
}

func TestFileListNotFound(t *testing.T) {
	rs := startServerAndClient(t)

	_, err := rs.client.List("/does-not-exist", false)
	assert.Error(t, err)
}

func TestFileGetSuccess(t *testing.T) {
	rs := startServerAndClient(t)

	srcContent := "download me"
	writeFile(t, rs.serverDir, "files/data.txt", srcContent)

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "out.txt")

	err := rs.client.Get("files/data.txt", destPath)
	require.NoError(t, err)

	bytes, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, string(bytes))
}

func TestFileGetNotFound(t *testing.T) {
	rs := startServerAndClient(t)

	err := rs.client.Get("missing.txt", filepath.Join(t.TempDir(), "out.txt"))
	assert.Error(t, err)
}

func TestFileSendSuccess(t *testing.T) {
	rs := startServerAndClient(t)

	srcDir := t.TempDir()
	srcPath := writeFile(t, srcDir, "upload.txt", "upload content")
	checksum := fileChecksum(t, srcPath)

	err := rs.client.Send(srcPath, "remote/upload.txt")
	require.NoError(t, err)

	dstPath := filepath.Join(rs.serverDir, "remote/upload.txt")
	waitForFileWithContent(t, dstPath, 500*time.Millisecond)
	bytes, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, "upload content", string(bytes))

	gotChecksum := fileChecksum(t, dstPath)
	assert.Equal(t, checksum, gotChecksum)
}

func TestFileSendCreatesDirectories(t *testing.T) {
	rs := startServerAndClient(t)

	srcDir := t.TempDir()
	srcPath := writeFile(t, srcDir, "upload.txt", "nested upload")

	err := rs.client.Send(srcPath, "deep/nested/path/file.txt")
	require.NoError(t, err)

	dstPath := filepath.Join(rs.serverDir, "deep/nested/path/file.txt")
	waitForFileWithContent(t, dstPath, 500*time.Millisecond)
	info, err := os.Stat(dstPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestFileSendMissingLocalFile(t *testing.T) {
	rs := startServerAndClient(t)

	err := rs.client.Send(filepath.Join(t.TempDir(), "no-file.txt"), "remote/missing.txt")
	assert.Error(t, err)
}

func TestFileListEmptyDirectory(t *testing.T) {
	rs := startServerAndClient(t)

	entries, err := rs.client.List("/", false)
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestFileSendOverwrite(t *testing.T) {
	rs := startServerAndClient(t)

	// Existing file on server
	dstPath := writeFile(t, rs.serverDir, "remote/overwrite.txt", "old")

	// Send new content
	srcDir := t.TempDir()
	srcPath := writeFile(t, srcDir, "upload.txt", "new content")
	err := rs.client.Send(srcPath, "remote/overwrite.txt")
	require.NoError(t, err)

	waitDeadline := time.Now().Add(500 * time.Millisecond)
	for {
		bytes, readErr := os.ReadFile(dstPath)
		if readErr == nil && string(bytes) == "new content" {
			break
		}
		if time.Now().After(waitDeadline) {
			require.NoError(t, readErr)
			assert.Equal(t, "new content", string(bytes))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFileSendConcurrent(t *testing.T) {
	rs := startServerAndClient(t)

	srcDir := t.TempDir()
	srcA := writeFile(t, srcDir, "a.txt", "content A")
	srcB := writeFile(t, srcDir, "b.txt", "content B")

	errCh := make(chan error, 2)
	go func() { errCh <- rs.client.Send(srcA, "concurrent/a.txt") }()
	go func() { errCh <- rs.client.Send(srcB, "concurrent/b.txt") }()

	for i := 0; i < 2; i++ {
		require.NoError(t, <-errCh)
	}

	waitForFileWithContent(t, filepath.Join(rs.serverDir, "concurrent/a.txt"), 500*time.Millisecond)
	waitForFileWithContent(t, filepath.Join(rs.serverDir, "concurrent/b.txt"), 500*time.Millisecond)

	dataA, err := os.ReadFile(filepath.Join(rs.serverDir, "concurrent/a.txt"))
	require.NoError(t, err)
	dataB, err := os.ReadFile(filepath.Join(rs.serverDir, "concurrent/b.txt"))
	require.NoError(t, err)

	contentSet := map[string]bool{
		string(dataA): true,
		string(dataB): true,
	}
	assert.True(t, contentSet["content A"])
	assert.True(t, contentSet["content B"])
}

func TestFileGetConcurrent(t *testing.T) {
	rs := startServerAndClient(t)

	// Seed files on server
	writeFile(t, rs.serverDir, "files/a.txt", "download A")
	writeFile(t, rs.serverDir, "files/b.txt", "download B")

	destDir := t.TempDir()
	require.NoError(t, rs.client.Get("files/a.txt", filepath.Join(destDir, "a.txt")))
	require.NoError(t, rs.client.Get("files/b.txt", filepath.Join(destDir, "b.txt")))

	bytesA, err := os.ReadFile(filepath.Join(destDir, "a.txt"))
	require.NoError(t, err)
	bytesB, err := os.ReadFile(filepath.Join(destDir, "b.txt"))
	require.NoError(t, err)

	assert.Equal(t, "download A", string(bytesA))
	assert.Equal(t, "download B", string(bytesB))
}

func TestFileSendDestNotWritable(t *testing.T) {
	rs := startServerAndClient(t)

	// Make server root unwritable
	require.NoError(t, os.Chmod(rs.serverDir, 0o500))
	defer os.Chmod(rs.serverDir, 0o755)

	srcDir := t.TempDir()
	srcPath := writeFile(t, srcDir, "upload.txt", "no write")

	err := rs.client.Send(srcPath, "blocked/file.txt")
	assert.Error(t, err)
}

func TestFileGetDestNotWritable(t *testing.T) {
	rs := startServerAndClient(t)

	writeFile(t, rs.serverDir, "files/protected.txt", "locked")

	destDir := t.TempDir()
	require.NoError(t, os.Chmod(destDir, 0o500))
	defer os.Chmod(destDir, 0o755)

	err := rs.client.Get("files/protected.txt", filepath.Join(destDir, "out.txt"))
	assert.Error(t, err)
}

func TestFileSendConnectionClosed(t *testing.T) {
	rs := startServerAndClient(t)

	srcDir := t.TempDir()
	srcPath := writeFile(t, srcDir, "upload.txt", "will fail")

	// Close server to simulate drop
	rs.cancel()
	_ = rs.server.Close()

	err := rs.client.Send(srcPath, "remote/file.txt")
	assert.Error(t, err)
}

func TestFileGetConnectionClosed(t *testing.T) {
	rs := startServerAndClient(t)
	writeFile(t, rs.serverDir, "files/x.txt", "data")

	destPath := filepath.Join(t.TempDir(), "x.txt")

	// Close server to simulate drop
	rs.cancel()
	_ = rs.server.Close()

	err := rs.client.Get("files/x.txt", destPath)
	assert.Error(t, err)
}
