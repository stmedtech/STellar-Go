package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"stellar/core/constant"
	"stellar/p2p/identity"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-core-config")

var (
	instance     *Config
	instanceOnce sync.Once
	instanceMu   sync.RWMutex
)

type Config struct {
	// Connection
	ListenHost string `json:"listen_host,omitempty"`
	ListenPort int    `json:"listen_port,omitempty"`

	// Bootstrapper settings
	Bootstrapper bool `json:"bootstrapper,omitempty"`
	Relay        bool `json:"relay,omitempty"`

	// Key
	Seed       int64  `json:"seed,omitempty"`
	B64PrivKey string `json:"b64privkey,omitempty"`

	// Miscellaneous
	ReferenceToken string   `json:"reference_token,omitempty"`
	Metrics        bool     `json:"metrics,omitempty"`
	MetricsPort    int      `json:"metrics_port,omitempty"`
	DisablePolicy  bool     `json:"disable_policy,omitempty"`
	NoSocketServer bool     `json:"no_socket,omitempty"`
	APIServer      bool     `json:"api,omitempty"`
	APIPort        int      `json:"api_port,omitempty"`
	Debug          bool     `json:"debug,omitempty"`
	WhiteList      []string `json:"whitelist,omitempty"`
	DataDir        string   `json:"data_dir,omitempty"`
}

func defaultDataDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		logger.Warnf("Failed to get working directory: %v", err)
		return ""
	}
	return filepath.Join(pwd, "data")
}

func DefaultConfig() *Config {
	// Generate a new private key for the default config
	privKey, err := identity.GeneratePrivateKey(0)
	var b64PrivKey string
	if err != nil {
		logger.Warnf("Failed to generate private key for default config: %v, using empty key", err)
		b64PrivKey = ""
	} else {
		encodedKey, encodeErr := identity.EncodePrivateKey(privKey)
		if encodeErr != nil {
			logger.Warnf("Failed to encode private key for default config: %v, using empty key", encodeErr)
			b64PrivKey = ""
		} else {
			b64PrivKey = encodedKey
		}
	}

	return &Config{
		ListenHost:     "0.0.0.0",
		ListenPort:     0,
		Bootstrapper:   false,
		Relay:          false,
		Seed:           0,
		B64PrivKey:     b64PrivKey,
		ReferenceToken: "",
		Metrics:        false,
		MetricsPort:    5001,
		DisablePolicy:  false,
		NoSocketServer: false,
		APIServer:      true,
		APIPort:        1524,
		Debug:          false,
		WhiteList:      []string{},
		DataDir:        defaultDataDir(),
	}
}

func GetConfigPath() (string, error) {
	stellarPath, err := constant.StellarPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(stellarPath, "config.json"), nil
}

// GetInstance returns the singleton config instance, initializing it if necessary
// This function is thread-safe and can be called from anywhere
// The returned instance should be treated as read-only unless you're calling Save()
func GetInstance() *Config {
	instanceOnce.Do(func() {
		cfg, _, err := loadConfigFromFile()
		if err != nil {
			logger.Warnf("Failed to load config: %v, using defaults", err)
			instance = DefaultConfig()
		} else {
			instance = cfg
		}
	})
	// Return a reference to the singleton
	// Note: Direct field modifications are not thread-safe
	// Use Save() or SyncWhiteList() for thread-safe updates
	return instance
}

// loadConfigFromFile loads config from file (internal function, not singleton-aware)
func loadConfigFromFile() (*Config, bool, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, false, err
	}

	// If config file doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Infof("Config file not found at %s, using defaults", configPath)
		return DefaultConfig(), false, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false, err
	}

	// Start with defaults, then overlay json
	config := *DefaultConfig()
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, false, err
	}

	return &config, true, nil
}

// LoadConfig loads config from file (deprecated: use GetInstance() for singleton access)
// Kept for backward compatibility
func LoadConfig() (*Config, bool, error) {
	return loadConfigFromFile()
}

// saveUnlocked saves the config to file without locking (internal use)
func (c *Config) saveUnlocked() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	stellarPath, err := constant.StellarPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stellarPath, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return err
	}

	// Update singleton instance
	instance = c
	return nil
}

// Save saves the singleton config instance to file
func (c *Config) Save() error {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	return c.saveUnlocked()
}

// SaveConfig saves config to file (deprecated: use config.Save() for singleton-aware saving)
// Kept for backward compatibility
func SaveConfig(config *Config) error {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	// Update singleton instance before saving
	instance = config
	return config.saveUnlocked()
}

// SyncWhiteList syncs only the whitelist to the config file
func (c *Config) SyncWhiteList(whitelist []string) error {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	// Ensure instance is initialized (GetInstance() is safe to call even if already initialized)
	cfg := GetInstance()

	// Update the singleton instance's whitelist
	cfg.WhiteList = whitelist
	return cfg.saveUnlocked()
}
