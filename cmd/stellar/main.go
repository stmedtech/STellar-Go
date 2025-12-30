package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"stellar/core/constant"

	golog "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger = golog.Logger("stellar-cli")

const help = `Stellar cli`

var (
	logFile        *os.File
	originalStdout *os.File
	originalStderr *os.File
)

// dualWriter writes to both terminal (primary) and log file (secondary)
type dualWriter struct {
	terminal *os.File
	file     *os.File
}

func (w *dualWriter) Write(p []byte) (n int, err error) {
	// Write to terminal first (primary)
	n1, err1 := w.terminal.Write(p)
	// Write to file (secondary) - ignore errors to ensure terminal always works
	_, err2 := w.file.Write(p)

	// Return the number of bytes written (use terminal's count)
	if err1 != nil {
		return n1, err1
	}
	// If file write failed, ignore it (can't log here as it would cause recursion)
	_ = err2
	// Return terminal's write count
	return n1, nil
}

func (w *dualWriter) Close() error {
	return w.file.Close()
}

func setupLogFile() error {
	// Save original stdout and stderr before redirecting
	originalStdout = os.Stdout
	originalStderr = os.Stderr

	stellarPath, err := constant.StellarPath()
	if err != nil {
		return fmt.Errorf("failed to get stellar path: %w", err)
	}

	// Ensure stellar directory exists
	if err := os.MkdirAll(stellarPath, 0755); err != nil {
		return fmt.Errorf("failed to create stellar directory: %w", err)
	}

	// Create logs subdirectory
	logsPath := filepath.Join(stellarPath, "logs")
	if err := os.MkdirAll(logsPath, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	logFileName := fmt.Sprintf("stellar_%s.log", timestamp)
	logFilePath := filepath.Join(logsPath, logFileName)

	var fileErr error
	logFile, fileErr = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if fileErr != nil {
		return fmt.Errorf("failed to create log file: %w", fileErr)
	}

	// Create dual writers that write to both terminal (primary) and file (secondary)
	stdoutDual := &dualWriter{terminal: originalStdout, file: logFile}
	stderrDual := &dualWriter{terminal: originalStderr, file: logFile}

	// Use os.NewFile to create file descriptors from our dual writers
	// We need to use a pipe-based approach but with immediate writing
	// Actually, we can't use os.NewFile with a custom writer, so we need pipes

	// Create pipes for stdout and stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Redirect stdout and stderr to pipe writers
	os.Stdout = stdoutW
	os.Stderr = stderrW

	// Start goroutines to copy from pipes to both terminal (primary) and log file (secondary)
	// This captures all output including GIN logs which write to stderr
	// Use immediate flushing to ensure logs are written to file promptly
	go func() {
		defer stdoutR.Close()
		buf := make([]byte, 4096)
		for {
			n, err := stdoutR.Read(buf)
			if n > 0 {
				// Write to both terminal and file immediately
				stdoutDual.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		defer stderrR.Close()
		buf := make([]byte, 4096)
		for {
			n, err := stderrR.Read(buf)
			if n > 0 {
				// Write to both terminal and file immediately
				stderrDual.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
	}()

	// Configure zap to write to both terminal (primary) and log file (secondary)
	// go-log uses zap under the hood, so we need to configure zap's core
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	// Create a MultiWriter for zap that writes to both terminal and file
	zapMultiWriter := io.MultiWriter(originalStderr, logFile)
	core := zapcore.NewCore(encoder, zapcore.AddSync(zapMultiWriter), zapcore.DebugLevel)

	// Set the primary core for go-log
	// This will affect all loggers using go-log
	golog.SetPrimaryCore(core)

	// Write initial message to both terminal and log file
	fmt.Fprintf(os.Stdout, "=== Stellar started at %s ===\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stdout, "Logging to file: %s\n", logFilePath)

	return nil
}

func main() {
	golog.SetLogLevelRegex("stellar.*", "info")

	// golog.SetAllLoggers(golog.LevelInfo)

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

	args := os.Args
	subCommand := ""
	if len(os.Args) < 2 {
		logger.Warn("expected 'key', 'node', 'gui', 'conda' subcommands")
		// os.Exit(1)
		subCommand = "node"
		args = os.Args[1:]
	} else {
		subCommand = args[1]
		args = args[2:]
	}

	// Set up log file before doing anything else
	// This redirects all stdout, stderr, and logs to both terminal (primary) and file (secondary)
	if subCommand == "node" || subCommand == "gui" {
		if err := setupLogFile(); err != nil {
			// If log file setup fails, we can't use logger yet, so use fmt to stderr
			fmt.Fprintf(os.Stderr, "Failed to set up log file: %v, continuing with console logging only\n", err)
			// Continue execution - logging will go to console only
		} else {
			// Ensure log file is closed on exit
			defer func() {
				if logFile != nil {
					fmt.Fprintf(os.Stdout, "=== Stellar exited at %s ===\n", time.Now().Format(time.RFC3339))
					logFile.Close()
				}
			}()
		}
	}

	switch subCommand {
	case "key":
		keyCommand(args)
	case "node":
		nodeCommand(args)
	case "gui":
		guiCommand(args)
	case "conda":
		condaCommand(args)
	default:
		logger.Fatalf("unknown command: %s", subCommand)
	}
}
