#!/usr/bin/env python3
"""
Test runner for DCFL Client tests.

This script runs tests in the correct protocol usage order:
1. Echo Protocol - Basic connectivity and device discovery
2. File Protocol - File transfer operations
3. Compute Protocol - Computational tasks
4. Proxy Protocol - Network proxying
5. Integration Suite - End-to-end testing

Usage:
    python run_tests.py [--unit] [--integration] [--all] [--verbose]
"""

import sys
import subprocess
import argparse
from pathlib import Path


def run_tests(test_files, verbose=False):
    """Run specified test files."""
    cmd = ["python", "-m", "pytest"]
    
    if verbose:
        cmd.append("-v")
    
    cmd.extend(test_files)
    
    print(f"Running: {' '.join(cmd)}")
    result = subprocess.run(cmd, cwd=Path(__file__).parent)
    return result.returncode


def main():
    """Main test runner function."""
    parser = argparse.ArgumentParser(description="Run DCFL Client tests in protocol order")
    parser.add_argument("--unit", action="store_true", help="Run unit tests only")
    parser.add_argument("--integration", action="store_true", help="Run integration tests only")
    parser.add_argument("--all", action="store_true", help="Run all tests")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--protocol", choices=["echo", "file", "compute", "proxy"], 
                       help="Run tests for specific protocol only")
    
    args = parser.parse_args()
    
    # Define test files in protocol usage order
    test_files = [
        "test_echo_protocol.py",
        "test_file_protocol.py", 
        "test_compute_protocol.py",
        "test_proxy_protocol.py",
        "test_integration_suite.py"
    ]
    
    if args.protocol:
        # Run specific protocol tests
        protocol_map = {
            "echo": "test_echo_protocol.py",
            "file": "test_file_protocol.py",
            "compute": "test_compute_protocol.py",
            "proxy": "test_proxy_protocol.py"
        }
        test_files = [protocol_map[args.protocol]]
        return run_tests(test_files, args.verbose)
    
    elif args.unit:
        # Run unit tests only (exclude integration tests)
        cmd = ["python", "-m", "pytest", "-m", "not integration"]
        if args.verbose:
            cmd.append("-v")
        cmd.extend(test_files)
        
        print(f"Running unit tests: {' '.join(cmd)}")
        result = subprocess.run(cmd, cwd=Path(__file__).parent)
        return result.returncode
    
    elif args.integration:
        # Run integration tests only
        cmd = ["python", "-m", "pytest", "-m", "integration"]
        if args.verbose:
            cmd.append("-v")
        cmd.extend(test_files)
        
        print(f"Running integration tests: {' '.join(cmd)}")
        result = subprocess.run(cmd, cwd=Path(__file__).parent)
        return result.returncode
    
    elif args.all or not any([args.unit, args.integration, args.protocol]):
        # Run all tests in protocol order
        return run_tests(test_files, args.verbose)
    
    return 0


if __name__ == "__main__":
    sys.exit(main())
