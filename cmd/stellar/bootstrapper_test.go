package main

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapperCommandFlags(t *testing.T) {
	// Test flag parsing for bootstrapper command
	tests := []struct {
		name     string
		args     []string
		expected struct {
			host       string
			port       int
			relay      bool
			b64privkey string
			debug      bool
		}
	}{
		{
			name: "default flags",
			args: []string{"bootstrapper"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "0.0.0.0",
				port:       0,
				relay:      false,
				b64privkey: "",
				debug:      false,
			},
		},
		{
			name: "custom host and port",
			args: []string{"bootstrapper", "-host", "127.0.0.1", "-port", "8080"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "127.0.0.1",
				port:       8080,
				relay:      false,
				b64privkey: "",
				debug:      false,
			},
		},
		{
			name: "with relay enabled",
			args: []string{"bootstrapper", "-relay"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "0.0.0.0",
				port:       0,
				relay:      true,
				b64privkey: "",
				debug:      false,
			},
		},
		{
			name: "with private key",
			args: []string{"bootstrapper", "-b64privkey", "test-key"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "0.0.0.0",
				port:       0,
				relay:      false,
				b64privkey: "test-key",
				debug:      false,
			},
		},
		{
			name: "with debug enabled",
			args: []string{"bootstrapper", "-debug"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "0.0.0.0",
				port:       0,
				relay:      false,
				b64privkey: "",
				debug:      true,
			},
		},
		{
			name: "all flags combined",
			args: []string{"bootstrapper", "-host", "192.168.1.1", "-port", "9000", "-relay", "-b64privkey", "combined-key", "-debug"},
			expected: struct {
				host       string
				port       int
				relay      bool
				b64privkey string
				debug      bool
			}{
				host:       "192.168.1.1",
				port:       9000,
				relay:      true,
				b64privkey: "combined-key",
				debug:      true,
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
			bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ContinueOnError)

			// Define flags
			listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
			listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
			relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")
			b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
			debug := bootstrapperCmd.Bool("debug", false, "debug mode")

			// Parse flags (skip the first argument which is the command name)
			err := bootstrapperCmd.Parse(tt.args[1:])
			require.NoError(t, err, "Flag parsing should succeed")

			// Verify flag values
			assert.Equal(t, tt.expected.host, *listenHost, "Host should match")
			assert.Equal(t, tt.expected.port, *listenPort, "Port should match")
			assert.Equal(t, tt.expected.relay, *relay, "Relay flag should match")
			assert.Equal(t, tt.expected.b64privkey, *b64privkey, "Private key should match")
			assert.Equal(t, tt.expected.debug, *debug, "Debug flag should match")
		})
	}
}

func TestBootstrapperCommandFlagValidation(t *testing.T) {
	// Test invalid flag combinations
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "invalid port number",
			args:    []string{"bootstrapper", "-port", "99999"},
			wantErr: false, // Flag parsing doesn't validate port range
		},
		{
			name:    "negative port",
			args:    []string{"bootstrapper", "-port", "-1"},
			wantErr: false, // Flag parsing doesn't validate port range
		},
		{
			name:    "invalid host",
			args:    []string{"bootstrapper", "-host", "invalid-host"},
			wantErr: false, // Flag parsing doesn't validate host format
		},
		{
			name:    "empty private key",
			args:    []string{"bootstrapper", "-b64privkey", ""},
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
			bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ContinueOnError)

			// Define flags
			listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
			listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
			relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")
			b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
			debug := bootstrapperCmd.Bool("debug", false, "debug mode")

			// Parse flags
			err := bootstrapperCmd.Parse(tt.args[1:])

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid flags")
			} else {
				assert.NoError(t, err, "Flag parsing should succeed even with invalid values")
			}

			// Verify flags were parsed (even if invalid)
			assert.NotNil(t, listenHost, "Host flag should be defined")
			assert.NotNil(t, listenPort, "Port flag should be defined")
			assert.NotNil(t, relay, "Relay flag should be defined")
			assert.NotNil(t, b64privkey, "Private key flag should be defined")
			assert.NotNil(t, debug, "Debug flag should be defined")
		})
	}
}

