# Makefile for Stellar Go project

# Variables
BINARY_NAME=stellar
BUILD_DIR=build
COVERAGE_DIR=coverage
TEST_TIMEOUT=5m

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOLINT=golangci-lint

# Frontend build directory
FRONTEND_DIR=frontend
FRONTEND_DIST=$(FRONTEND_DIR)/dist

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(shell git describe --tags --always --dirty) -X main.BuildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')"

.PHONY: all build clean test test-verbose test-coverage test-benchmark lint fmt vet deps help

# Default target
all: clean build

# Build the application
build: build-frontend
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/stellar
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build frontend (optional - will not fail the build if frontend build fails)
build-frontend:
	@echo "Building frontend (optional)..."
	@mkdir -p $(FRONTEND_DIST)
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		if command -v npm >/dev/null 2>&1; then \
			cd $(FRONTEND_DIR) && npm install && npm run build || echo "Warning: Frontend build failed, continuing without frontend..."; \
		else \
			echo "Warning: npm not found, skipping frontend build"; \
		fi \
	else \
		echo "Frontend directory not found or package.json missing, skipping frontend build"; \
	fi
	@if [ ! -f "$(FRONTEND_DIST)/index.html" ]; then \
		echo "Creating placeholder file for embed..."; \
		touch $(FRONTEND_DIST)/index.html; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -timeout=$(TEST_TIMEOUT) ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) -v -timeout=$(TEST_TIMEOUT) ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -timeout=$(TEST_TIMEOUT) -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	$(GOCMD) tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo "Coverage report generated: $(COVERAGE_DIR)/coverage.html"

# Run benchmarks
test-benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Run all tests (unit, coverage, benchmark)
test-all: test test-coverage test-benchmark

# Run tests using the test runner script
test-script:
	@echo "Running tests with script..."
	@chmod +x scripts/run_tests.sh
	./scripts/run_tests.sh -v -c -b

# Lint the code
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

# Format the code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Vet the code
vet:
	@echo "Vetting code..."
	$(GOCMD) vet ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Install dependencies for development
deps-dev:
	@echo "Installing development dependencies..."
	$(GOGET) github.com/stretchr/testify/assert
	$(GOGET) github.com/stretchr/testify/require
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/stellar
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run the application in development mode
run-dev:
	@echo "Running $(BINARY_NAME) in development mode..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/stellar
	./$(BUILD_DIR)/$(BINARY_NAME) node --host 0.0.0.0 --port 4001

# Create a new release build
release: clean
	@echo "Creating release build..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/stellar
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/stellar
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/stellar
	@echo "Release builds complete"

# Install the application
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) ./cmd/stellar
	@echo "Installation complete"

# Uninstall the application
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Uninstallation complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  clean         - Clean build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-verbose  - Run tests with verbose output"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-benchmark- Run benchmarks"
	@echo "  test-all      - Run all tests (unit, coverage, benchmark)"
	@echo "  test-script   - Run tests using the test runner script"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  deps          - Download dependencies"
	@echo "  deps-dev      - Install development dependencies"
	@echo "  run           - Run the application"
	@echo "  run-dev       - Run the application in development mode"
	@echo "  release       - Create release builds for multiple platforms"
	@echo "  install       - Install the application"
	@echo "  uninstall     - Uninstall the application"
	@echo "  help          - Show this help message"

# Development workflow
dev-setup: deps deps-dev fmt lint test
	@echo "Development environment setup complete"

# CI/CD workflow
ci: deps fmt lint test-coverage
	@echo "CI/CD pipeline complete"

# Pre-commit hook
pre-commit: fmt lint test
	@echo "Pre-commit checks passed"
