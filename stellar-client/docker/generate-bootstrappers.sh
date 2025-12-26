#!/bin/bash
# Script to generate bootstrappers.txt with fixed bootstrap peer ID
# This is used by node containers to generate the bootstrappers.txt file

set -e

BOOTSTRAP_PEER_ID="12D3KooW9ttuSFa68YvBeat21vFDmCAwEZKLJ14h6SHFht2usRFj"
BOOTSTRAP_HOST="bootstrap"
BOOTSTRAP_PORT="43210"

# Format: /dns4/<service_name>/tcp/<port>/p2p/<peer_id>
# Docker DNS will resolve 'bootstrap' to internal IP
BOOTSTRAPPERS_DIR="/app/bootstrap-data"
BOOTSTRAPPERS_TXT="${BOOTSTRAPPERS_DIR}/bootstrappers.txt"

# Ensure directory exists
mkdir -p "${BOOTSTRAPPERS_DIR}"

echo "Generating bootstrappers.txt..."
echo "/dns4/${BOOTSTRAP_HOST}/tcp/${BOOTSTRAP_PORT}/p2p/${BOOTSTRAP_PEER_ID}" > "${BOOTSTRAPPERS_TXT}"

echo "Generated bootstrappers.txt:"
cat "${BOOTSTRAPPERS_TXT}"
