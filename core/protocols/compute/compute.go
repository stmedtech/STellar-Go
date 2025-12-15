package compute

import (
	"stellar/p2p/node"
	p2p_compute "stellar/p2p/protocols/compute"
)

// BindComputeStream binds the compute protocol stream handler to the libp2p host.
func BindComputeStream(n *node.Node) {
	p2p_compute.BindComputeStream(n)
}

// The following functions are temporarily removed as part of Phase 0 cleanup.
// They will be replaced with new raw command execution API in later phases.
//
// - ListCondaPythonEnvs
// - PrepareCondaPython
// - ExecuteCondaPythonScript
// - ExecuteCondaPythonWorkspace
// - CancelRun
