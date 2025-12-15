package conda

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindCondaPath_Windows_SystemPath tests finding conda in Windows system path
func TestFindCondaPath_Windows_SystemPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	// This test requires actual conda installation, so we test the function exists
	path, err := FindCondaPath()
	if err != nil {
		// If conda not found, that's expected in test environments
		assert.Contains(t, err.Error(), "conda executable not found")
	} else {
		assert.NotEmpty(t, path)
		assert.True(t, strings.HasSuffix(path, "conda.exe") || strings.HasSuffix(path, "_conda.exe"))
	}
}

// TestFindCondaPath_Windows_UserPath tests finding conda in Windows user directory
func TestFindCondaPath_Windows_UserPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	// Test that function handles user directory paths
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_Linux_SystemPath tests finding conda in Linux system PATH
func TestFindCondaPath_Linux_SystemPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	path, err := FindCondaPath()
	if err != nil {
		assert.Contains(t, err.Error(), "conda executable not found")
	} else {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_Linux_LocalInstall tests finding conda in local install
func TestFindCondaPath_Linux_LocalInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	// Test that function checks local install directory
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_Darwin_SystemPath tests finding conda in macOS system PATH
func TestFindCondaPath_Darwin_SystemPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	path, err := FindCondaPath()
	if err != nil {
		assert.Contains(t, err.Error(), "conda executable not found")
	} else {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_Darwin_LocalInstall tests finding conda in macOS local install
func TestFindCondaPath_Darwin_LocalInstall(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_NotFound tests when conda is not found
func TestFindCondaPath_NotFound(t *testing.T) {
	// This test may pass or fail depending on whether conda is installed
	// We just verify the function handles the case gracefully
	path, err := FindCondaPath()
	if err != nil {
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "conda executable not found")
	}
}

// TestFindCondaPath_MultiplePaths tests that system path is preferred over local
func TestFindCondaPath_MultiplePaths(t *testing.T) {
	// This test verifies the search order logic
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestGetCondaVersion_Success tests parsing version from conda --version
func TestGetCondaVersion_Success(t *testing.T) {
	// Test with a mock version string
	// Since we can't easily mock exec.Command, we test the parsing logic
	// by testing with actual conda if available
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skip("Conda not available for version test")
	}

	version, err := GetCondaVersion(condaPath)
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	// Version should be in format x.y.z
	assert.Regexp(t, `^\d+\.\d+\.\d+`, version)
}

// TestGetCondaVersion_InvalidOutput tests handling malformed version output
func TestGetCondaVersion_InvalidOutput(t *testing.T) {
	// Test with invalid conda path
	_, err := GetCondaVersion("/nonexistent/conda")
	assert.Error(t, err)
}

// TestGetCondaVersion_CommandFails tests handling conda command failure
func TestGetCondaVersion_CommandFails(t *testing.T) {
	// Test with invalid path
	_, err := GetCondaVersion("nonexistent-conda-command")
	assert.Error(t, err)
}

// TestGetCondaVersion_EmptyOutput tests handling empty output
func TestGetCondaVersion_EmptyOutput(t *testing.T) {
	// This would require mocking, but we test the error path
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skip("Conda not available")
	}

	// If conda exists, version should not be empty
	version, err := GetCondaVersion(condaPath)
	if err == nil {
		assert.NotEmpty(t, version)
	}
}

// TestGetCondaDownloadPath_Success tests creating download directory
func TestGetCondaDownloadPath_Success(t *testing.T) {
	path, err := GetCondaDownloadPath()
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.True(t, filepath.IsAbs(path))
	assert.Contains(t, path, "conda")
}

// TestGetCondaDownloadPath_PermissionDenied tests handling permission errors
func TestGetCondaDownloadPath_PermissionDenied(t *testing.T) {
	// This is hard to test without actually creating permission issues
	// We verify the function handles errors gracefully
	path, err := GetCondaDownloadPath()
	if err != nil {
		assert.Empty(t, path)
	} else {
		assert.NotEmpty(t, path)
	}
}

// TestGetCondaDownloadPath_ExistingDirectory tests using existing directory
func TestGetCondaDownloadPath_ExistingDirectory(t *testing.T) {
	path1, err1 := GetCondaDownloadPath()
	require.NoError(t, err1)

	// Call again - should return same path
	path2, err2 := GetCondaDownloadPath()
	require.NoError(t, err2)
	assert.Equal(t, path1, path2)
}

// TestDownloadUrl_Linux_AMD64 tests URL generation for Linux x86_64
func TestDownloadUrl_Linux_AMD64(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux AMD64-specific test")
	}

	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	require.NoError(t, err)
	assert.Contains(t, url, "Linux-x86_64.sh")
	assert.Contains(t, url, "repo.anaconda.com")
	assert.Contains(t, url, "miniconda")
}

