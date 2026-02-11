#!/usr/bin/env bash

set -euo pipefail

FRONTEND_URL="http://frontend-development-default.openchoreoapis.localhost:19080"

echo "Starting loop to invoke frontend URL: ${FRONTEND_URL}"
echo "Press Ctrl+C to stop."

while true; do
  # Fire-and-forget request to homepage to generate traffic and trigger error logs alert and high cpu usage alert
  curl -s -o /dev/null "${FRONTEND_URL}" || true
  # Fire-and-forget request to load cart page to trigger high memory usage alert
  curl -s -o /dev/null "${FRONTEND_URL}/cart" || true

  sleep 0.5
done
