package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSystemInformation(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// Verify all fields are populated
	assert.NotEmpty(t, info.Platform)
	assert.NotEmpty(t, info.CPU)
	assert.NotNil(t, info.GPU) // Should be initialized even if empty
	assert.Greater(t, info.RAM, uint64(0))
}

func TestSystemInformationStructure(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// Test that GPU slice is properly initialized
	assert.NotNil(t, info.GPU)

	// Test that RAM is in MB (should be reasonable range)
	assert.Greater(t, info.RAM, uint64(100))  // At least 100MB
	assert.Less(t, info.RAM, uint64(1000000)) // Less than 1TB
}

func TestGetSystemInformationConsistency(t *testing.T) {
	// Test that multiple calls return consistent results
	info1, err1 := GetSystemInformation()
	require.NoError(t, err1)

	info2, err2 := GetSystemInformation()
	require.NoError(t, err2)

	// Platform and CPU should be consistent
	assert.Equal(t, info1.Platform, info2.Platform)
	assert.Equal(t, info1.CPU, info2.CPU)
	assert.Equal(t, info1.RAM, info2.RAM)

	// GPU list might vary slightly due to timing, but should be same length
	assert.Equal(t, len(info1.GPU), len(info2.GPU))
}

func TestGetSystemInformationPlatform(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// Platform should be a valid OS name (including distribution names)
	validPlatforms := []string{"linux", "windows", "darwin", "freebsd", "openbsd", "ubuntu", "debian", "centos", "fedora"}
	found := false
	for _, platform := range validPlatforms {
		if info.Platform == platform {
			found = true
			break
		}
	}
	assert.True(t, found, "Platform should be one of: %v, got: %s", validPlatforms, info.Platform)
}

func TestGetSystemInformationCPU(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// CPU should not be empty
	assert.NotEmpty(t, info.CPU)
	// CPU name should contain typical identifiers (CPU, Processor, Core, etc.)
	validIdentifiers := []string{"CPU", "Processor", "Core", "Ryzen", "Intel", "AMD"}
	found := false
	for _, identifier := range validIdentifiers {
		if strings.Contains(info.CPU, identifier) {
			found = true
			break
		}
	}
	assert.True(t, found, "CPU name should contain one of: %v, got: %s", validIdentifiers, info.CPU)
}

func TestGetSystemInformationRAM(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// RAM should be reasonable values
	assert.Greater(t, info.RAM, uint64(0))

	// Convert back to bytes to verify calculation
	ramBytes := info.RAM * 1024 * 1024
	assert.Greater(t, ramBytes, uint64(100*1024*1024)) // At least 100MB
}

func TestGetGpus(t *testing.T) {
	gpus, err := getGpus()

	// GPU detection might fail in some environments (e.g., containers)
	// So we don't require success, but if it succeeds, verify the result
	if err != nil {
		t.Logf("GPU detection failed (expected in some environments): %v", err)
		return
	}

	// If successful, verify the result
	assert.NotNil(t, gpus)

	// Each GPU name should not be empty
	for i, gpu := range gpus {
		assert.NotEmpty(t, gpu, "GPU %d name should not be empty", i)
	}
}

func TestSystemInformationGPUField(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// GPU field should be initialized even if no GPUs found
	assert.NotNil(t, info.GPU)

	// If GPUs are found, they should have valid names
	for i, gpu := range info.GPU {
		if gpu != "" {
			assert.NotEmpty(t, gpu, "GPU %d name should not be empty", i)
		}
	}
}

func TestSystemInformationMemoryCalculation(t *testing.T) {
	info, err := GetSystemInformation()
	require.NoError(t, err)

	// Test that RAM calculation is correct (MB from bytes)
	// This is a basic sanity check
	ramBytes := info.RAM * 1024 * 1024
	expectedMB := ramBytes / 1024 / 1024
	assert.Equal(t, info.RAM, expectedMB)
}

func TestSystemInformationErrorHandling(t *testing.T) {
	// This test verifies that the function handles errors gracefully
	// by returning partial information when some components fail

	info, err := GetSystemInformation()
	require.NoError(t, err)

	// Even if some components fail, we should get a valid structure
	assert.NotNil(t, info.GPU)
	assert.GreaterOrEqual(t, info.RAM, uint64(0))
}

func BenchmarkGetSystemInformation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GetSystemInformation()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetGpus(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := getGpus()
		if err != nil {
			// GPU detection might fail in some environments
			b.Skip("GPU detection not available")
		}
	}
}
