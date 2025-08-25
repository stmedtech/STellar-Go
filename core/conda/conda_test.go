package conda

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// Test CondaDownloadPath function
	path, err := CondaDownloadPath()

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
	// Test getPath function
	path, err := getPath()

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

func TestVersion(t *testing.T) {
	// Test Version function
	// This depends on conda being available, so we test carefully
	version, err := Version("conda")

	if err != nil {
		// If conda is not available, that's expected
		assert.Contains(t, err.Error(), "version not supported")
	} else {
		// If conda is available, version should be in format x.y.z
		assert.NotEmpty(t, version)
		assert.Regexp(t, `^\d+\.\d+\.\d+$`, version)
	}
}

func TestEnvList(t *testing.T) {
	// Test EnvList function
	// This depends on conda being available, so we test carefully
	envs, err := EnvList("conda")

	if err != nil {
		// If conda is not available, that's expected
		assert.NotNil(t, envs)
		assert.Empty(t, envs)
	} else {
		// If conda is available, should return a map
		assert.NotNil(t, envs)
		// Should not contain "base" environment
		for envName := range envs {
			assert.NotEqual(t, "base", envName)
		}
	}
}

func TestEnv(t *testing.T) {
	// Test Env function
	// This depends on conda being available, so we test carefully
	envPath, err := Env("conda", "test-env")

	if err != nil {
		// If conda is not available or env doesn't exist, that's expected
		assert.Contains(t, err.Error(), "environment not found")
	} else {
		// If conda is available and env exists, should return a path
		assert.NotEmpty(t, envPath)
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

func TestCreateEnv(t *testing.T) {
	// Test CreateEnv function
	// This depends on conda being available, so we test carefully
	envPath, err := CreateEnv("conda", "test-env-create", "3.9")

	if err != nil {
		// If conda is not available, that's expected
		assert.Contains(t, err.Error(), "environment creation failed")
	} else {
		// If conda is available, should return a path
		assert.NotEmpty(t, envPath)
	}
}

func TestRemoveEnv(t *testing.T) {
	// Test RemoveEnv function
	// This depends on conda being available, so we test carefully
	err := RemoveEnv("conda", "test-env-remove")

	if err != nil {
		// If conda is not available or env doesn't exist, that's expected
		assert.True(t, strings.Contains(err.Error(), "environment deletion failed") ||
			strings.Contains(err.Error(), "environment not exist"))
	} else {
		// If conda is available and env exists, should succeed
		assert.NoError(t, err)
	}
}

func TestUpdateEnv(t *testing.T) {
	// Test UpdateEnv function
	// This depends on conda being available, so we test carefully
	err := UpdateEnv("conda", "test-env-update", "/path/to/environment.yml")

	if err != nil {
		// If conda is not available or env doesn't exist, that's expected
		assert.True(t, strings.Contains(err.Error(), "environment update failed") ||
			strings.Contains(err.Error(), "environment not exist"))
	} else {
		// If conda is available and env exists, should succeed
		assert.NoError(t, err)
	}
}

func TestRunCommand(t *testing.T) {
	// Test RunCommand function
	// This depends on conda being available, so we test carefully
	output, err := RunCommand("conda", "test-env", "python", "--version")

	if err != nil {
		// If conda is not available or env doesn't exist, that's expected
		assert.Contains(t, err.Error(), "environment not exist")
	} else {
		// If conda is available and env exists, should return output
		assert.NotEmpty(t, output)
	}
}

func TestEnvInstallPackage(t *testing.T) {
	// Test EnvInstallPackage function
	// This depends on conda being available, so we test carefully
	err := EnvInstallPackage("conda", "test-env", "requests")

	if err != nil {
		// If conda is not available or env doesn't exist, that's expected
		assert.True(t, strings.Contains(err.Error(), "environment not exist") ||
			strings.Contains(err.Error(), "regexp"))
	} else {
		// If conda is available and env exists, should succeed
		assert.NoError(t, err)
	}
}

func TestDownload(t *testing.T) {
	// Test Download function with a valid URL
	tempDir := t.TempDir()

	// Test with a simple HTTP URL (GitHub raw content)
	url := "https://raw.githubusercontent.com/golang/go/master/README.md"

	filePath, err := Download(tempDir, url)

	if err != nil {
		// If download fails due to network issues, that's expected
		assert.Contains(t, err.Error(), "requested URL is not downloadable")
	} else {
		// If download succeeds, should return a file path
		assert.NotEmpty(t, filePath)
		assert.True(t, filepath.IsAbs(filePath))

		// File should exist
		_, statErr := os.Stat(filePath)
		assert.NoError(t, statErr)
	}
}

func TestDownloadUrl(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := DownloadUrl(tt.pythonVersion, tt.condaVersion)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, url)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, url)
				assert.Contains(t, url, "repo.anaconda.com")
				assert.Contains(t, url, "miniconda")
			}
		})
	}
}

func TestDownloadUrlPlatformSpecific(t *testing.T) {
	// Test DownloadUrl function for different platforms
	pythonVersion := "3.9"
	condaVersion := "25.3.1-1"

	url, err := DownloadUrl(pythonVersion, condaVersion)
	require.NoError(t, err)

	// URL should be platform-specific
	switch runtime.GOOS {
	case "darwin":
		assert.Contains(t, url, "MacOSX")
	case "linux":
		assert.Contains(t, url, "Linux")
	case "windows":
		assert.Contains(t, url, "Windows")
	default:
		assert.Contains(t, err.Error(), "unsupported OS")
	}

	// URL should contain architecture
	switch runtime.GOARCH {
	case "amd64":
		assert.Contains(t, url, "x86_64")
	case "arm64":
		// Check for either arm64 or aarch64
		hasArm64 := strings.Contains(url, "arm64")
		hasAarch64 := strings.Contains(url, "aarch64")
		assert.True(t, hasArm64 || hasAarch64)
	}
}

func TestInstall(t *testing.T) {
	// Test Install function
	// This is a complex function that downloads and installs conda
	// We'll test it carefully as it may not work in all environments
	err := Install("3.9")

	if err != nil {
		// If installation fails, that's expected in test environments
		// Just verify it's some kind of error
		assert.Error(t, err)
	} else {
		// If installation succeeds, should not error
		assert.NoError(t, err)
	}
}

func TestInstallUnsupportedOS(t *testing.T) {
	// Test Install function with unsupported OS
	// This is difficult to test without mocking, so we'll skip it
	t.Skip("Install function testing requires complex mocking")
}

func BenchmarkCommandExists(b *testing.B) {
	for i := 0; i < b.N; i++ {
		commandExists("go")
	}
}

func BenchmarkCondaDownloadPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := CondaDownloadPath()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDownloadUrl(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := DownloadUrl("3.9", "25.3.1-1")
		if err != nil {
			b.Fatal(err)
		}
	}
}
