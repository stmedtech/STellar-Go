package conda

import (
	"sync"

	"stellar/p2p/protocols/compute/service"
)

var (
	initOnce    sync.Once
	initialized bool
	initMu      sync.Mutex
)

// init registers CondaOperations creator with service package
// This breaks the import cycle: conda imports service, but service doesn't import conda
// Using sync.Once ensures it only registers once even if package is imported multiple times
func init() {
	initOnce.Do(registerCreator)
}

// Initialize explicitly registers the conda operations creator
// This can be called from tests to ensure initialization happens
// Safe to call multiple times (uses sync.Once internally)
func Initialize() {
	initOnce.Do(registerCreator)
}

// IsInitialized returns whether the conda operations creator has been registered
func IsInitialized() bool {
	initMu.Lock()
	defer initMu.Unlock()
	return initialized
}

func registerCreator() {
	// Register creator that returns CondaHandler (which implements HandleSubcommand)
	// This allows server to use shared handler pattern
	service.SetCondaOperationsCreator(func(condaPath string) (interface{}, error) {
		ops, err := NewCondaOperations(condaPath)
		if err != nil {
			return nil, err
		}
		// Return handler that wraps operations (implements HandleSubcommand interface)
		return NewCondaHandler(ops), nil
	})
	initMu.Lock()
	initialized = true
	initMu.Unlock()
}
