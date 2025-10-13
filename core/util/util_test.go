package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	// Create a test source file
	srcContent := "Hello, World! This is test content for file copying."
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Create destination path
	dstFile := filepath.Join(t.TempDir(), "destination.txt")

	// Test successful copy
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Verify destination file exists and has correct content
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, srcContent, string(dstContent))
}

func TestCopyFileSourceNotFound(t *testing.T) {
	// Test copying non-existent source file
	dstFile := filepath.Join(t.TempDir(), "destination.txt")

	bytesCopied, err := CopyFile("/non/existent/source.txt", dstFile)
	assert.Error(t, err)
	assert.Equal(t, int64(0), bytesCopied)
	assert.Contains(t, err.Error(), "failed to open source file")
}

func TestCopyFileDestinationError(t *testing.T) {
	// Create a test source file
	srcContent := "Test content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Test copying to invalid destination (directory that doesn't exist)
	dstFile := "/non/existent/directory/destination.txt"

	bytesCopied, err := CopyFile(srcFile, dstFile)
	assert.Error(t, err)
	assert.Equal(t, int64(0), bytesCopied)
	assert.Contains(t, err.Error(), "failed to create destination file")
}

func TestCopyFileLargeFile(t *testing.T) {
	// Create a larger test file
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	srcFile := filepath.Join(t.TempDir(), "large_source.txt")
	err := os.WriteFile(srcFile, largeContent, 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "large_destination.txt")

	// Test copying large file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(largeContent)), bytesCopied)

	// Verify destination file has correct content
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, largeContent, dstContent)
}

func TestCopyFileEmptyFile(t *testing.T) {
	// Create an empty source file
	srcFile := filepath.Join(t.TempDir(), "empty_source.txt")
	err := os.WriteFile(srcFile, []byte{}, 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "empty_destination.txt")

	// Test copying empty file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytesCopied)

	// Verify destination file exists and is empty
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Empty(t, dstContent)
}

func TestCopyFileOverwriteExisting(t *testing.T) {
	// Create a test source file
	srcContent := "New content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Create an existing destination file with different content
	existingContent := "Old content that should be overwritten"
	dstFile := filepath.Join(t.TempDir(), "destination.txt")
	err = os.WriteFile(dstFile, []byte(existingContent), 0644)
	require.NoError(t, err)

	// Test copying over existing file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Verify destination file has new content
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, srcContent, string(dstContent))
}

func TestCopyFileSourceClosed(t *testing.T) {
	// Create a test source file
	srcContent := "Test content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "destination.txt")

	// Test that source file is properly closed after copy
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Try to delete the source file to verify it's not locked
	err = os.Remove(srcFile)
	assert.NoError(t, err)
}

func TestCopyFileDestinationClosed(t *testing.T) {
	// Create a test source file
	srcContent := "Test content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "destination.txt")

	// Test that destination file is properly closed after copy
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Try to delete the destination file to verify it's not locked
	err = os.Remove(dstFile)
	assert.NoError(t, err)
}

func TestCopyFileBinaryData(t *testing.T) {
	// Create a test file with binary data
	binaryData := make([]byte, 256)
	for i := range binaryData {
		binaryData[i] = byte(i)
	}

	srcFile := filepath.Join(t.TempDir(), "binary_source.bin")
	err := os.WriteFile(srcFile, binaryData, 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "binary_destination.bin")

	// Test copying binary file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(binaryData)), bytesCopied)

	// Verify destination file has correct binary content
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, binaryData, dstContent)
}

func TestCopyFileUnicodeContent(t *testing.T) {
	// Create a test file with Unicode content
	unicodeContent := "Hello, 世界! 🌍 This is Unicode content: ñáéíóú"
	srcFile := filepath.Join(t.TempDir(), "unicode_source.txt")
	err := os.WriteFile(srcFile, []byte(unicodeContent), 0644)
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "unicode_destination.txt")

	// Test copying Unicode file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(unicodeContent)), bytesCopied)

	// Verify destination file has correct Unicode content
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, unicodeContent, string(dstContent))
}

func TestCopyFilePermissions(t *testing.T) {
	// Create a test source file
	srcContent := "Test content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0755) // Set executable permissions
	require.NoError(t, err)

	dstFile := filepath.Join(t.TempDir(), "destination.txt")

	// Test copying file
	bytesCopied, err := CopyFile(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Verify destination file exists
	assert.FileExists(t, dstFile)

	// Note: CopyFile doesn't preserve permissions, so we just verify the file exists
	// The destination file will have default permissions (0644)
}

func TestCopyFileSameSourceDestination(t *testing.T) {
	// Create a test file
	srcContent := "Test content"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Test copying file to itself (should work but overwrite)
	bytesCopied, err := CopyFile(srcFile, srcFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(srcContent)), bytesCopied)

	// Verify file still exists and has content
	dstContent, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, srcContent, string(dstContent))
}

func TestCopyFileConcurrentAccess(t *testing.T) {
	// Create a test source file
	srcContent := "Test content for concurrent access"
	srcFile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Test multiple concurrent copies
	results := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(index int) {
			dstFile := filepath.Join(t.TempDir(), "destination.txt")
			_, err := CopyFile(srcFile, dstFile)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < 3; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent copy should succeed")
	}
}

// Benchmark tests
func BenchmarkCopyFile(b *testing.B) {
	// Create a test file for benchmarking
	content := make([]byte, 1024*100) // 100KB
	for i := range content {
		content[i] = byte(i % 256)
	}

	srcFile := filepath.Join(b.TempDir(), "benchmark_source.txt")
	err := os.WriteFile(srcFile, content, 0644)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dstFile := filepath.Join(b.TempDir(), "benchmark_destination.txt")
		_, err := CopyFile(srcFile, dstFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCopyFileLarge(b *testing.B) {
	// Create a larger test file for benchmarking
	content := make([]byte, 1024*1024) // 1MB
	for i := range content {
		content[i] = byte(i % 256)
	}

	srcFile := filepath.Join(b.TempDir(), "benchmark_large_source.txt")
	err := os.WriteFile(srcFile, content, 0644)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dstFile := filepath.Join(b.TempDir(), "benchmark_large_destination.txt")
		_, err := CopyFile(srcFile, dstFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}
