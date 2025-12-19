//go:build !conda

package conda

import (
	"sync"

	"stellar/p2p/protocols/compute/service"
)

var (
	mockInitOnce sync.Once
)

// InitializeMockCondaOperations registers the mock conda operations creator
// This should ONLY be called from test files, never in production code
func InitializeMockCondaOperations() {
	mockInitOnce.Do(func() {
		service.SetCondaOperationsCreator(func(condaPath string) (interface{}, error) {
			// Return handler that wraps mock operations (same pattern as real conda)
			return NewCondaHandler(NewMockCondaOperations()), nil
		})
	})
}
