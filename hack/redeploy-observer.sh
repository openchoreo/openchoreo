#!/usr/bin/env bash
# redeploy-observer.sh
#
# Cross-compiles the observer binary, builds its Docker image, imports it into
# the k3d cluster, and rolls out the observer deployment. Use this for the
# fast inner-loop after editing observer code.
#
# Prerequisites:
#   - k3d cluster "openchoreo-quick-start" running
#   - Docker available locally
#   - Go toolchain installed
#
# Flags:
#   --no-wait     Skip the rollout-status wait at the end.
#   --skip-build  Reuse the previously built binary + image (just re-import + rollout).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CLUSTER_NAME="${K3D_CLUSTER_NAME:-openchoreo-quick-start}"
IMAGE_TAG="${IMAGE_TAG:-latest-dev}"
IMAGE_REPO="${IMAGE_REPO:-ghcr.io/openchoreo}"
IMAGE="${IMAGE_REPO}/observer:${IMAGE_TAG}"
NAMESPACE="${OBSERVER_NAMESPACE:-openchoreo-observability-plane}"
DEPLOYMENT="${OBSERVER_DEPLOYMENT:-observer}"

WAIT_FOR_ROLLOUT=true
SKIP_BUILD=false

for arg in "$@"; do
  case "$arg" in
    --no-wait)    WAIT_FOR_ROLLOUT=false ;;
    --skip-build) SKIP_BUILD=true ;;
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

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*" >&2; }

cd "$PROJECT_DIR"

ARCH="$(go env GOARCH)"
BIN_OUT="bin/dist/linux/${ARCH}/observer"

if ! docker info >/dev/null 2>&1; then
  err "Docker daemon is not reachable."
  exit 1
fi

if ! k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
  err "k3d cluster '${CLUSTER_NAME}' not found."
  exit 1
fi

if [ "$SKIP_BUILD" = false ]; then
  log "Cross-compiling observer (linux/${ARCH})..."
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    go build -o "$BIN_OUT" -ldflags "-s -w" ./cmd/observer/

  log "Building Docker image ${IMAGE}..."
  docker build \
    --build-arg TARGETOS=linux \
    --build-arg TARGETARCH="$ARCH" \
    -t "$IMAGE" \
    -f cmd/observer/Dockerfile \
    .
else
  log "Skipping build (--skip-build); reusing existing image ${IMAGE}."
  if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    err "Image ${IMAGE} not found locally — drop --skip-build."
    exit 1
  fi
fi

log "Importing image into k3d cluster '${CLUSTER_NAME}'..."
k3d image import "$IMAGE" --cluster "$CLUSTER_NAME"

log "Restarting deployment/${DEPLOYMENT} in namespace ${NAMESPACE}..."
docker exec "k3d-${CLUSTER_NAME}-server-0" \
  kubectl rollout restart "deployment/${DEPLOYMENT}" -n "$NAMESPACE"

if [ "$WAIT_FOR_ROLLOUT" = true ]; then
  log "Waiting for rollout to finish..."
  docker exec "k3d-${CLUSTER_NAME}-server-0" \
    kubectl rollout status "deployment/${DEPLOYMENT}" -n "$NAMESPACE" --timeout=120s
  log "Done. Observer is running the new image."
else
  log "Rollout triggered; not waiting (--no-wait)."
fi
