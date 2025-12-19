//go:build !test_conda_skip
// +build !test_conda_skip

package service_test

import (
	"stellar/p2p/protocols/compute/service/conda"
)

// init ensures conda package is imported in test builds
// This triggers conda's init() which registers the creator
// Using a separate test package (service_test) allows importing conda without cycles
func init() {
	// Explicitly initialize conda to ensure it's registered
	// This is safe because service_test package can import both service and conda
	conda.Initialize()
}