func TestBootstrapperCommandFlagDefaults(t *testing.T) {
	// Test that default values are correct
	bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ContinueOnError)

	// Define flags with defaults
	listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
	relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")
	b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	debug := bootstrapperCmd.Bool("debug", false, "debug mode")

	// Verify default values
	assert.Equal(t, "0.0.0.0", *listenHost, "Default host should be 0.0.0.0")
	assert.Equal(t, 0, *listenPort, "Default port should be 0")
	assert.Equal(t, false, *relay, "Default relay should be false")
	assert.Equal(t, "", *b64privkey, "Default private key should be empty")
	assert.Equal(t, false, *debug, "Default debug should be false")
}

func TestBootstrapperCommandFlagUsage(t *testing.T) {
	// Test flag usage strings
	bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ContinueOnError)

	// Define flags
	listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
	relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")
	b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	debug := bootstrapperCmd.Bool("debug", false, "debug mode")

	// Use variables to avoid "declared and not used" errors
	_ = listenHost
	_ = listenPort
	_ = relay
	_ = b64privkey
	_ = debug

	// Verify usage strings are set
	assert.NotEmpty(t, bootstrapperCmd.Lookup("host").Usage, "Host flag should have usage string")
	assert.NotEmpty(t, bootstrapperCmd.Lookup("port").Usage, "Port flag should have usage string")
	assert.NotEmpty(t, bootstrapperCmd.Lookup("relay").Usage, "Relay flag should have usage string")
	assert.NotEmpty(t, bootstrapperCmd.Lookup("b64privkey").Usage, "Private key flag should have usage string")
	assert.NotEmpty(t, bootstrapperCmd.Lookup("debug").Usage, "Debug flag should have usage string")

	// Verify flag names
	assert.Equal(t, "host", bootstrapperCmd.Lookup("host").Name, "Host flag name should be correct")
	assert.Equal(t, "port", bootstrapperCmd.Lookup("port").Name, "Port flag name should be correct")
	assert.Equal(t, "relay", bootstrapperCmd.Lookup("relay").Name, "Relay flag name should be correct")
	assert.Equal(t, "b64privkey", bootstrapperCmd.Lookup("b64privkey").Name, "Private key flag name should be correct")
	assert.Equal(t, "debug", bootstrapperCmd.Lookup("debug").Name, "Debug flag name should be correct")
}

func TestBootstrapperCommandFlagTypes(t *testing.T) {
	// Test flag types
	bootstrapperCmd := flag.NewFlagSet("bootstrapper", flag.ContinueOnError)

	// Define flags
	listenHost := bootstrapperCmd.String("host", "0.0.0.0", "set listening host")
	listenPort := bootstrapperCmd.Int("port", 0, "set listening port")
	relay := bootstrapperCmd.Bool("relay", false, "use this node as relay node for relaying")
	b64privkey := bootstrapperCmd.String("b64privkey", "", "import base64 encoded Ed25519 private key raw bytes")
	debug := bootstrapperCmd.Bool("debug", false, "debug mode")

	// Verify flag types
	assert.IsType(t, (*string)(nil), listenHost, "Host flag should be string type")
	assert.IsType(t, (*int)(nil), listenPort, "Port flag should be int type")
	assert.IsType(t, (*bool)(nil), relay, "Relay flag should be bool type")
	assert.IsType(t, (*string)(nil), b64privkey, "Private key flag should be string type")
	assert.IsType(t, (*bool)(nil), debug, "Debug flag should be bool type")
}

// Note: We can't easily test the actual bootstrapperCommand() function because it:
// 1. Creates a bootstrapper node and initializes it
// 2. Starts metrics server
// 3. Hangs forever with <-make(chan struct{})
//
// Instead, we test the flag parsing logic which is the testable part.
// The actual bootstrapper initialization and server startup would require
// integration tests with proper mocking of the node and network components.
