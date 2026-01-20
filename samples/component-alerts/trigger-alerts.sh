#!/usr/bin/env bash

set -euo pipefail

FRONTEND_URL="http://frontend-development.openchoreoapis.localhost:19080"

echo "Starting loop to invoke frontend URL: ${FRONTEND_URL}"
echo "Press Ctrl+C to stop."

while true; do
  # Fire-and-forget request to generate traffic and trigger alerts
  curl -s -o /dev/null "${FRONTEND_URL}" || true
  sleep 1
done
