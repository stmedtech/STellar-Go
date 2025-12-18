package conda

import (
	"stellar/p2p/protocols/compute/service"
)

// init registers CondaOperations creator with service package
// This breaks the import cycle: conda imports service, but service doesn't import conda
func init() {
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
}
