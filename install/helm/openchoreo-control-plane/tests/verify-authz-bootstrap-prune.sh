#!/usr/bin/env bash
# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

chart_dir="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
rendered="$(mktemp)"
trap 'rm -f "$rendered"' EXIT

helm template openchoreo-control-plane "$chart_dir" \
  --namespace openchoreo-control-plane \
  --set-string openchoreoApi.http.hostnames[0]=api.example.com \
  --set-string backstage.http.hostnames[0]=backstage.example.com \
  --set-string backstage.baseUrl=https://backstage.example.com \
  --set-string gateway.tls.hostname=gateway.example.com \
  --set-string security.oidc.issuer=https://issuer.example.com \
  --set-string backstage.secretName=backstage-secrets > "$rendered"

require_rendered() {
  local expected="$1"
  if ! grep -Fq -- "$expected" "$rendered"; then
    echo "missing rendered content: $expected" >&2
    exit 1
  fi
}

require_rendered "- --prune"
require_rendered "- --selector"
require_rendered "- \"openchoreo.dev/managed-by=openchoreo-authz-bootstrap\""
require_rendered "- --prune-allowlist"
require_rendered "- openchoreo.dev/v1alpha1/ClusterAuthzRole"
require_rendered "- openchoreo.dev/v1alpha1/ClusterAuthzRoleBinding"
require_rendered "- openchoreo.dev/v1alpha1/AuthzRole"
require_rendered "- openchoreo.dev/v1alpha1/AuthzRoleBinding"
require_rendered "openchoreo.dev/managed-by: \"openchoreo-authz-bootstrap\""
require_rendered "- list"
require_rendered "- delete"
