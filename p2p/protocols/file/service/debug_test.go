package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDebugUploadIssue helps diagnose the upload issue
func TestDebugUploadIssue(t *testing.T) {
	rs := startServerAndClient(t)

	// Test with the problematic size
	size := int64(261463)

	// Generate test file
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "testfile.dat")
	file, err := os.Create(srcPath)
	require.NoError(t, err)

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	_, err = file.Write(data)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	// Upload file
	remotePath := "debug/testfile.dat"
	err = rs.client.Send(srcPath, remotePath)

	if err != nil {
		t.Logf("Upload error: %v", err)
	}

	// Wait a bit for file to be written
	time.Sleep(500 * time.Millisecond)

	dstPath := filepath.Join(rs.serverDir, remotePath)
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	t.Logf("Expected size: %d bytes", size)
	t.Logf("Actual size: %d bytes", info.Size())
	t.Logf("Difference: %d bytes", size-info.Size())
	t.Logf("Percentage received: %.2f%%", float64(info.Size())/float64(size)*100)

	if info.Size() != size {
		// Read the file to see what we got
		received, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		t.Logf("First 100 bytes of received file: %x", received[:min(100, len(received))])
		if len(received) < len(data) {
			t.Logf("Last 100 bytes of received file: %x", received[max(0, len(received)-100):])
			t.Logf("Expected bytes at position %d: %x", len(received), data[len(received):min(len(received)+100, len(data))])
		}
	}

	require.Equal(t, size, info.Size(), "File size mismatch")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
