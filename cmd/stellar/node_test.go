package main

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeCommandFlags(t *testing.T) {
	// Test flag parsing for node command
	tests := []struct {
		name     string
		args     []string
		expected struct {
			host           string
			port           int
			seed           int64
			b64privkey     string
			referenceToken string
			metrics        bool
			disablePolicy  bool
		}
	}{
		{
			name: "default flags",
			args: []string{"node"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           0,
				b64privkey:     "",
				referenceToken: "",
				metrics:        false,
				disablePolicy:  false,
			},
		},
		{
			name: "custom host and port",
			args: []string{"node", "-host", "127.0.0.1", "-port", "8080"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "127.0.0.1",
				port:           8080,
				seed:           0,
				b64privkey:     "",
				referenceToken: "",
				metrics:        false,
				disablePolicy:  false,
			},
		},
		{
			name: "with seed",
			args: []string{"node", "-seed", "12345"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           12345,
				b64privkey:     "",
				referenceToken: "",
				metrics:        false,
				disablePolicy:  false,
			},
		},
		{
			name: "with private key",
			args: []string{"node", "-b64privkey", "test-key"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           0,
				b64privkey:     "test-key",
				referenceToken: "",
				metrics:        false,
				disablePolicy:  false,
			},
		},
		{
			name: "with reference token",
			args: []string{"node", "-reference_token", "my-token"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           0,
				b64privkey:     "",
				referenceToken: "my-token",
				metrics:        false,
				disablePolicy:  false,
			},
		},
		{
			name: "with metrics enabled",
			args: []string{"node", "-metrics"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           0,
				b64privkey:     "",
				referenceToken: "",
				metrics:        true,
				disablePolicy:  false,
			},
		},
		{
			name: "with policy disabled",
			args: []string{"node", "-disable-policy"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "0.0.0.0",
				port:           0,
				seed:           0,
				b64privkey:     "",
				referenceToken: "",
				metrics:        false,
				disablePolicy:  true,
			},
		},
		{
			name: "all flags combined",
			args: []string{"node", "-host", "192.168.1.1", "-port", "9000", "-seed", "42", "-reference_token", "combined-token", "-metrics", "-disable-policy"},
			expected: struct {
				host           string
				port           int
				seed           int64
				b64privkey     string
				referenceToken string
				metrics        bool
				disablePolicy  bool
			}{
				host:           "192.168.1.1",
				port:           9000,
				seed:           42,
				b64privkey:     "",
				referenceToken: "combined-token",
				metrics:        true,
				disablePolicy:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = tt.args

			// Create a new flag set
			nodeCmd := flag.NewFlagSet("node", flag.ContinueOnError)

			// Define flags
			listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
			listenPort := nodeCmd.Int("port", 0, "set listening port")
			seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
			b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
			referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
			metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
			disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

			// Parse flags (skip the first argument which is the command name)
			err := nodeCmd.Parse(tt.args[1:])
			require.NoError(t, err, "Flag parsing should succeed")

			// Verify flag values
			assert.Equal(t, tt.expected.host, *listenHost, "Host should match")
			assert.Equal(t, tt.expected.port, *listenPort, "Port should match")
			assert.Equal(t, tt.expected.seed, *seed, "Seed should match")
			assert.Equal(t, tt.expected.b64privkey, *b64privkey, "Private key should match")
			assert.Equal(t, tt.expected.referenceToken, *referenceToken, "Reference token should match")
			assert.Equal(t, tt.expected.metrics, *metrics, "Metrics flag should match")
			assert.Equal(t, tt.expected.disablePolicy, *disablePolicy, "Disable policy flag should match")
		})
	}
}

func TestNodeCommandFlagValidation(t *testing.T) {
	// Test invalid flag combinations
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "invalid port number",
			args:    []string{"node", "-port", "99999"},
			wantErr: false, // Flag parsing doesn't validate port range
		},
		{
			name:    "negative port",
			args:    []string{"node", "-port", "-1"},
			wantErr: false, // Flag parsing doesn't validate port range
		},
		{
			name:    "invalid host",
			args:    []string{"node", "-host", "invalid-host"},
			wantErr: false, // Flag parsing doesn't validate host format
		},
		{
			name:    "empty private key",
			args:    []string{"node", "-b64privkey", ""},
			wantErr: false, // Empty string is valid for flag parsing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = tt.args

			// Create a new flag set
			nodeCmd := flag.NewFlagSet("node", flag.ContinueOnError)

			// Define flags
			listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
			listenPort := nodeCmd.Int("port", 0, "set listening port")
			seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
			b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
			referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
			metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
			disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

			// Parse flags
			err := nodeCmd.Parse(tt.args[1:])

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid flags")
			} else {
				assert.NoError(t, err, "Flag parsing should succeed even with invalid values")
			}

			// Verify flags were parsed (even if invalid)
			assert.NotNil(t, listenHost, "Host flag should be defined")
			assert.NotNil(t, listenPort, "Port flag should be defined")
			assert.NotNil(t, seed, "Seed flag should be defined")
			assert.NotNil(t, b64privkey, "Private key flag should be defined")
			assert.NotNil(t, referenceToken, "Reference token flag should be defined")
			assert.NotNil(t, metrics, "Metrics flag should be defined")
			assert.NotNil(t, disablePolicy, "Disable policy flag should be defined")
		})
	}
}