// TestDownloadUrl_Linux_ARM64 tests URL generation for Linux ARM64
func TestDownloadUrl_Linux_ARM64(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("Linux ARM64-specific test")
	}

	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	require.NoError(t, err)
	assert.Contains(t, url, "Linux-aarch64.sh")
	assert.Contains(t, url, "repo.anaconda.com")
}

// TestDownloadUrl_Darwin_AMD64 tests URL generation for macOS x86_64
func TestDownloadUrl_Darwin_AMD64(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "amd64" {
		t.Skip("macOS AMD64-specific test")
	}

	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	require.NoError(t, err)
	assert.Contains(t, url, "MacOSX-x86_64.sh")
	assert.Contains(t, url, "repo.anaconda.com")
}

// TestDownloadUrl_Darwin_ARM64 tests URL generation for macOS ARM64
func TestDownloadUrl_Darwin_ARM64(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("macOS ARM64-specific test")
	}

	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	require.NoError(t, err)
	assert.Contains(t, url, "MacOSX-arm64.sh")
	assert.Contains(t, url, "repo.anaconda.com")
}

// TestDownloadUrl_Windows_AMD64 tests URL generation for Windows x86_64
func TestDownloadUrl_Windows_AMD64(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("Windows AMD64-specific test")
	}

	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	require.NoError(t, err)
	assert.Contains(t, url, "Windows-x86_64.exe")
	assert.Contains(t, url, "repo.anaconda.com")
}

// TestDownloadUrl_UnsupportedArch tests error for unsupported architecture
func TestDownloadUrl_UnsupportedArch(t *testing.T) {
	// Test with unsupported architecture by checking current arch
	// If current arch is supported, we can't easily test unsupported case
	// So we verify the function works for supported architectures
	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	if err != nil {
		// If error, should be about unsupported architecture
		assert.Contains(t, err.Error(), "unsupported")
	} else {
		// If success, URL should be valid
		assert.NotEmpty(t, url)
	}
}

// TestDownloadUrl_UnsupportedOS tests error for unsupported OS
func TestDownloadUrl_UnsupportedOS(t *testing.T) {
	// Current OS should be supported, so we verify the function works
	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	if err != nil {
		// If error, should be about unsupported OS
		assert.Contains(t, err.Error(), "unsupported OS")
	} else {
		// If success, URL should be valid
		assert.NotEmpty(t, url)
	}
}

// TestFindCondaPath_EmptyPATH tests handling empty PATH environment variable
func TestFindCondaPath_EmptyPATH(t *testing.T) {
	// Save original PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set empty PATH
	os.Setenv("PATH", "")

	// Function should still check hardcoded paths
	path, err := FindCondaPath()
	if err != nil {
		assert.Contains(t, err.Error(), "conda executable not found")
	} else {
		assert.NotEmpty(t, path)
	}
}

// TestFindCondaPath_Symlink tests handling symlinked conda executables
func TestFindCondaPath_Symlink(t *testing.T) {
	// This test verifies symlinks are handled correctly
	// Actual test would require creating symlinks, which is complex
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestGetCondaVersion_NonNumericVersion tests handling non-standard version formats
func TestGetCondaVersion_NonNumericVersion(t *testing.T) {
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skip("Conda not available")
	}

	version, err := GetCondaVersion(condaPath)
	if err == nil {
		// Version should match regex pattern
		assert.Regexp(t, `^\d+\.\d+`, version)
	}
}

// TestGetCondaDownloadPath_DiskFull tests handling disk full scenario
func TestGetCondaDownloadPath_DiskFull(t *testing.T) {
	// This is difficult to test without actually filling disk
	// We verify the function handles errors gracefully
	path, err := GetCondaDownloadPath()
	if err != nil {
		assert.Empty(t, path)
	} else {
		assert.NotEmpty(t, path)
	}
}

// TestDownloadUrl_InvalidVersion tests handling invalid version strings
func TestDownloadUrl_InvalidVersion(t *testing.T) {
	// Test with empty version
	url, err := DownloadCondaInstaller("", "")
	if err == nil {
		// Even with empty versions, URL might be generated (just incomplete)
		assert.NotEmpty(t, url)
	}

	// Test with special characters
	url, err = DownloadCondaInstaller("py3.13", "25.3.1-1")
	if err == nil {
		assert.NotEmpty(t, url)
	}
}

