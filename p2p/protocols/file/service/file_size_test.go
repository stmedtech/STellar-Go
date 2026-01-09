package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test file sizes around the problematic threshold
var testSizes = []int64{
	100,     // Small file
	1024,    // 1KB
	4096,    // 4KB
	32768,   // 32KB
	65536,   // 64KB
	131072,  // 128KB
	256000,  // ~250KB
	261460,  // Just below threshold
	261461,  // Just below threshold
	261462,  // Exact threshold (reported working)
	261463,  // Just above threshold (reported failing)
	261464,  // Just above threshold
	262144,  // 256KB (2^18)
	300000,  // ~300KB
	524288,  // 512KB
	1048576, // 1MB
	2097152, // 2MB
	5242880, // 5MB
}

// generateTestFile creates a file with the specified size filled with deterministic data
func generateTestFile(t *testing.T, size int64) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile.dat")

	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	// Write deterministic data (pattern based on byte position)
	buf := make([]byte, 8192) // 8KB buffer
	for written := int64(0); written < size; {
		remaining := size - written
		if remaining > int64(len(buf)) {
			remaining = int64(len(buf))
		}

		// Fill buffer with deterministic pattern
		for i := int64(0); i < remaining; i++ {
			buf[i] = byte((written + i) % 256)
		}

		n, err := file.Write(buf[:remaining])
		require.NoError(t, err)
		written += int64(n)
	}

	require.NoError(t, file.Sync())
	return path
}

// calculateFileChecksum calculates SHA256 checksum of a file
func calculateFileChecksum(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	require.NoError(t, err)

	return hex.EncodeToString(hash.Sum(nil))
}

// verifyFileContent verifies that a file has the expected size and checksum
func verifyFileContent(t *testing.T, path string, expectedSize int64, expectedChecksum string) {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, expectedSize, info.Size(), "file size mismatch")

	actualChecksum := calculateFileChecksum(t, path)
	assert.Equal(t, expectedChecksum, actualChecksum, "file checksum mismatch")
}

// TestFileSendVariousSizes tests upload (Send) with various file sizes
func TestFileSendVariousSizes(t *testing.T) {
	rs := startServerAndClient(t)

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Send_%d_bytes", size), func(t *testing.T) {
			// Generate test file
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			// Upload file
			remotePath := filepath.Join("test", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Send(srcPath, remotePath)
			require.NoError(t, err, "Send failed for size %d", size)

			// Wait for file to be written with correct size
			dstPath := filepath.Join(rs.serverDir, remotePath)
			waitForFileWithExactSize(t, dstPath, size, 5*time.Second)

			// Verify file
			verifyFileContent(t, dstPath, size, expectedChecksum)
		})
	}
}

// TestFileGetVariousSizes tests download (Get) with various file sizes
func TestFileGetVariousSizes(t *testing.T) {
	rs := startServerAndClient(t)

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Get_%d_bytes", size), func(t *testing.T) {
			// Generate test file on server
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			// Copy to server directory
			serverPath := filepath.Join(rs.serverDir, "test", fmt.Sprintf("file_%d.dat", size))
			require.NoError(t, os.MkdirAll(filepath.Dir(serverPath), 0755))
			require.NoError(t, copyFile(srcPath, serverPath))

			// Download file
			destDir := t.TempDir()
			destPath := filepath.Join(destDir, fmt.Sprintf("downloaded_%d.dat", size))
			remotePath := filepath.Join("test", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Get(remotePath, destPath)
			require.NoError(t, err, "Get failed for size %d", size)

			// Verify file
			verifyFileContent(t, destPath, size, expectedChecksum)
		})
	}
}

