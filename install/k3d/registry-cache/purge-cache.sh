#!/usr/bin/env bash
# Purge cached images from the registry caches.
#
# Usage:
#   ./purge-cache.sh external-secrets/external-secrets:v1.3.2   # Purge a specific image:tag
#   ./purge-cache.sh openchoreo/controller                      # Purge all tags of a repo
#   ./purge-cache.sh openchoreo/*                               # Purge all openchoreo repos
#   ./purge-cache.sh --all                                      # Purge everything from all caches

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/compose.yaml"
REPO_BASE="/var/lib/registry/docker/registry/v2/repositories"

ALL_SERVICES="ghcr-cache dockerhub-cache quay-cache kgateway-cache"

compose_exec() {
  local service="$1"
  shift
  docker compose -f "$COMPOSE_FILE" exec -T "$service" "$@"
}

gc() {
  local service="$1"
  compose_exec "$service" registry garbage-collect /etc/docker/registry/config.yml --delete-untagged 2>/dev/null || true
}

usage() {
  cat <<EOF
Usage: $0 <image-ref> [<image-ref>...]

Purge cached images so the next pull fetches fresh from upstream.

Arguments:
  <repo>:<tag>      Purge a specific tag (e.g., openchoreo/controller:latest-dev)
  <repo>            Purge all tags of a repo (e.g., openchoreo/controller)
  <org>/*           Purge all repos under an org (e.g., openchoreo/*)
  --all             Purge everything from all caches

The image is automatically found and purged from whichever cache contains it.

Examples:
  $0 openchoreo/controller:latest-dev
  $0 openchoreo/*
  $0 external-secrets/external-secrets:v1.3.2
  $0 jetstack/cert-manager-controller:v1.19.2
  $0 --all
EOF
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

# Handle --all
if [[ "$1" == "--all" ]]; then
  for svc in $ALL_SERVICES; do
    echo "Purging all cached images from ${svc}..."
    compose_exec "$svc" sh -c "rm -rf ${REPO_BASE}/*" 2>/dev/null || true
    gc "$svc"
  done
  echo "Done."
  exit 0
fi

# Track which services need garbage collection
gc_services=""

for ref in "$@"; do
  # Split into repo and tag
  if [[ "$ref" == *:* ]]; then
    repo="${ref%%:*}"
    tag="${ref#*:}"
  else
    repo="$ref"
    tag=""
  fi

  # Handle wildcard (e.g., openchoreo/*)
  if [[ "$repo" == *'/*' ]]; then
    org="${repo%/*}"
    found=false
    for svc in $ALL_SERVICES; do
      if compose_exec "$svc" test -d "${REPO_BASE}/${org}" 2>/dev/null; then
        compose_exec "$svc" rm -rf "${REPO_BASE}/${org}"
        echo "Purged ${org}/* from ${svc}"
        gc_services="${gc_services} ${svc}"
        found=true
      fi
    done
    if [[ "$found" == "false" ]]; then
      echo "Not cached: ${org}/*"
    fi
    continue
  fi

  found=false
  for svc in $ALL_SERVICES; do
    if [[ -z "$tag" ]]; then
      # Purge entire repo
      if compose_exec "$svc" test -d "${REPO_BASE}/${repo}" 2>/dev/null; then
        compose_exec "$svc" rm -rf "${REPO_BASE}/${repo}"
        echo "Purged ${repo} from ${svc}"
        gc_services="${gc_services} ${svc}"
        found=true
        break
      fi
    else
      # Purge specific tag
      tag_path="${REPO_BASE}/${repo}/_manifests/tags/${tag}"
      if compose_exec "$svc" test -d "$tag_path" 2>/dev/null; then
        compose_exec "$svc" rm -rf "$tag_path"
        echo "Purged ${repo}:${tag} from ${svc}"
        gc_services="${gc_services} ${svc}"
        found=true
        break
      fi
    fi
  done

  if [[ "$found" == "false" ]]; then
    echo "Not cached: ${ref}"
  fi
done

# Run garbage collection on affected caches (deduplicated)
for svc in $(echo "$gc_services" | tr ' ' '\n' | sort -u); do
  [[ -z "$svc" ]] && continue
  echo "Running garbage collection on ${svc}..."
  gc "$svc"
done

echo "Done."
