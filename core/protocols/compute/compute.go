package compute

import (
	"stellar/p2p/node"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-core-protocols-compute")

// BindComputeStream is a temporary stub for the compute protocol.
// This will be replaced with the new implementation in Phase 6.
func BindComputeStream(n *node.Node) {
	logger.Warn("Compute protocol is not yet implemented - this is a stub")
	// TODO: Implement in Phase 6
}

// The following functions are temporarily removed as part of Phase 0 cleanup.
// They will be replaced with new raw command execution API in later phases.
//
// - ListCondaPythonEnvs
// - PrepareCondaPython
// - ExecuteCondaPythonScript
// - ExecuteCondaPythonWorkspace
// - CancelRun
