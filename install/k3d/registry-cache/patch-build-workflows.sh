#!/usr/bin/env bash
# Patch or revert ClusterWorkflows in the control plane cluster to create a
# registries-conf ConfigMap per WorkflowRun for the registry cache.
#
# Usage:
#   ./patch-build-workflows.sh                          # patch, current context
#   ./patch-build-workflows.sh --context k3d-cp         # patch, specific context
#   ./patch-build-workflows.sh --revert                 # revert, current context
#   ./patch-build-workflows.sh --revert --context k3d-cp
#
# Idempotent — safe to run multiple times.
# Prerequisites: kubectl, docker (for mikefarah/yq)

set -euo pipefail

REVERT=false
KUBECTL="kubectl"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --context)  KUBECTL="kubectl --context $2"; shift 2 ;;
    --revert)   REVERT=true; shift ;;
    *)          echo "Unknown flag: $1"; exit 1 ;;
  esac
done

YQ="docker run --rm -i mikefarah/yq"

REGISTRIES_CONF_FILE=$(mktemp)
trap 'rm -f "${REGISTRIES_CONF_FILE}"' EXIT
cat > "${REGISTRIES_CONF_FILE}" <<'REGEOF'
unqualified-search-registries = ["docker.io"]

[[registry]]
location = "host.k3d.internal:10082"
insecure = true

[[registry]]
location = "docker.io"
[[registry.mirror]]
location = "host.k3d.internal:5602"
insecure = true

[[registry]]
location = "ghcr.io"
[[registry.mirror]]
location = "host.k3d.internal:5601"
insecure = true

[[registry]]
location = "quay.io"
[[registry.mirror]]
location = "host.k3d.internal:5603"
insecure = true

[[registry]]
location = "cr.kgateway.dev"
[[registry.mirror]]
location = "host.k3d.internal:5604"
insecure = true

[[registry]]
location = "gcr.io"
[[registry.mirror]]
location = "host.k3d.internal:5605"
insecure = true
REGEOF

CI_WORKFLOWS=(
  dockerfile-builder
  paketo-buildpacks-builder
  ballerina-buildpack-builder
  gcp-buildpacks-builder
)

CONFIGMAP_RESOURCE='{
  "id": "build-registries-conf",
  "template": {
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {
      "name": "${metadata.workflowRunName}-registries-conf",
      "namespace": "${metadata.namespace}"
    },
    "data": {
      "registries.conf": ""
    }
  }
}'

has_patch() {
  local name="$1"
  $KUBECTL get clusterworkflow "$name" -o yaml \
    | $YQ '.spec.resources[] | select(.id == "build-registries-conf") | .id' 2>/dev/null || true
}

apply_patch() {
  local name="$1"
  echo ">> Patching ClusterWorkflow/${name}"
  $KUBECTL get clusterworkflow "$name" -o yaml \
    | $YQ 'del(.spec.resources[] | select(.id == "build-registries-conf"))' \
    | $YQ '.spec.resources = (.spec.resources // []) + ['"${CONFIGMAP_RESOURCE}"']' \
    | REGISTRIES_CONF_CONTENT="$(cat "${REGISTRIES_CONF_FILE}")" \
      docker run --rm -i -e REGISTRIES_CONF_CONTENT mikefarah/yq '
        (.spec.resources[] | select(.id == "build-registries-conf")).template.data."registries.conf" = strenv(REGISTRIES_CONF_CONTENT) |
        (.spec.resources[] | select(.id == "build-registries-conf")).template.data."registries.conf" style="literal"
      ' \
    | $KUBECTL apply -f -
}

revert_patch() {
  local name="$1"
  echo ">> Reverting ClusterWorkflow/${name}"
  $KUBECTL get clusterworkflow "$name" -o yaml \
    | $YQ 'del(.spec.resources[] | select(.id == "build-registries-conf"))' \
    | $KUBECTL apply -f -
}

for wf in "${CI_WORKFLOWS[@]}"; do
  if ! $KUBECTL get clusterworkflow "$wf" &>/dev/null; then
    echo ">> Skipping ClusterWorkflow/${wf} (not found)"
    continue
  fi

  patched=$(has_patch "$wf")

  if [[ "$REVERT" == true ]]; then
    if [[ "$patched" != "build-registries-conf" ]]; then
      echo ">> ClusterWorkflow/${wf} already clean, skipping"
    else
      revert_patch "$wf"
    fi
  else
    if [[ "$patched" == "build-registries-conf" ]]; then
      echo ">> ClusterWorkflow/${wf} already patched, skipping"
    else
      apply_patch "$wf"
    fi
  fi
done

echo ">> Done."