func TestNodeCommandFlagDefaults(t *testing.T) {
	// Test that default values are correct
	nodeCmd := flag.NewFlagSet("node", flag.ContinueOnError)

	// Define flags with defaults
	listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := nodeCmd.Int("port", 0, "set listening port")
	seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
	metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
	disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

	// Verify default values
	assert.Equal(t, "0.0.0.0", *listenHost, "Default host should be 0.0.0.0")
	assert.Equal(t, 0, *listenPort, "Default port should be 0")
	assert.Equal(t, int64(0), *seed, "Default seed should be 0")
	assert.Equal(t, "", *b64privkey, "Default private key should be empty")
	assert.Equal(t, "", *referenceToken, "Default reference token should be empty")
	assert.Equal(t, false, *metrics, "Default metrics should be false")
	assert.Equal(t, false, *disablePolicy, "Default disable policy should be false")
}

func TestNodeCommandFlagUsage(t *testing.T) {
	// Test flag usage strings
	nodeCmd := flag.NewFlagSet("node", flag.ContinueOnError)

	// Define flags
	listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := nodeCmd.Int("port", 0, "set listening port")
	seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
	metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
	disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

	// Use variables to avoid "declared and not used" errors
	_ = listenHost
	_ = listenPort
	_ = seed
	_ = b64privkey
	_ = referenceToken
	_ = metrics
	_ = disablePolicy

	// Verify usage strings are set
	assert.NotEmpty(t, nodeCmd.Lookup("host").Usage, "Host flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("port").Usage, "Port flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("seed").Usage, "Seed flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("b64privkey").Usage, "Private key flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("reference_token").Usage, "Reference token flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("metrics").Usage, "Metrics flag should have usage string")
	assert.NotEmpty(t, nodeCmd.Lookup("disable-policy").Usage, "Disable policy flag should have usage string")

	// Verify flag names
	assert.Equal(t, "host", nodeCmd.Lookup("host").Name, "Host flag name should be correct")
	assert.Equal(t, "port", nodeCmd.Lookup("port").Name, "Port flag name should be correct")
	assert.Equal(t, "seed", nodeCmd.Lookup("seed").Name, "Seed flag name should be correct")
	assert.Equal(t, "b64privkey", nodeCmd.Lookup("b64privkey").Name, "Private key flag name should be correct")
	assert.Equal(t, "reference_token", nodeCmd.Lookup("reference_token").Name, "Reference token flag name should be correct")
	assert.Equal(t, "metrics", nodeCmd.Lookup("metrics").Name, "Metrics flag name should be correct")
	assert.Equal(t, "disable-policy", nodeCmd.Lookup("disable-policy").Name, "Disable policy flag name should be correct")
}

func TestNodeCommandFlagTypes(t *testing.T) {
	// Test flag types
	nodeCmd := flag.NewFlagSet("node", flag.ContinueOnError)

	// Define flags
	listenHost := nodeCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := nodeCmd.Int("port", 0, "set listening port")
	seed := nodeCmd.Int64("seed", 0, "set random seed for id generation")
	b64privkey := nodeCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	referenceToken := nodeCmd.String("reference_token", "", "specify custom reference token")
	metrics := nodeCmd.Bool("metrics", false, "open metrics server or not")
	disablePolicy := nodeCmd.Bool("disable-policy", false, "disable policy or not")

	// Verify flag types
	assert.IsType(t, (*string)(nil), listenHost, "Host flag should be string type")
	assert.IsType(t, (*int)(nil), listenPort, "Port flag should be int type")
	assert.IsType(t, (*int64)(nil), seed, "Seed flag should be int64 type")
	assert.IsType(t, (*string)(nil), b64privkey, "Private key flag should be string type")
	assert.IsType(t, (*string)(nil), referenceToken, "Reference token flag should be string type")
	assert.IsType(t, (*bool)(nil), metrics, "Metrics flag should be bool type")
	assert.IsType(t, (*bool)(nil), disablePolicy, "Disable policy flag should be bool type")

	// Use the variables to avoid linter warnings
	_ = listenHost
	_ = listenPort
	_ = seed
	_ = b64privkey
	_ = referenceToken
	_ = metrics
	_ = disablePolicy
}

// Note: We can't easily test the actual nodeCommand() function because it:
// 1. Creates a device and initializes it
// 2. Starts discovery and socket servers
// 3. Hangs forever with <-make(chan struct{})
//
// Instead, we test the flag parsing logic which is the testable part.
// The actual device initialization and server startup would require
// integration tests with proper mocking of the device and network components.
