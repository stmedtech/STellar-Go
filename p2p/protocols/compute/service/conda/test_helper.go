package conda

import (
	"testing"

	golog "github.com/ipfs/go-log/v2"
)

// InitTestLogging initializes logging for tests
// Call this in TestMain or at the start of tests that need logging
func InitTestLogging(t *testing.T) {
	// Enable debug logging for conda operations in tests
	_ = golog.SetLogLevelRegex("stellar-conda.*", "debug")
	// Also enable info level for other stellar loggers
	_ = golog.SetLogLevelRegex("stellar.*", "info")
}
