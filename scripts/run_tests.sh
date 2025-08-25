#!/bin/bash

# Test runner script for Stellar Go project
# This script runs all tests with coverage and generates reports

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_DIR="coverage"
TEST_TIMEOUT="5m"
VERBOSE=false
COVERAGE=false
BENCHMARK=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -b|--benchmark)
            BENCHMARK=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  -v, --verbose     Run tests in verbose mode"
            echo "  -c, --coverage    Generate coverage report"
            echo "  -b, --benchmark   Run benchmarks"
            echo "  -h, --help        Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
    shift
done

echo -e "${BLUE}=== Stellar Go Test Runner ===${NC}"
echo

# Create coverage directory if needed
if [ "$COVERAGE" = true ]; then
    mkdir -p "$COVERAGE_DIR"
fi

# Function to run tests for a specific package
run_package_tests() {
    local package=$1
    local test_args=""
    
    if [ "$VERBOSE" = true ]; then
        test_args="-v"
    fi
    
    if [ "$COVERAGE" = true ]; then
        test_args="$test_args -coverprofile=$COVERAGE_DIR/${package//\//_}.out"
    fi
    
    echo -e "${YELLOW}Testing package: $package${NC}"
    
    if go test $test_args -timeout=$TEST_TIMEOUT ./$package; then
        echo -e "${GREEN}✓ $package tests passed${NC}"
        return 0
    else
        echo -e "${RED}✗ $package tests failed${NC}"
        return 1
    fi
}

# Function to run benchmarks
run_benchmarks() {
    local package=$1
    
    echo -e "${YELLOW}Running benchmarks for: $package${NC}"
    
    if go test -bench=. -benchmem ./$package; then
        echo -e "${GREEN}✓ $package benchmarks completed${NC}"
        return 0
    else
        echo -e "${RED}✗ $package benchmarks failed${NC}"
        return 1
    fi
}

# Main test execution
failed_packages=()
passed_packages=()

echo -e "${BLUE}Running unit tests...${NC}"
echo

# Test packages in order of dependency
packages=(
    "p2p/identity"
    "p2p/policy"
    "p2p/protocols/file"
    "pkg/testutils"
)

for package in "${packages[@]}"; do
    if run_package_tests "$package"; then
        passed_packages+=("$package")
    else
        failed_packages+=("$package")
    fi
    echo
done

# Run benchmarks if requested
if [ "$BENCHMARK" = true ]; then
    echo -e "${BLUE}Running benchmarks...${NC}"
    echo
    
    for package in "${packages[@]}"; do
        if run_benchmarks "$package"; then
            echo -e "${GREEN}✓ $package benchmarks passed${NC}"
        else
            echo -e "${RED}✗ $package benchmarks failed${NC}"
        fi
        echo
    done
fi

# Generate coverage report if requested
if [ "$COVERAGE" = true ] && [ ${#failed_packages[@]} -eq 0 ]; then
    echo -e "${BLUE}Generating coverage report...${NC}"
    
    # Combine coverage files
    if ls $COVERAGE_DIR/*.out 1> /dev/null 2>&1; then
        go tool cover -html=$COVERAGE_DIR/*.out -o $COVERAGE_DIR/coverage.html
        echo -e "${GREEN}Coverage report generated: $COVERAGE_DIR/coverage.html${NC}"
        
        # Show coverage summary
        go tool cover -func=$COVERAGE_DIR/*.out | tail -1
    fi
fi

# Summary
echo
echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "${GREEN}Passed: ${#passed_packages[@]}${NC}"
echo -e "${RED}Failed: ${#failed_packages[@]}${NC}"

if [ ${#passed_packages[@]} -gt 0 ]; then
    echo -e "${GREEN}Passed packages:${NC}"
    for package in "${passed_packages[@]}"; do
        echo -e "  ✓ $package"
    done
fi

if [ ${#failed_packages[@]} -gt 0 ]; then
    echo -e "${RED}Failed packages:${NC}"
    for package in "${failed_packages[@]}"; do
        echo -e "  ✗ $package"
    done
    exit 1
fi

echo
echo -e "${GREEN}All tests passed! 🎉${NC}"
