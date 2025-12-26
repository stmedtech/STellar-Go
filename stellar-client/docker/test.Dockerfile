# Dockerfile for running stellar-client tests with stellar-go daemon
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build stellar binary without GUI dependencies
# Directly create minimal main.go (no GUI) via Dockerfile
RUN mkdir -p ./cmd/stellar && \
    cat > ./cmd/stellar/main.go <<'EOF'
package main

import (
	"flag"
	"os"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-cli")

const help = `Stellar cli`

func main() {
	golog.SetLogLevelRegex("stellar.*", "info")

	flag.Usage = func() {
		logger.Info(help)
		flag.PrintDefaults()
	}

    
	args := os.Args
	subCommand := ""
	if len(os.Args) < 2 {
		logger.Warn("expected 'key', 'bootstrapper', 'node', 'gui', 'conda' subcommands")
		os.Exit(1)
	} else {
		subCommand = args[1]
		args = args[2:]
	}

	switch subCommand {
	case "key":
		keyCommand(args)
	case "bootstrapper":
		bootstrapperCommand(args)
	case "node":
		nodeCommand(args)
	case "conda":
		condaCommand(args)
	default:
		os.Exit(1)
	}
}
EOF
# Remove gui.go to prevent it from being compiled
RUN rm -f ./cmd/stellar/gui.go
# Build with the minimal main (no GUI)
RUN CGO_ENABLED=0 GOOS=linux go build -o stellar \
    -ldflags="-s -w" \
    ./cmd/stellar

# Python test environment
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy stellar binary from builder to a specific location
COPY --from=builder /build/stellar /usr/local/bin/stellar-go
RUN chmod +x /usr/local/bin/stellar-go

# Copy Python client code
COPY stellar-client/ /app/

# Make docker scripts executable
RUN chmod +x /app/docker/generate-bootstrappers.sh /app/docker/docker-test-entrypoint.sh

# Install Python dependencies
WORKDIR /app
# Note: requests-unixsocket not needed - using custom adapter
# Install pytest-timeout for test timeout handling
RUN pip install --no-cache-dir -e .[dev] pytest-timeout

# Create directory for socket
RUN mkdir -p /root/.local/share/stellar

# Create data directory for file protocol (file protocol uses ./data relative to working dir)
RUN mkdir -p /app/data

# Expose port for testing
EXPOSE 4001

# Default command (can be overridden in docker-compose)
CMD ["stellar", "--help"]

