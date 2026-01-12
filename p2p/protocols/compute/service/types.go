package service

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// Executor defines the interface for executing raw commands
type Executor interface {
	ExecuteRaw(ctx context.Context, req RawExecutionRequest) (*RawExecution, error)
}

// RawExecutionRequest contains the parameters for executing a raw command
type RawExecutionRequest struct {
	Command    string            // Command to execute
	Args       []string          // Command arguments
	Env        map[string]string // Environment variables (optional)
	WorkingDir string            // Working directory (optional)
	Stdin      io.Reader         // Stdin source (nil if no stdin)
}

// RawExecution represents an active command execution
type RawExecution struct {
	RunID    string             // Unique execution ID
	Stdin    io.WriteCloser     // Write to process stdin
	Stdout   io.ReadCloser      // Read from process stdout
	Stderr   io.ReadCloser      // Read from process stderr
	Log      io.ReadCloser      // Read merged logs (stdout + stderr with timestamps)
	Done     <-chan error       // Signals completion (error or nil)
	ExitCode <-chan int         // Exit code when done
	Cancel   context.CancelFunc // Cancel function to stop execution
}

// generateRunID generates a unique run ID
func generateRunID() string {
	return uuid.New().String()
}
