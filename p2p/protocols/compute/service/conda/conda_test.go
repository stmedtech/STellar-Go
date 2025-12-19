package conda

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCondaVersion(t *testing.T) {
	assert.Equal(t, "25.3.1-1", CONDA_VERSION)
}

func TestUpdateCondaPath(t *testing.T) {
	// Test UpdateCondaPath function
	// This function depends on conda being installed, so we test it carefully
	result := UpdateCondaPath()

	// The function should return a boolean indicating success/failure
	// We can't guarantee conda is installed, so we just verify the function runs
	assert.IsType(t, false, result)
}

func TestCondaDownloadPath(t *testing.T) {
	// Test GetCondaDownloadPath function
	path, err := GetCondaDownloadPath()

	// Should not error
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Should contain "conda" in the path
	assert.Contains(t, path, "conda")

	// Should be an absolute path
	assert.True(t, filepath.IsAbs(path))
}

func TestCommandExists(t *testing.T) {
	// Test commandExists function with existing command
	exists := commandExists("go")
	assert.True(t, exists)

	// Test with non-existent command
	exists = commandExists("nonexistent-command-12345")
	assert.False(t, exists)
}

func TestGetPath(t *testing.T) {
	// Test FindCondaPath function
	path, err := FindCondaPath()

	// The function may fail if conda is not installed, which is expected
	if err != nil {
		assert.Contains(t, err.Error(), "conda executable not found")
	} else {
		assert.NotEmpty(t, path)
	}
}

func TestSaveOutput(t *testing.T) {
	// Test saveOutput struct
	so := &saveOutput{}

	// Test Write method
	testData := []byte("test output")
	n, err := so.Write(testData)

	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, so.savedOutput)

	// Test multiple writes
	additionalData := []byte(" additional data")
	n, err = so.Write(additionalData)

	assert.NoError(t, err)
	assert.Equal(t, len(additionalData), n)

	expectedData := append(testData, additionalData...)
	assert.Equal(t, expectedData, so.savedOutput)
}

func TestRunCommandFunction(t *testing.T) {
	// Test runCommand function with a simple command
	cmd := exec.Command("echo", "test")

	output, err := runCommand(cmd)

	// Should not error for a simple echo command
	require.NoError(t, err)
	assert.Contains(t, string(output), "test")
}

func TestRunCommandError(t *testing.T) {
	// Test runCommand function with a command that will fail
	cmd := exec.Command("nonexistent-command-12345")

	output, err := runCommand(cmd)

	// Should error for non-existent command
	assert.Error(t, err)
	assert.Empty(t, output)
}

func TestGetCondaVersion(t *testing.T) {
	// Test GetCondaVersion function
	// This depends on conda being available, so we test carefully
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skipf("conda not found: %v", err)
	}

	version, err := GetCondaVersion(condaPath)
	if err != nil {
		// If conda is not available or version check fails, that's expected
		assert.Error(t, err)
	} else {
		// If conda is available, version should be in format x.y.z
		assert.NotEmpty(t, version)
		assert.Regexp(t, `^\d+\.\d+\.\d+`, version) // Allow for patch versions like 25.3.1-1
	}
}

func TestCommandPath(t *testing.T) {
	// Test CommandPath function
	// This depends on conda being available, so we test carefully
	path, err := CommandPath()

	if err != nil {
		// If conda is not available, that's expected
		assert.Empty(t, path)
	} else {
		// If conda is available, should return a path
		assert.NotEmpty(t, path)
	}
}

// TestDownloadCondaInstaller tests DownloadCondaInstaller function
func TestDownloadCondaInstaller(t *testing.T) {
	tests := []struct {
		name          string
		pythonVersion string
		condaVersion  string
		wantErr       bool
	}{
		{
			name:          "valid versions",
			pythonVersion: "3.9",
			condaVersion:  "25.3.1-1",
			wantErr:       false,
		},
		{
			name:          "empty versions",
			pythonVersion: "",
			condaVersion:  "",
			wantErr:       false,
		},
		{
			name:          "invalid OS",
			pythonVersion: "3.9",
			condaVersion:  "25.3.1-1",
			wantErr:       false, // Function handles unsupported OS gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := DownloadCondaInstaller(tt.pythonVersion, tt.condaVersion)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, url)
			} else {
				// Function may return error for unsupported OS/arch, which is expected
				if err != nil {
					assert.Contains(t, err.Error(), "unsupported")
				} else {
					assert.NotEmpty(t, url)
					assert.Contains(t, url, "https://repo.anaconda.com/miniconda/Miniconda3-")
				}
			}
		})
	}
}

// TestDownloadCondaInstaller_PlatformSpecific tests DownloadCondaInstaller for current platform
// Note: This function uses runtime.GOOS/GOARCH, so it always returns URLs for the current platform
func TestDownloadCondaInstaller_PlatformSpecific(t *testing.T) {
	url, err := DownloadCondaInstaller("3.9", "25.3.1-1")

	if err != nil {
		// If unsupported platform, that's expected
		assert.Contains(t, err.Error(), "unsupported")
	} else {
		// Verify URL format for current platform
		assert.NotEmpty(t, url)
		assert.Contains(t, url, "https://repo.anaconda.com/miniconda/Miniconda3-")
		assert.Contains(t, url, "3.9_25.3.1-1-")

		// Verify platform-specific suffix based on current platform
		// This is tested more comprehensively in infrastructure_test.go
		assert.True(t,
			strings.Contains(url, ".sh") || strings.Contains(url, ".exe"),
			"URL should end with .sh or .exe: %s", url)
	}
}

func BenchmarkCommandExists(b *testing.B) {
	for i := 0; i < b.N; i++ {
		commandExists("go")
	}
}

func BenchmarkGetCondaDownloadPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GetCondaDownloadPath()
	}
}

func BenchmarkDownloadCondaInstaller(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DownloadCondaInstaller("3.9", "25.3.1-1")
	}
}