// TestFileSendThresholdBoundary tests the exact threshold where issues occur
func TestFileSendThresholdBoundary(t *testing.T) {
	rs := startServerAndClient(t)

	// Test sizes around the reported threshold
	thresholdSizes := []int64{
		261460, // Just below
		261461, // Just below
		261462, // Exact threshold (reported working)
		261463, // Just above (reported failing)
		261464, // Just above
		261465, // Just above
	}

	for _, size := range thresholdSizes {
		t.Run(fmt.Sprintf("Threshold_%d_bytes", size), func(t *testing.T) {
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			remotePath := filepath.Join("threshold", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Send(srcPath, remotePath)

			if err != nil {
				t.Logf("Send failed for size %d: %v", size, err)
				// This is the problematic case - log details
				t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
			}
			require.NoError(t, err, "Send failed for size %d", size)

			dstPath := filepath.Join(rs.serverDir, remotePath)
			// Wait for file with exact size - this ensures the transfer completes
			waitForFileWithExactSize(t, dstPath, size, 10*time.Second)
			verifyFileContent(t, dstPath, size, expectedChecksum)
		})
	}
}

// TestFileGetThresholdBoundary tests download at the threshold
func TestFileGetThresholdBoundary(t *testing.T) {
	rs := startServerAndClient(t)

	thresholdSizes := []int64{
		261460,
		261461,
		261462,
		261463,
		261464,
		261465,
	}

	for _, size := range thresholdSizes {
		t.Run(fmt.Sprintf("Threshold_%d_bytes", size), func(t *testing.T) {
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			serverPath := filepath.Join(rs.serverDir, "threshold", fmt.Sprintf("file_%d.dat", size))
			require.NoError(t, os.MkdirAll(filepath.Dir(serverPath), 0755))
			require.NoError(t, copyFile(srcPath, serverPath))

			destDir := t.TempDir()
			destPath := filepath.Join(destDir, fmt.Sprintf("downloaded_%d.dat", size))
			remotePath := filepath.Join("threshold", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Get(remotePath, destPath)

			if err != nil {
				t.Logf("Get failed for size %d: %v", size, err)
				t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
			}
			require.NoError(t, err, "Get failed for size %d", size)

			verifyFileContent(t, destPath, size, expectedChecksum)
		})
	}
}

// TestFileSendLargeFiles tests with progressively larger files
func TestFileSendLargeFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	rs := startServerAndClient(t)

	largeSizes := []int64{
		1024 * 1024,      // 1MB
		5 * 1024 * 1024,  // 5MB
		10 * 1024 * 1024, // 10MB
		50 * 1024 * 1024, // 50MB
	}

	for _, size := range largeSizes {
		t.Run(fmt.Sprintf("Large_%d_bytes", size), func(t *testing.T) {
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			remotePath := filepath.Join("large", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Send(srcPath, remotePath)
			require.NoError(t, err, "Send failed for size %d", size)

			dstPath := filepath.Join(rs.serverDir, remotePath)
			// Wait for file with exact size - large files need more time
			waitForFileWithExactSize(t, dstPath, size, 60*time.Second)
			verifyFileContent(t, dstPath, size, expectedChecksum)
		})
	}
}

// TestFileSendConcurrentVariousSizes tests concurrent uploads of different sizes
func TestFileSendConcurrentVariousSizes(t *testing.T) {
	rs := startServerAndClient(t)

	// Test with sizes around the threshold
	sizes := []int64{261460, 261462, 261463, 261464}

	errCh := make(chan error, len(sizes))
	for i, size := range sizes {
		go func(idx int, sz int64) {
			srcPath := generateTestFile(t, sz)
			remotePath := filepath.Join("concurrent", fmt.Sprintf("file_%d_%d.dat", idx, sz))
			errCh <- rs.client.Send(srcPath, remotePath)
		}(i, size)
	}

	for i := 0; i < len(sizes); i++ {
		err := <-errCh
		require.NoError(t, err, "Concurrent send %d failed", i)
	}
}

// waitForFileWithExactSize waits for a file to exist and have the exact expected size
func waitForFileWithExactSize(t *testing.T, path string, expectedSize int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Size() == expectedSize {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Check one more time to get the actual size for error message
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file %s not created before timeout: %v", path, err)
	}
	t.Fatalf("file %s has wrong size: got %d, expected %d", path, info.Size(), expectedSize)
}

// Helper function to copy file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// TestMultiplexerWriteDataDirectly tests the multiplexer writeData function directly
func TestMultiplexerWriteDataDirectly(t *testing.T) {
	c1, c2 := net.Pipe()

	// Create multiplexers
	// This test will help identify if the issue is in the multiplexer layer
	// We'll need to import the multiplexer package for this

	// For now, test through the file service
	rs := startServerAndClientWithConn(t, c1, c2)

	// Test sizes that might trigger buffer issues
	testSizes := []int64{
		256 * 1024, // 256KB
		261462,     // Threshold
		261463,     // Above threshold
		512 * 1024, // 512KB
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Direct_%d_bytes", size), func(t *testing.T) {
			srcPath := generateTestFile(t, size)
			expectedChecksum := calculateFileChecksum(t, srcPath)

			remotePath := filepath.Join("direct", fmt.Sprintf("file_%d.dat", size))
			err := rs.client.Send(srcPath, remotePath)

			if err != nil {
				t.Logf("Direct test failed for size %d: %v", size, err)
				t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
			}
			require.NoError(t, err, "Direct test failed for size %d", size)

			dstPath := filepath.Join(rs.serverDir, remotePath)
			// Wait for file with exact size - this ensures the transfer completes
			waitForFileWithExactSize(t, dstPath, size, 10*time.Second)
			verifyFileContent(t, dstPath, size, expectedChecksum)
		})
	}
}

// startServerAndClientWithConn is a helper that uses provided connections
func startServerAndClientWithConn(t *testing.T, clientConn, serverConn net.Conn) *runningServer {
	t.Helper()

	serverDir := t.TempDir()
	client := NewClient("test-client", clientConn)
	require.NotNil(t, client)

	server := NewServer(serverConn, serverDir)
	require.NotNil(t, server)

	acceptErr := make(chan error, 1)
	go func() {
		acceptErr <- server.Accept()
	}()

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
