#!/bin/bash
# OpenFGA Traffic Generator
# This script sends random /check requests to OpenFGA to generate metrics data.

STORE_ID="01KJNT55E86WH1AM7BDPNXYN3X"
OPENFGA_URL="http://localhost:8080"

echo "Using Store ID: $STORE_ID"
echo "Target: $OPENFGA_URL"
echo "Press [CTRL+C] to stop..."

while true; do
  # Randomly check permissions
  USER_ID="user:$((RANDOM % 100))"
  DOC_ID="document:doc$((RANDOM % 50))"
  RELATION=$([ $((RANDOM % 2)) -eq 0 ] && echo "reader" || echo "writer")

  curl -s -X POST "$OPENFGA_URL/stores/$STORE_ID/check" \
    -H "Content-Type: application/json" \
    -d "{\"tuple_key\": {\"user\": \"$USER_ID\", \"relation\": \"$RELATION\", \"object\": \"$DOC_ID\"}}" > /dev/null

  # Short sleep to avoid overwhelming local cluster but fast enough to see spikes
  sleep 0.1
done
