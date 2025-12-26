#!/bin/bash
set -e

# This script is used in the test-runner container
# The stellar-go node is already running in node1 container with Unix socket enabled
# We just need to wait for device discovery and then run tests

SOCKET_PATH="${STELLAR_SOCKET_PATH:-/root/.local/share/stellar/stellar.sock}"
PYTHON_SCRIPT="/tmp/wait_for_devices.py"

echo "=== Test Environment Setup ==="
echo "Socket path: ${SOCKET_PATH}"

# Wait for socket to be created (node1 should have created it)
echo "Waiting for Stellar socket..."
MAX_WAIT=60
WAITED=0

while [ ! -S "$SOCKET_PATH" ] && [ $WAITED -lt $MAX_WAIT ]; do
    sleep 1
    WAITED=$((WAITED + 1))
    if [ $((WAITED % 5)) -eq 0 ]; then
        echo "Waiting for socket... ($WAITED/$MAX_WAIT)"
    fi
done

if [ ! -S "$SOCKET_PATH" ]; then
    echo "ERROR: Socket not found at ${SOCKET_PATH} after ${MAX_WAIT} seconds"
    ls -la /root/.local/share/stellar/ || echo "Directory does not exist"
    exit 1
fi

echo "✓ Socket found at ${SOCKET_PATH}"

# Create Python script to wait for device discovery
cat > "$PYTHON_SCRIPT" << 'EOF'
import os
import sys
import time
import requests
from requests.adapters import HTTPAdapter
from urllib3.connection import HTTPConnection
from urllib3.connectionpool import HTTPConnectionPool
import socket

class UnixSocketConnection(HTTPConnection):
    def __init__(self, socket_path):
        super().__init__("localhost", port=None)
        self.socket_path = socket_path
    
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.socket_path)

class UnixSocketConnectionPool(HTTPConnectionPool):
    def __init__(self, socket_path):
        super().__init__("localhost", port=None)
        self.socket_path = socket_path
    
    def _new_conn(self):
        return UnixSocketConnection(self.socket_path)

class UnixSocketAdapter(HTTPAdapter):
    def __init__(self, socket_path):
        self.socket_path = socket_path
        self._pool = None
        super().__init__()
    
    def init_poolmanager(self, *args, **kwargs):
        # Always initialize poolmanager for HTTP fallback
        super().init_poolmanager(*args, **kwargs)
    
    def get_connection(self, url, proxies=None):
        # Only use Unix socket for localhost URLs, use parent's HTTP for others
        if "localhost" in url or url.startswith("http://localhost"):
            if self._pool is None:
                self._pool = UnixSocketConnectionPool(self.socket_path)
            return self._pool
        # For other URLs, use parent's HTTP connection
        return super().get_connection(url, proxies)

def wait_for_devices(socket_path, min_devices=2, max_wait=120):
    """Wait for at least min_devices to be discovered."""
    # Skip waiting if min_devices is 0
    if min_devices == 0:
        print("Skipping device discovery wait (MIN_DEVICES=0)")
        return True
    
    import subprocess
    import json
    
    print(f"Waiting for at least {min_devices} devices to be discovered...")
    start_time = time.time()
    last_count = 0
    consecutive_errors = 0
    max_errors = 5
    
    while time.time() - start_time < max_wait:
        try:
            # Use curl which we know works
            result = subprocess.run(
                ['curl', '--unix-socket', socket_path, 'http://localhost/devices'],
                capture_output=True,
                text=True,
                timeout=5
            )
            
            if result.returncode == 0:
                consecutive_errors = 0
                devices = json.loads(result.stdout)
                # /devices returns a dict/object, not a list
                if isinstance(devices, dict):
                    device_count = len(devices)
                elif isinstance(devices, list):
                    device_count = len(devices)
                else:
                    device_count = 0
                
                if device_count != last_count:
                    print(f"  Discovered {device_count} device(s)...")
                    last_count = device_count
                
                # Check if we have enough devices
                if device_count >= min_devices:
                    elapsed = int(time.time() - start_time)
                    print(f"✓ Found {device_count} device(s) after {elapsed} seconds")
                    return True
            else:
                consecutive_errors += 1
                if consecutive_errors <= 3:
                    print(f"  curl returned code {result.returncode}, retrying...")
        except Exception as e:
            consecutive_errors += 1
            if consecutive_errors <= 3:
                print(f"  Error querying devices: {type(e).__name__}: {str(e)[:50]}, retrying...")
            if consecutive_errors >= max_errors:
                print(f"  Too many consecutive errors, giving up")
                break
        
        time.sleep(2)
    
    # Final check: if we found any devices, proceed anyway (they might still be connecting)
    if last_count > 0:
        elapsed = int(time.time() - start_time)
        print(f"⚠ Found {last_count} device(s) after {elapsed} seconds (expected {min_devices}, but proceeding anyway)")
        return True
    
    elapsed = int(time.time() - start_time)
    print(f"✗ Timeout: Only found {last_count} device(s) after {elapsed} seconds (expected at least {min_devices})")
    # If min_devices is 0, proceed anyway (tests might not need devices)
    if min_devices == 0:
        print(f"  Proceeding with {last_count} device(s) (MIN_DEVICES=0, tests may not need devices)")
        return True
    # If we found some devices but not enough, still proceed (they might still be connecting)
    if last_count > 0:
        print(f"  Warning: Proceeding with {last_count} device(s) instead of {min_devices}")
        return True
    return False

if __name__ == "__main__":
    socket_path = os.environ.get("STELLAR_SOCKET_PATH", "/root/.local/share/stellar/stellar.sock")
    min_devices = int(os.environ.get("MIN_DEVICES", "2"))
    max_wait = int(os.environ.get("MAX_WAIT", "120"))
    
    if not os.path.exists(socket_path):
        print(f"ERROR: Socket not found at {socket_path}")
        sys.exit(1)
    
    success = wait_for_devices(socket_path, min_devices, max_wait)
    sys.exit(0 if success else 1)
EOF

# Wait for device discovery using the Python script
echo ""
echo "Waiting for nodes to discover each other..."
python3 "$PYTHON_SCRIPT"
if [ $? -ne 0 ]; then
    echo "ERROR: Device discovery failed or timeout"
    exit 1
fi

echo ""
echo "=== Running Python Client Tests ==="

# Run tests
# Default to running the main test files if no arguments provided
# Arguments passed to docker-compose run will be available here
# Add timeout to prevent hanging tests (30 seconds per test, 10 minutes total)
PYTEST_ARGS="-v --tb=short --timeout=30 --timeout-method=thread"
if [ $# -eq 0 ]; then
    # No arguments - run default test suite
    pytest $PYTEST_ARGS tests/test_client_api.py tests/test_client_api_comprehensive.py
else
    # Skip "pytest" if it's the first argument (docker-compose passes it)
    if [ "$1" = "pytest" ]; then
        shift  # Remove "pytest" from arguments
    fi
    # Pass remaining arguments to pytest, but add timeout if not already specified
    # Check if timeout is already in arguments
    if echo "$@" | grep -q -- "--timeout"; then
        pytest "$@"
    else
        pytest $PYTEST_ARGS "$@"
    fi
fi

TEST_EXIT_CODE=$?

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✓ All tests passed!"
else
    echo "✗ Some tests failed (exit code: $TEST_EXIT_CODE)"
fi

exit $TEST_EXIT_CODE
