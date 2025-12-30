package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"stellar/core/constant"
	"stellar/p2p/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("generates unique keys", func(t *testing.T) {
		// Generate multiple default configs and verify keys are unique
		cfg1 := DefaultConfig()
		cfg2 := DefaultConfig()
		cfg3 := DefaultConfig()

		// All should have non-empty keys
		assert.NotEmpty(t, cfg1.B64PrivKey, "First config should have a generated key")
		assert.NotEmpty(t, cfg2.B64PrivKey, "Second config should have a generated key")
		assert.NotEmpty(t, cfg3.B64PrivKey, "Third config should have a generated key")

		// Keys should be different (very high probability)
		assert.NotEqual(t, cfg1.B64PrivKey, cfg2.B64PrivKey, "Keys should be unique")
		assert.NotEqual(t, cfg1.B64PrivKey, cfg3.B64PrivKey, "Keys should be unique")
		assert.NotEqual(t, cfg2.B64PrivKey, cfg3.B64PrivKey, "Keys should be unique")
	})

	t.Run("generated key is valid base64", func(t *testing.T) {
		cfg := DefaultConfig()
		require.NotEmpty(t, cfg.B64PrivKey, "Config should have a generated key")

		// Try to decode the key
		privKey, err := identity.DecodePrivateKey(cfg.B64PrivKey)
		assert.NoError(t, err, "Generated key should be valid base64 and decodeable")
		assert.NotNil(t, privKey, "Decoded key should not be nil")
	})

	t.Run("has correct default values", func(t *testing.T) {
		cfg := DefaultConfig()

		assert.Equal(t, "0.0.0.0", cfg.ListenHost, "Default host should be 0.0.0.0")
		assert.Equal(t, 0, cfg.ListenPort, "Default port should be 0")
		assert.Equal(t, false, cfg.Bootstrapper, "Default bootstrapper should be false")
		assert.Equal(t, false, cfg.Relay, "Default relay should be false")
		assert.Equal(t, int64(0), cfg.Seed, "Default seed should be 0")
		assert.NotEmpty(t, cfg.B64PrivKey, "Default should have a generated key")
		assert.Equal(t, "", cfg.ReferenceToken, "Default reference token should be empty")
		assert.Equal(t, false, cfg.Metrics, "Default metrics should be false")
		assert.Equal(t, 5001, cfg.MetricsPort, "Default metrics port should be 5001")
		assert.Equal(t, false, cfg.DisablePolicy, "Default disable policy should be false")
		assert.Equal(t, false, cfg.NoSocketServer, "Default no socket server should be false")
		assert.Equal(t, true, cfg.APIServer, "Default API server should be true")
		assert.Equal(t, 1524, cfg.APIPort, "Default API port should be 1524")
		assert.Equal(t, false, cfg.Debug, "Default debug should be false")
	})
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override StellarPath for testing
	originalPath := constant.STELLAR_PATH
	defer func() {
		constant.STELLAR_PATH = originalPath
	}()
	constant.STELLAR_PATH = tmpDir

	t.Run("save and load config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.ListenHost = "127.0.0.1"
		cfg.ListenPort = 8080
		cfg.Bootstrapper = true
		cfg.Relay = true
		cfg.ReferenceToken = "test-token"
		cfg.Metrics = true
		cfg.MetricsPort = 9090

		// Save config
		err := SaveConfig(cfg)
		require.NoError(t, err, "Should save config without error")

		// Verify file exists
		configPath, err := GetConfigPath()
		require.NoError(t, err)
		_, err = os.Stat(configPath)
		assert.NoError(t, err, "Config file should exist")

		// Load config
		loadedCfg, exists, err := LoadConfig()
		require.NoError(t, err, "Should load config without error")
		assert.True(t, exists, "Config should exist")
		require.NotNil(t, loadedCfg, "Loaded config should not be nil")

		// Verify all fields match
		assert.Equal(t, cfg.ListenHost, loadedCfg.ListenHost, "ListenHost should match")
		assert.Equal(t, cfg.ListenPort, loadedCfg.ListenPort, "ListenPort should match")
		assert.Equal(t, cfg.Bootstrapper, loadedCfg.Bootstrapper, "Bootstrapper should match")
		assert.Equal(t, cfg.Relay, loadedCfg.Relay, "Relay should match")
		assert.Equal(t, cfg.B64PrivKey, loadedCfg.B64PrivKey, "B64PrivKey should match")
		assert.Equal(t, cfg.ReferenceToken, loadedCfg.ReferenceToken, "ReferenceToken should match")
		assert.Equal(t, cfg.Metrics, loadedCfg.Metrics, "Metrics should match")
		assert.Equal(t, cfg.MetricsPort, loadedCfg.MetricsPort, "MetricsPort should match")
	})

	t.Run("load non-existent config returns default", func(t *testing.T) {
		// Ensure config file doesn't exist
		configPath, err := GetConfigPath()
		require.NoError(t, err)
		os.Remove(configPath)

		// Load config
		cfg, exists, err := LoadConfig()
		require.NoError(t, err, "Should not error when config doesn't exist")
		assert.False(t, exists, "Config should not exist")
		require.NotNil(t, cfg, "Should return default config")

		// Verify it's a default config (has generated key)
		assert.NotEmpty(t, cfg.B64PrivKey, "Default config should have generated key")
		assert.Equal(t, "0.0.0.0", cfg.ListenHost, "Should have default host")
	})

	t.Run("preserves generated key when saving", func(t *testing.T) {
		// Load default config (generates key)
		cfg, exists, err := LoadConfig()
		require.NoError(t, err)
		assert.False(t, exists, "Config should not exist initially")
		originalKey := cfg.B64PrivKey
		require.NotEmpty(t, originalKey, "Should have generated key")

		// Modify other fields
		cfg.ListenHost = "192.168.1.1"
		cfg.ListenPort = 9000

		// Save config
		err = SaveConfig(cfg)
		require.NoError(t, err)

		// Load again
		loadedCfg, exists, err := LoadConfig()
		require.NoError(t, err)
		assert.True(t, exists, "Config should exist now")
		assert.Equal(t, originalKey, loadedCfg.B64PrivKey, "Key should be preserved")
		assert.Equal(t, "192.168.1.1", loadedCfg.ListenHost, "Host should be updated")
		assert.Equal(t, 9000, loadedCfg.ListenPort, "Port should be updated")
	})
}

