#!/bin/bash
# OpenFGA Error Generator
# This script sends invalid requests to OpenFGA to generate error metrics.

STORE_ID="01KJNT55E86WH1AM7BDPNXYN3X"
INVALID_STORE="invalid-store-id-12345"
OPENFGA_URL="http://localhost:8080"

echo "Targeting errors on: $OPENFGA_URL"
echo "Press [CTRL+C] to stop..."

while true; do
  TYPE=$((RANDOM % 3))

  case $TYPE in
    0)
      # 404 Not Found: Invalid Store ID
      echo -n "."
      curl -s -o /dev/null -w "%{http_code}" -X POST "$OPENFGA_URL/stores/$INVALID_STORE/check" \
        -H "Content-Type: application/json" \
        -d '{"tuple_key": {"user": "user:err", "relation": "reader", "object": "doc:err"}}'
      ;;
    1)
      # 400 Bad Request: Malformed Tuple (missing user)
      echo -n "x"
      curl -s -o /dev/null -w "%{http_code}" -X POST "$OPENFGA_URL/stores/$STORE_ID/check" \
        -H "Content-Type: application/json" \
        -d '{"tuple_key": {"relation": "reader", "object": "document:doc1"}}'
      ;;
    2)
      # 400 Bad Request: Undefined Relation
      echo -n "?"
      curl -s -o /dev/null -w "%{http_code}" -X POST "$OPENFGA_URL/stores/$STORE_ID/check" \
        -H "Content-Type: application/json" \
        -d '{"tuple_key": {"user": "user:1", "relation": "undefined_rel", "object": "document:doc1"}}'
      ;;
  esac

  sleep 0.2
done
