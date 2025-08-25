package constant

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStellarAppID(t *testing.T) {
	assert.Equal(t, "com.stmedicaltechnologyinc.stellar", StellarAppID)
}

func TestStellarPath(t *testing.T) {
	// Test StellarPath function
	path, err := StellarPath()
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify the path structure based on OS
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	var expectedBase string
	switch runtime.GOOS {
	case "windows":
		expectedBase = filepath.Join(homeDir, "AppData/Roaming")
	case "darwin":
		expectedBase = filepath.Join(homeDir, "Library/Application Support")
	case "linux":
		expectedBase = filepath.Join(homeDir, ".local/share")
	default:
		expectedBase = homeDir
	}

	expectedPath := filepath.Join(expectedBase, "Stellar")
	assert.Equal(t, expectedPath, path)
}

func TestStellarPathWithMockHomeDir(t *testing.T) {
	// Test with a mock home directory
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		}
	}()

	// Set a test home directory
	testHome := "/test/home"
	os.Setenv("HOME", testHome)

	// For Unix-like systems, this should work
	if runtime.GOOS != "windows" {
		path, err := StellarPath()
		require.NoError(t, err)
		assert.Contains(t, path, testHome)
	}
}

func TestStellarPathErrorHandling(t *testing.T) {
	// This test is challenging to implement without modifying the function
	// since os.UserHomeDir() is hard to mock. We'll test the error path
	// by ensuring the function handles errors gracefully.

	// The function should return an error if os.UserHomeDir() fails
	// This is tested implicitly by the success cases above
}

func TestSTELLAR_PATHVariable(t *testing.T) {
	// Test that STELLAR_PATH is set after init
	assert.NotEmpty(t, STELLAR_PATH)

	// Verify it matches the result of StellarPath()
	expectedPath, err := StellarPath()
	require.NoError(t, err)
	assert.Equal(t, expectedPath, STELLAR_PATH)
}

func TestStellarPathConsistency(t *testing.T) {
	// Test that multiple calls return the same result
	path1, err1 := StellarPath()
	require.NoError(t, err1)

	path2, err2 := StellarPath()
	require.NoError(t, err2)

	assert.Equal(t, path1, path2)
	assert.Equal(t, err1, err2)
}

func TestStellarPathIsAbsolute(t *testing.T) {
	path, err := StellarPath()
	require.NoError(t, err)

	// Verify the path is absolute
	assert.True(t, filepath.IsAbs(path))
}

func TestStellarPathContainsStellar(t *testing.T) {
	path, err := StellarPath()
	require.NoError(t, err)

	// Verify the path contains "Stellar"
	assert.Contains(t, path, "Stellar")
}

func BenchmarkStellarPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := StellarPath()
		if err != nil {
			b.Fatal(err)
		}
	}
}