func TestConfigKeyHandling(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := constant.STELLAR_PATH
	defer func() {
		constant.STELLAR_PATH = originalPath
	}()
	constant.STELLAR_PATH = tmpDir

	t.Run("empty key in config file is preserved", func(t *testing.T) {
		// Create config with empty key
		cfg := &Config{
			ListenHost: "0.0.0.0",
			ListenPort: 0,
			B64PrivKey: "",
		}

		err := SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)
		assert.Equal(t, "", loadedCfg.B64PrivKey, "Empty key should be preserved")
	})

	t.Run("custom key in config file is preserved", func(t *testing.T) {
		// Generate a test key
		privKey, err := identity.GeneratePrivateKey(42)
		require.NoError(t, err)
		testKey, err := identity.EncodePrivateKey(privKey)
		require.NoError(t, err)

		cfg := &Config{
			ListenHost: "0.0.0.0",
			ListenPort: 0,
			B64PrivKey: testKey,
		}

		err = SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)
		assert.Equal(t, testKey, loadedCfg.B64PrivKey, "Custom key should be preserved")
	})

	t.Run("key can be decoded and is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		require.NotEmpty(t, cfg.B64PrivKey)

		// Save and load
		err := SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)

		// Verify key can be decoded
		privKey, err := identity.DecodePrivateKey(loadedCfg.B64PrivKey)
		assert.NoError(t, err, "Saved key should be decodable")
		assert.NotNil(t, privKey, "Decoded key should not be nil")
	})
}

func TestConfigEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := constant.STELLAR_PATH
	defer func() {
		constant.STELLAR_PATH = originalPath
	}()
	constant.STELLAR_PATH = tmpDir

	t.Run("handles invalid JSON gracefully", func(t *testing.T) {
		configPath, err := GetConfigPath()
		require.NoError(t, err)

		// Write invalid JSON
		err = os.WriteFile(configPath, []byte("{ invalid json }"), 0644)
		require.NoError(t, err)

		// Try to load
		_, _, err = LoadConfig()
		assert.Error(t, err, "Should error on invalid JSON")
	})

	t.Run("handles empty config file", func(t *testing.T) {
		configPath, err := GetConfigPath()
		require.NoError(t, err)

		// Write empty file
		err = os.WriteFile(configPath, []byte(""), 0644)
		require.NoError(t, err)

		// Try to load
		_, _, err = LoadConfig()
		assert.Error(t, err, "Should error on empty JSON")
	})

	t.Run("handles partial config file", func(t *testing.T) {
		configPath, err := GetConfigPath()
		require.NoError(t, err)

		// Write partial config (only some fields)
		partialCfg := map[string]interface{}{
			"listen_host": "127.0.0.1",
			"listen_port": 8080,
		}
		data, err := json.Marshal(partialCfg)
		require.NoError(t, err)
		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		// Load should succeed with partial config
		cfg, exists, err := LoadConfig()
		require.NoError(t, err, "Should load partial config")
		assert.True(t, exists, "Config should exist")
		assert.Equal(t, "127.0.0.1", cfg.ListenHost, "Should have set host")
		assert.Equal(t, 8080, cfg.ListenPort, "Should have set port")
		// Other fields should have zero values
		assert.Equal(t, "", cfg.B64PrivKey, "B64PrivKey should be empty if not in file")
	})

	t.Run("handles directory creation", func(t *testing.T) {
		// Use a nested path that doesn't exist
		nestedDir := filepath.Join(tmpDir, "nested", "path")
		constant.STELLAR_PATH = nestedDir

		cfg := DefaultConfig()
		err := SaveConfig(cfg)
		assert.NoError(t, err, "Should create directory structure")

		// Verify config file was created (which means directory was created)
		configPath, err := GetConfigPath()
		require.NoError(t, err)
		_, err = os.Stat(configPath)
		assert.NoError(t, err, "Config file should exist (directory was created)")

		// Verify parent directory exists by checking its parent
		parentDir := filepath.Dir(configPath)
		_, err = os.Stat(parentDir)
		assert.NoError(t, err, "Parent directory should be created")
	})

	t.Run("handles all boolean flags", func(t *testing.T) {
		cfg := &Config{
			Bootstrapper:   true,
			Relay:          true,
			Metrics:        true,
			DisablePolicy:  true,
			NoSocketServer: true,
			APIServer:      true,
			Debug:          true,
		}

		err := SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)
		assert.True(t, loadedCfg.Bootstrapper, "Bootstrapper should be preserved")
		assert.True(t, loadedCfg.Relay, "Relay should be preserved")
		assert.True(t, loadedCfg.Metrics, "Metrics should be preserved")
		assert.True(t, loadedCfg.DisablePolicy, "DisablePolicy should be preserved")
		assert.True(t, loadedCfg.NoSocketServer, "NoSocketServer should be preserved")
		assert.True(t, loadedCfg.APIServer, "APIServer should be preserved")
		assert.True(t, loadedCfg.Debug, "Debug should be preserved")
	})

	t.Run("handles all integer fields", func(t *testing.T) {
		cfg := &Config{
			ListenPort:  12345,
			Seed:        999,
			MetricsPort: 6789,
			APIPort:     5432,
		}

		err := SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)
		assert.Equal(t, 12345, loadedCfg.ListenPort, "ListenPort should be preserved")
		assert.Equal(t, int64(999), loadedCfg.Seed, "Seed should be preserved")
		assert.Equal(t, 6789, loadedCfg.MetricsPort, "MetricsPort should be preserved")
		assert.Equal(t, 5432, loadedCfg.APIPort, "APIPort should be preserved")
	})

	t.Run("handles all string fields", func(t *testing.T) {
		cfg := &Config{
			ListenHost:     "192.168.1.100",
			B64PrivKey:     "test-key-123",
			ReferenceToken: "my-reference-token",
		}

		err := SaveConfig(cfg)
		require.NoError(t, err)

		loadedCfg, _, err := LoadConfig()
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.100", loadedCfg.ListenHost, "ListenHost should be preserved")
		assert.Equal(t, "test-key-123", loadedCfg.B64PrivKey, "B64PrivKey should be preserved")
		assert.Equal(t, "my-reference-token", loadedCfg.ReferenceToken, "ReferenceToken should be preserved")
	})
}

