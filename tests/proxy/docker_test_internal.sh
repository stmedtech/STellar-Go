#!/bin/sh
# Internal test script for Docker Compose proxy test

set -e

echo '=== Stellar-Go Proxy Docker Compose Test ==='
echo 'Waiting for services to be ready...'
sleep 20

echo ''
echo 'Step 1: Get server peer info...'
SERVER_NODE=$(curl -s http://server-node:1524/node)
echo "Server node info: $SERVER_NODE"

# Extract NodeID
SERVER_NODE_ID=$(echo "$SERVER_NODE" | grep -o '"NodeID":"[^"]*"' | cut -d'"' -f4)
echo "Server node ID: $SERVER_NODE_ID"

# Extract first address from Addresses array (format: ["/ip4/172.x.x.x/tcp/xxxxx", ...])
# Addresses is a JSON array, so we need to extract the first element
SERVER_ADDR=$(echo "$SERVER_NODE" | grep -o '"/ip4/[^"]*"' | head -1 | tr -d '"')
if [ -z "$SERVER_ADDR" ]; then
  echo 'WARNING: Could not extract server address from node info'
  echo "Server node response: $SERVER_NODE"
  echo "Will try to use bootstrap discovery instead..."
  # If we can't get the address, we'll rely on DHT discovery
  SERVER_PEER_INFO=""
else
  echo "Server address: $SERVER_ADDR"
  # Construct peer_info in format: /ip4/address/tcp/port/p2p/nodeid
  SERVER_PEER_INFO="$SERVER_ADDR/p2p/$SERVER_NODE_ID"
  echo "Server peer info: $SERVER_PEER_INFO"
fi

echo ''
if [ -n "$SERVER_PEER_INFO" ]; then
  echo 'Step 2: Connect client to server via API...'
  CONNECT_RESPONSE=$(curl -s -X POST http://client-node:1524/connect \
    -H 'Content-Type: application/json' \
    -d "{\"peer_info\":\"$SERVER_PEER_INFO\"}")
  echo "Connect response: $CONNECT_RESPONSE"
  
  # Wait for connection to be established
  echo 'Waiting for connection to establish...'
  sleep 10
else
  echo 'Step 2: Waiting for DHT discovery (no explicit connection)...'
  echo 'Nodes should discover each other through bootstrap node'
  sleep 20
fi

echo ''
echo 'Step 3: Get devices...'
DEVICES=$(curl -s http://client-node:1524/devices)
echo "Devices: $DEVICES"

# Extract device ID from devices response
# Devices is a JSON object where keys are device IDs
DEVICE_ID=$(echo "$DEVICES" | grep -o '"[^"]*":{' | head -1 | tr -d '":{')
if [ -z "$DEVICE_ID" ]; then
  echo 'ERROR: Could not extract device ID from devices'
  echo "Devices response: $DEVICES"
  echo "Trying to use server node ID directly..."
  DEVICE_ID="$SERVER_NODE_ID"
fi
echo "Device ID: $DEVICE_ID"

echo ''
echo 'Step 4: Close any existing proxy on port 9000...'
curl -s -X DELETE http://client-node:1524/proxy/9000 > /dev/null
sleep 2

echo ''
echo 'Step 5: Create proxy (local_port=9000, remote=hello-world:8000)...'
PROXY_RESPONSE=$(curl -s -X POST http://client-node:1524/proxy \
  -H 'Content-Type: application/json' \
  -d "{\"device_id\":\"$DEVICE_ID\",\"local_port\":9000,\"remote_host\":\"hello-world\",\"remote_port\":8000}")
echo "Proxy creation response: $PROXY_RESPONSE"
sleep 10

echo ''
echo 'Step 6: List proxies...'
PROXIES=$(curl -s http://client-node:1524/proxy)
echo "Proxies: $PROXIES"

echo ''
echo 'Step 7: Test proxy connection (similar to stellar-proxy-core)...'
echo 'Testing: curl http://client-node:9000'
curl -v --max-time 15 http://client-node:9000

echo ''
echo '=== Test completed successfully! ==='

