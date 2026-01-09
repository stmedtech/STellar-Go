#!/bin/bash
# Comprehensive file transfer test using actual stellar executable in Docker
# Tests various file sizes and verifies content integrity

set -e

echo "=== Stellar File Transfer Docker Test ==="
echo "Testing file upload/download with various sizes and content integrity verification"
echo ""

# Wait for services to be ready
echo "Step 1: Waiting for services to be ready..."
sleep 30

# Function to calculate SHA256 checksum
calculate_checksum() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | cut -d' ' -f1
    else
        echo "ERROR: No checksum tool available"
        exit 1
    fi
}

# Function to generate test file with deterministic content
generate_test_file() {
    local size="$1"
    local output="$2"
    
    # Use Python to generate deterministic data
    python3 -c "
import sys
size = int(sys.argv[1])
output = sys.argv[2]
with open(output, 'wb') as f:
    for i in range(size):
        f.write(bytes([i % 256]))
" "$size" "$output"
}

# Function to test file upload and download
test_file_transfer() {
    local size="$1"
    local test_name="$2"
    
    echo ""
    echo "--- Testing $test_name ($size bytes) ---"
    
    # Generate source file - use shared volume path
    # Files need to be accessible from client-node container
    local src_file="/shared/testfile_${size}.dat"
    local remote_path="test/file_${size}.dat"
    local download_file="/shared/downloaded_${size}.dat"
    
    echo "  Generating test file..."
    generate_test_file "$size" "$src_file"
    local src_checksum=$(calculate_checksum "$src_file")
    echo "  Source file checksum: $src_checksum"
    
    # Get server node info
    echo "  Getting server node info..."
    SERVER_NODE=$(curl -s http://server-node:1524/node)
    SERVER_NODE_ID=$(echo "$SERVER_NODE" | grep -o '"NodeID":"[^"]*"' | cut -d'"' -f4)
    echo "  Server Node ID: $SERVER_NODE_ID"
    
    # Wait for device discovery
    echo "  Waiting for device discovery..."
    sleep 15
    
    # Get devices from client
    echo "  Getting devices..."
    DEVICES=$(curl -s http://client-node:1524/devices)
    echo "  Devices response: $DEVICES"
    
    # Extract device ID (should be server node)
    DEVICE_ID=$(echo "$DEVICES" | grep -o '"[^"]*":{' | head -1 | tr -d '":{')
    if [ -z "$DEVICE_ID" ]; then
        echo "  WARNING: Could not extract device ID, using server node ID"
        DEVICE_ID="$SERVER_NODE_ID"
    fi
    echo "  Using Device ID: $DEVICE_ID"
    
    # Upload file - localPath must be accessible from client-node
    # Since we're using a shared volume, the path should work
    echo "  Uploading file..."
    UPLOAD_RESPONSE=$(curl -s -X POST "http://client-node:1524/devices/${DEVICE_ID}/files/upload" \
        -F "localPath=/shared/testfile_${size}.dat" \
        -F "remotePath=${remote_path}")
    echo "  Upload response: $UPLOAD_RESPONSE"
    
    # Check upload success
    if echo "$UPLOAD_RESPONSE" | grep -q '"success":true'; then
        echo "  ✓ Upload successful"
    else
        echo "  ✗ Upload failed: $UPLOAD_RESPONSE"
        return 1
    fi
    
    # Wait a bit for file to be written
    echo "  Waiting for file to be written..."
    sleep 5
    
    # Download file - destPath must be accessible from client-node
    echo "  Downloading file..."
    DOWNLOAD_RESPONSE=$(curl -s -X GET "http://client-node:1524/devices/${DEVICE_ID}/files/download?remotePath=${remote_path}&destPath=/shared/downloaded_${size}.dat")
    echo "  Download response: $DOWNLOAD_RESPONSE"
    
    # Check download success
    if echo "$DOWNLOAD_RESPONSE" | grep -q '"success":true'; then
        echo "  ✓ Download successful"
    else
        echo "  ✗ Download failed: $DOWNLOAD_RESPONSE"
        return 1
    fi
    
    # Wait for download to complete
    sleep 3
    
    # Verify file exists
    if [ ! -f "$download_file" ]; then
        echo "  ✗ Downloaded file not found at $download_file"
        return 1
    fi
    
    # Verify file size
    local src_size=$(stat -f%z "$src_file" 2>/dev/null || stat -c%s "$src_file" 2>/dev/null || echo "0")
    local dst_size=$(stat -f%z "$download_file" 2>/dev/null || stat -c%s "$download_file" 2>/dev/null || echo "0")
    
    echo "  Source size: $src_size bytes"
    echo "  Downloaded size: $dst_size bytes"
    
    if [ "$src_size" != "$dst_size" ]; then
        echo "  ✗ Size mismatch! Expected $src_size, got $dst_size"
        return 1
    fi
    echo "  ✓ Size matches"
    
    # Verify checksum
    local dst_checksum=$(calculate_checksum "$download_file")
    echo "  Downloaded file checksum: $dst_checksum"
    
    if [ "$src_checksum" != "$dst_checksum" ]; then
        echo "  ✗ Checksum mismatch!"
        echo "    Expected: $src_checksum"
        echo "    Got:      $dst_checksum"
        
        # Show first 100 bytes of both files for debugging
        echo "  First 100 bytes of source:"
        head -c 100 "$src_file" | xxd
        echo "  First 100 bytes of downloaded:"
        head -c 100 "$download_file" | xxd
        
        return 1
    fi
    echo "  ✓ Checksum matches"
    
    # Cleanup
    rm -f "$src_file" "$download_file"
    
    echo "  ✓ Test passed for $test_name"
    return 0
}

# Test sizes - especially around the problematic threshold
TEST_SIZES=(
    "100:Small file"
    "1024:1KB"
    "4096:4KB"
    "32768:32KB"
    "65536:64KB"
    "131072:128KB"
    "256000:250KB"
    "261460:Just below threshold"
    "261461:Just below threshold"
    "261462:Exact threshold"
    "261463:Just above threshold"
    "261464:Just above threshold"
    "261465:Just above threshold"
    "262144:256KB"
    "524288:512KB"
    "1048576:1MB"
    "2097152:2MB"
)

# Track results
PASSED=0
FAILED=0
FAILED_TESTS=()

echo ""
echo "Step 2: Running file transfer tests..."
echo ""

for test_case in "${TEST_SIZES[@]}"; do
    size=$(echo "$test_case" | cut -d':' -f1)
    name=$(echo "$test_case" | cut -d':' -f2)
    
    if test_file_transfer "$size" "$name"; then
        PASSED=$((PASSED + 1))
    else
        FAILED=$((FAILED + 1))
        FAILED_TESTS+=("$name ($size bytes)")
    fi
done

# Summary
echo ""
echo "=== Test Summary ==="
echo "Passed: $PASSED"
echo "Failed: $FAILED"

if [ $FAILED -gt 0 ]; then
    echo ""
    echo "Failed tests:"
    for test in "${FAILED_TESTS[@]}"; do
        echo "  - $test"
    done
    exit 1
else
    echo ""
    echo "✓ All tests passed!"
    exit 0
fi