func TestGetConfigPath(t *testing.T) {
	t.Run("returns valid path", func(t *testing.T) {
		path, err := GetConfigPath()
		assert.NoError(t, err, "Should not error")
		assert.NotEmpty(t, path, "Path should not be empty")
		assert.Contains(t, path, "config.json", "Path should contain config.json")
	})

	t.Run("path is absolute", func(t *testing.T) {
		path, err := GetConfigPath()
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(path), "Path should be absolute")
	})
}

func TestConfigRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := constant.STELLAR_PATH
	defer func() {
		constant.STELLAR_PATH = originalPath
	}()
	constant.STELLAR_PATH = tmpDir

	t.Run("full config round trip", func(t *testing.T) {
		// Create a comprehensive config
		originalCfg := DefaultConfig()
		originalCfg.ListenHost = "10.0.0.1"
		originalCfg.ListenPort = 5000
		originalCfg.Bootstrapper = true
		originalCfg.Relay = true
		originalCfg.Seed = 12345
		originalCfg.ReferenceToken = "round-trip-token"
		originalCfg.Metrics = true
		originalCfg.MetricsPort = 6000
		originalCfg.DisablePolicy = true
		originalCfg.NoSocketServer = true
		originalCfg.APIServer = false
		originalCfg.APIPort = 7000
		originalCfg.Debug = true

		// Save
		err := SaveConfig(originalCfg)
		require.NoError(t, err)

		// Load
		loadedCfg, exists, err := LoadConfig()
		require.NoError(t, err)
		assert.True(t, exists, "Config should exist")

		// Compare all fields
		assert.Equal(t, originalCfg.ListenHost, loadedCfg.ListenHost)
		assert.Equal(t, originalCfg.ListenPort, loadedCfg.ListenPort)
		assert.Equal(t, originalCfg.Bootstrapper, loadedCfg.Bootstrapper)
		assert.Equal(t, originalCfg.Relay, loadedCfg.Relay)
		assert.Equal(t, originalCfg.Seed, loadedCfg.Seed)
		assert.Equal(t, originalCfg.B64PrivKey, loadedCfg.B64PrivKey)
		assert.Equal(t, originalCfg.ReferenceToken, loadedCfg.ReferenceToken)
		assert.Equal(t, originalCfg.Metrics, loadedCfg.Metrics)
		assert.Equal(t, originalCfg.MetricsPort, loadedCfg.MetricsPort)
		assert.Equal(t, originalCfg.DisablePolicy, loadedCfg.DisablePolicy)
		assert.Equal(t, originalCfg.NoSocketServer, loadedCfg.NoSocketServer)
		assert.Equal(t, originalCfg.APIServer, loadedCfg.APIServer)
		assert.Equal(t, originalCfg.APIPort, loadedCfg.APIPort)
		assert.Equal(t, originalCfg.Debug, loadedCfg.Debug)
	})
}
