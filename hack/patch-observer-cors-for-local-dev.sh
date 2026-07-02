#!/usr/bin/env bash
# patch-observer-cors-for-local-dev.sh
#
# Adds localhost:3000 and localhost:7007 to the observer's CORS allowed origins
# so a Backstage instance running on the host (yarn start) can call observer
# APIs that live inside the k3d cluster.
#
# Idempotent: appends only if the origin is missing.
#
# Prerequisites:
#   - k3d cluster "openchoreo-quick-start" running with the observability plane installed
#
# Flags:
#   --no-wait   Skip the rollout-status wait at the end.

set -euo pipefail

CLUSTER_NAME="${K3D_CLUSTER_NAME:-openchoreo-quick-start}"
NAMESPACE="${OBSERVER_NAMESPACE:-openchoreo-observability-plane}"
CONFIGMAP="${OBSERVER_CONFIGMAP:-observer-config}"
DEPLOYMENT="${OBSERVER_DEPLOYMENT:-observer}"
EXTRA_ORIGINS="${EXTRA_CORS_ORIGINS:-http://localhost:3000,http://localhost:7007}"

WAIT_FOR_ROLLOUT=true
for arg in "$@"; do
  case "$arg" in
    --no-wait) WAIT_FOR_ROLLOUT=false ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "Unknown flag: $arg" >&2
      exit 1
      ;;
  esac
done

GREEN='\033[0;32m'; NC='\033[0m'
log() { echo -e "${GREEN}[INFO]${NC} $*"; }

kctl() { docker exec "k3d-${CLUSTER_NAME}-server-0" kubectl "$@"; }

CURRENT="$(kctl get configmap "$CONFIGMAP" -n "$NAMESPACE" -o jsonpath='{.data.CORS_ALLOWED_ORIGINS}' 2>/dev/null || true)"
MERGED="$CURRENT"
IFS=',' read -ra WANTED <<< "$EXTRA_ORIGINS"
for origin in "${WANTED[@]}"; do
  if [[ ",${MERGED}," != *",${origin},"* ]]; then
    if [ -z "$MERGED" ]; then MERGED="$origin"; else MERGED="${MERGED},${origin}"; fi
  fi
done

if [ "$MERGED" = "$CURRENT" ]; then
  log "CORS already includes $EXTRA_ORIGINS — nothing to do."
  exit 0
fi

log "Patching $CONFIGMAP CORS_ALLOWED_ORIGINS to: $MERGED"
kctl patch configmap "$CONFIGMAP" -n "$NAMESPACE" --type=merge \
  -p "{\"data\":{\"CORS_ALLOWED_ORIGINS\":\"$MERGED\"}}"

log "Restarting deployment/$DEPLOYMENT to pick up the new CORS config..."
kctl rollout restart "deployment/$DEPLOYMENT" -n "$NAMESPACE"

if [ "$WAIT_FOR_ROLLOUT" = true ]; then
  log "Waiting for rollout to finish..."
  kctl rollout status "deployment/$DEPLOYMENT" -n "$NAMESPACE" --timeout=120s
  log "Done."
else
  log "Rollout triggered; not waiting (--no-wait)."
fi
