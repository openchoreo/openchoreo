#!/usr/bin/env bash
# Patch or revert ClusterWorkflowTemplates in the data plane cluster to route
# build-time image pulls through the local registry cache.
#
# Usage:
#   ./patch-build-templates.sh                          # patch, current context
#   ./patch-build-templates.sh --context k3d-dp         # patch, specific context
#   ./patch-build-templates.sh --revert                 # revert, current context
#   ./patch-build-templates.sh --revert --context k3d-dp
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

BUILD_TEMPLATES=(
  containerfile-build
  paketo-buildpacks-build
  ballerina-buildpack-build
  gcp-buildpacks-build
  generate-workload
)

VOLUME_ENTRY='{
  "name": "registries-conf",
  "configMap": {
    "name": "{{workflow.parameters.workflowrun-name}}-registries-conf",
    "optional": true
  }
}'

MOUNT_ENTRY='{
  "mountPath": "/etc/containers/registries.conf",
  "subPath": "registries.conf",
  "name": "registries-conf",
  "readOnly": true
}'

has_patch() {
  local name="$1"
  $KUBECTL get clusterworkflowtemplate "$name" -o yaml \
    | $YQ '.spec.templates[0].volumes[] | select(.name == "registries-conf") | .name' 2>/dev/null || true
}

apply_patch() {
  local name="$1"
  echo ">> Patching ClusterWorkflowTemplate/${name}"
  $KUBECTL get clusterworkflowtemplate "$name" -o yaml \
    | $YQ '
      del(.spec.templates[0].volumes[] | select(.name == "registries-conf")) |
      del(.spec.templates[0].container.volumeMounts[] | select(.name == "registries-conf")) |
      .spec.templates[0].volumes = (.spec.templates[0].volumes // []) + ['"${VOLUME_ENTRY}"'] |
      .spec.templates[0].container.volumeMounts = (.spec.templates[0].container.volumeMounts // []) + ['"${MOUNT_ENTRY}"']
    ' \
    | $KUBECTL apply -f -
}

revert_patch() {
  local name="$1"
  echo ">> Reverting ClusterWorkflowTemplate/${name}"
  $KUBECTL get clusterworkflowtemplate "$name" -o yaml \
    | $YQ '
      del(.spec.templates[0].volumes[] | select(.name == "registries-conf")) |
      del(.spec.templates[0].container.volumeMounts[] | select(.name == "registries-conf"))
    ' \
    | $YQ 'del(.spec.templates[0] | select(.volumes | length == 0) | .volumes)' \
    | $KUBECTL apply -f -
}

for tmpl in "${BUILD_TEMPLATES[@]}"; do
  if ! $KUBECTL get clusterworkflowtemplate "$tmpl" &>/dev/null; then
    echo ">> Skipping ClusterWorkflowTemplate/${tmpl} (not found)"
    continue
  fi

  patched=$(has_patch "$tmpl")

  if [[ "$REVERT" == true ]]; then
    if [[ "$patched" != "registries-conf" ]]; then
      echo ">> ClusterWorkflowTemplate/${tmpl} already clean, skipping"
    else
      revert_patch "$tmpl"
    fi
  else
    if [[ "$patched" == "registries-conf" ]]; then
      echo ">> ClusterWorkflowTemplate/${tmpl} already patched, skipping"
    else
      apply_patch "$tmpl"
    fi
  fi
done

echo ">> Done."