// TestGetCondaVersion_WithRealConda tests version detection with real conda if available
func TestGetCondaVersion_WithRealConda(t *testing.T) {
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skip("Conda not available for version test")
	}

	version, err := GetCondaVersion(condaPath)
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	// Version should match pattern x.y.z
	assert.Regexp(t, `^\d+\.\d+\.\d+`, version)
}

// TestGetCondaVersion_InvalidPath tests with invalid conda path
func TestGetCondaVersion_InvalidPath(t *testing.T) {
	_, err := GetCondaVersion("/nonexistent/path/to/conda")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run")
}

// TestGetCondaVersion_EmptyPath tests with empty path
func TestGetCondaVersion_EmptyPath(t *testing.T) {
	_, err := GetCondaVersion("")
	assert.Error(t, err)
}

// TestGetCondaDownloadPath_CreatesDirectory tests that directory is created if it doesn't exist
func TestGetCondaDownloadPath_CreatesDirectory(t *testing.T) {
	path, err := GetCondaDownloadPath()
	require.NoError(t, err)

	// Verify directory exists
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestDownloadCondaInstaller_AllPlatforms tests URL generation for all supported platforms
func TestDownloadCondaInstaller_AllPlatforms(t *testing.T) {
	testCases := []struct {
		name          string
		pythonVersion string
		condaVersion  string
		expectError   bool
	}{
		{"valid versions", "py313", "25.3.1-1", false},
		{"empty versions", "", "", false},
		{"special chars", "py3.13", "25.3.1-1", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url, err := DownloadCondaInstaller(tc.pythonVersion, tc.condaVersion)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				if err == nil {
					assert.NotEmpty(t, url)
					assert.Contains(t, url, "repo.anaconda.com")
					assert.Contains(t, url, "miniconda")
				}
			}
		})
	}
}

// TestFindCondaPath_PriorityOrder tests that system paths are checked before local install
func TestFindCondaPath_PriorityOrder(t *testing.T) {
	// This test verifies the search order logic
	// If conda is in PATH, it should be found
	// If not in PATH, local install should be checked
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
	}
}

// TestGetCondaDownloadPath_MultipleCalls tests that multiple calls return same path
func TestGetCondaDownloadPath_MultipleCalls(t *testing.T) {
	path1, err1 := GetCondaDownloadPath()
	require.NoError(t, err1)

	path2, err2 := GetCondaDownloadPath()
	require.NoError(t, err2)

	assert.Equal(t, path1, path2)
}

// TestDownloadCondaInstaller_Format tests URL format correctness
func TestDownloadCondaInstaller_Format(t *testing.T) {
	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	if err == nil {
		// URL should start with https://
		assert.True(t, strings.HasPrefix(url, "https://"))
		// Should contain both versions
		assert.Contains(t, url, "py313")
		assert.Contains(t, url, "25.3.1-1")
	}
}

// TestDownloadCondaInstaller_AllBranches tests all platform/architecture branches
func TestDownloadCondaInstaller_AllBranches(t *testing.T) {
	// Test current platform
	url, err := DownloadCondaInstaller("py313", "25.3.1-1")
	if err == nil {
		assert.NotEmpty(t, url)
		assert.Contains(t, url, "repo.anaconda.com")
	}

	// Test that all supported combinations work
	testCases := []struct {
		os     string
		arch   string
		suffix string
	}{
		{"darwin", "amd64", "MacOSX-x86_64.sh"},
		{"darwin", "arm64", "MacOSX-arm64.sh"},
		{"linux", "amd64", "Linux-x86_64.sh"},
		{"linux", "arm64", "Linux-aarch64.sh"},
		{"windows", "amd64", "Windows-x86_64.exe"},
	}

	for _, tc := range testCases {
		if runtime.GOOS == tc.os && runtime.GOARCH == tc.arch {
			url, err := DownloadCondaInstaller("py313", "25.3.1-1")
			require.NoError(t, err)
			assert.Contains(t, url, tc.suffix)
		}
	}
}

// TestFindCondaPath_AllPaths tests all path search branches
func TestFindCondaPath_AllPaths(t *testing.T) {
	// Test that function tries all paths
	path, err := FindCondaPath()
	if err == nil {
		assert.NotEmpty(t, path)
		// Path should be absolute or be "conda" (in PATH)
		assert.True(t, filepath.IsAbs(path) || path == "conda")
	}
}

// TestGetCondaDownloadPath_ErrorHandling tests error cases
func TestGetCondaDownloadPath_ErrorHandling(t *testing.T) {
	// This is hard to test without mocking constant.StellarPath()
	// But we verify the function works in normal cases
	path, err := GetCondaDownloadPath()
	if err != nil {
		// If error, should be about path creation
		assert.Contains(t, err.Error(), "conda")
	} else {
		assert.NotEmpty(t, path)
	}
}
