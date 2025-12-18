package conda

import (
	"os"
	"testing"

	golog "github.com/ipfs/go-log/v2"
)

// InDocker returns true if running inside a Docker container
func InDocker() bool {
	// Check for Docker-specific environment variables
	if os.Getenv("IN_DOCKER") == "true" || os.Getenv("CONDATEST_ENABLED") == "true" {
		return true
	}

	// Check for .dockerenv file (Docker creates this)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	return false
}

// ShouldRunCondaTests returns true if conda tests should run
// Tests should run if:
// 1. We're in Docker (conda is available)
// 2. Or if CONDATEST_ENABLED is explicitly set
func ShouldRunCondaTests() bool {
	return InDocker() || os.Getenv("CONDATEST_ENABLED") == "true"
}

// InitTestLogging initializes logging for tests
// Call this in TestMain or at the start of tests that need logging
func InitTestLogging(t *testing.T) {
	// Enable debug logging for conda manager in tests
	_ = golog.SetLogLevelRegex("stellar-conda.*", "debug")
	// Also enable info level for other stellar loggers
	_ = golog.SetLogLevelRegex("stellar.*", "info")
}
