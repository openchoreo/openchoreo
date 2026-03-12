#!/usr/bin/env bash
# List cached images across all registry caches.
#
# Usage:
#   ./list-cached.sh             # List all cached repo:tag pairs
#   ./list-cached.sh ghcr-cache  # List only ghcr cache

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/compose.yaml"
REPO_BASE="/var/lib/registry/docker/registry/v2/repositories"

SERVICES=("ghcr-cache" "dockerhub-cache" "quay-cache" "kgateway-cache")
UPSTREAMS=("ghcr.io" "docker.io" "quay.io" "cr.kgateway.dev")

# Filter to a specific service if provided
if [[ -n "$1" ]]; then
  SERVICES=("$1")
  case "$1" in
    ghcr-cache)      UPSTREAMS=("ghcr.io") ;;
    dockerhub-cache) UPSTREAMS=("docker.io") ;;
    quay-cache)      UPSTREAMS=("quay.io") ;;
    kgateway-cache)  UPSTREAMS=("cr.kgateway.dev") ;;
    *)
      echo "Unknown service: $1"
      echo "Available: ghcr-cache, dockerhub-cache, quay-cache, kgateway-cache"
      exit 1
      ;;
  esac
fi

for i in "${!SERVICES[@]}"; do
  service="${SERVICES[$i]}"
  upstream="${UPSTREAMS[$i]}"

  tags=$(docker compose -f "$COMPOSE_FILE" exec -T "$service" \
    find "$REPO_BASE" -name "link" -path "*/_manifests/tags/*/current/*" 2>/dev/null \
    | sed 's|.*/repositories/||;s|/_manifests/tags/|:|;s|/current/link||' \
    | sort) || true

  if [[ -n "$tags" ]]; then
    echo "=== ${upstream} ==="
    echo "$tags"
    echo ""
  fi
done
