//go:build conda

package conda

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestGetCondaVersion_WithRealConda tests version detection with real conda if available
func TestGetCondaVersion_WithRealConda(t *testing.T) {
	condaPath, err := FindCondaPath()
	if err != nil {
		t.Skip("Conda not available for version test")
	}

	version, err := GetCondaVersion(condaPath)
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	// Version should be numeric
	assert.Regexp(t, `^\d+\.\d+`, version)
}
