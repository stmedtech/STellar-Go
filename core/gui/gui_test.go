package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrettyByteSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int
		expected string
	}{
		{"zero bytes", 0, "0.0B"},
		{"bytes", 512, "512.0B"},
		{"kilobytes", 1024, "1.0KiB"},
		{"megabytes", 1024 * 1024, "1.0MiB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0GiB"},
		{"terabytes", 1024 * 1024 * 1024 * 1024, "1.0TiB"},
		{"petabytes", 1024 * 1024 * 1024 * 1024 * 1024, "1.0PiB"},
		{"exabytes", 1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1.0EiB"},
		{"fractional kilobytes", 1536, "1.5KiB"},
		{"fractional megabytes", 1024*1024 + 512*1024, "1.5MiB"},
		{"large number", 1234567890, "1.1GiB"},
		{"negative bytes", -1024, "-1.0KiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prettyByteSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrettyByteSizeEdgeCases(t *testing.T) {
	// Test very large numbers (within int limits)
	veryLarge := 1024 * 1024 * 1024 * 1024 * 1024 * 1024 // 1024^6 (EiB)
	result := prettyByteSize(veryLarge)
	assert.Contains(t, result, "EiB")

	// Test very small numbers
	result = prettyByteSize(1)
	assert.Equal(t, "1.0B", result)

	result = prettyByteSize(1023)
	assert.Equal(t, "1023.0B", result)
}

func TestPrettyByteSizePrecision(t *testing.T) {
	// Test that precision is maintained correctly
	tests := []struct {
		bytes    int
		expected string
	}{
		{1024, "1.0KiB"}, // Exactly 1 KiB
		{1025, "1.0KiB"}, // Just over 1 KiB
		{1536, "1.5KiB"}, // 1.5 KiB
		{1792, "1.8KiB"}, // 1.75 KiB, should round to 1.8
		{2048, "2.0KiB"}, // Exactly 2 KiB
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := prettyByteSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewGUIApp(t *testing.T) {
	// Test creating a new GUI app
	app, err := NewGUIApp()
	require.NoError(t, err)
	require.NotNil(t, app)

	// Verify initial state
	assert.False(t, app.Bypass)
	assert.NotNil(t, app.a)
	assert.Nil(t, app.w) // Window should be nil initially
	assert.NotNil(t, app.overviewContainer)
	assert.NotNil(t, app.devices)
	assert.NotNil(t, app.selectedDeviceId)
	assert.NotNil(t, app.proxies)
	assert.NotNil(t, app.selectedProxy)
}

func TestNewGUIAppMultiple(t *testing.T) {
	// Test creating multiple GUI apps
	app1, err1 := NewGUIApp()
	require.NoError(t, err1)
	require.NotNil(t, app1)

	app2, err2 := NewGUIApp()
	require.NoError(t, err2)
	require.NotNil(t, app2)

	// Apps should be different instances
	assert.NotEqual(t, app1, app2)
}

func TestGUIAppStruct(t *testing.T) {
	// Test GUIApp struct fields
	app := &GUIApp{
		Bypass: true,
	}

	// Verify struct can be created
	assert.True(t, app.Bypass)
	assert.Nil(t, app.node)
	assert.Nil(t, app.proxy)
	assert.Nil(t, app.a)
	assert.Nil(t, app.w)
	assert.Nil(t, app.overviewContainer)
	assert.Nil(t, app.devices)
	assert.Nil(t, app.selectedDeviceId)
	assert.Nil(t, app.proxies)
	assert.Nil(t, app.selectedProxy)
	assert.Nil(t, app.policyEnable)
	assert.Nil(t, app.whitelist)
}

func TestPrettyByteSizeBoundaryValues(t *testing.T) {
	// Test boundary values around unit transitions
	tests := []struct {
		bytes    int
		expected string
		unit     string
	}{
		{1023, "1023.0B", "bytes"},
		{1024, "1.0KiB", "kilobytes"},
		{1025, "1.0KiB", "kilobytes"},
		{1048575, "1024.0KiB", "kilobytes"},
		{1048576, "1.0MiB", "megabytes"},
		{1048577, "1.0MiB", "megabytes"},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			result := prettyByteSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrettyByteSizeNegativeValues(t *testing.T) {
	// Test negative values
	tests := []struct {
		bytes    int
		expected string
	}{
		{-1, "-1.0B"},
		{-1024, "-1.0KiB"},
		{-1048576, "-1.0MiB"},
		{-1073741824, "-1.0GiB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := prettyByteSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrettyByteSizeLargeNumbers(t *testing.T) {
	// Test very large numbers to ensure no overflow (within int limits)
	largeNumbers := []int{
		1 << 30, // 1 GiB
		1 << 40, // 1 TiB
		1 << 50, // 1 PiB
		1 << 60, // 1 EiB
	}

	for _, num := range largeNumbers {
		t.Run("large_number", func(t *testing.T) {
			result := prettyByteSize(num)
			// Should not panic and should return a valid string
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "B")
		})
	}
}

func BenchmarkPrettyByteSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		prettyByteSize(i % 1000000)
	}
}

func BenchmarkNewGUIApp(b *testing.B) {
	for i := 0; i < b.N; i++ {
		app, err := NewGUIApp()
		if err != nil {
			b.Fatal(err)
		}
		if app == nil {
			b.Fatal("app is nil")
		}
	}
}
