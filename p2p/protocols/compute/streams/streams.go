package streams

import "io"

// ExecutionStreamReader is a unified interface for reading from execution streams
// Works with both RawExecution (fields) and CommandExecution (methods)
// This interface is in a separate package to avoid import cycles between service and conda
type ExecutionStreamReader interface {
	GetStdout() io.ReadCloser
	GetStderr() io.ReadCloser
	GetDone() <-chan error
	GetExitCode() <-chan int
}
