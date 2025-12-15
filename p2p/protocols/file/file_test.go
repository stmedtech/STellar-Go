package file

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"stellar/pkg/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileChecksum(t *testing.T) {
	// Create a test file with known content
	content := "Hello, World! This is a test file for checksum calculation."
	testFile := testutils.TestTempFile(t, content)

	// Calculate expected checksum
	expectedHash := sha256.Sum256([]byte(content))
	expectedChecksum := hex.EncodeToString(expectedHash[:])

	// Test FileChecksum function
	checksum, err := FileChecksum(testFile)
	require.NoError(t, err)
	assert.Equal(t, expectedChecksum, checksum)

	// Test with non-existent file
	_, err = FileChecksum("/non/existent/file")
	assert.Error(t, err)
}

func TestFileEntry_FullName(t *testing.T) {
	tests := []struct {
		name           string
		directoryName  string
		filename       string
		expectedResult string
	}{
		{
			name:           "simple file",
			directoryName:  "/test",
			filename:       "file.txt",
			expectedResult: "/test/file.txt",
		},
		{
			name:           "file in root",
			directoryName:  "/",
			filename:       "file.txt",
			expectedResult: "/file.txt",
		},
		{
			name:           "nested directory",
			directoryName:  "/test/nested",
			filename:       "file.txt",
			expectedResult: "/test/nested/file.txt",
		},
		{
			name:           "empty directory",
			directoryName:  "",
			filename:       "file.txt",
			expectedResult: "file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := FileEntry{
				DirectoryName: tt.directoryName,
				Filename:      tt.filename,
			}

			result := entry.FullName()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
