#!/usr/bin/env bash
# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
#
# Verifies the cluster-agent Helm charts keep the two invariants that matter:
#
#   1. No cluster-agent Role or ClusterRole gets WRITE access to apiGroup
#      rbac.authorization.k8s.io. The wildcard over clusterroles /
#      clusterrolebindings is the privilege-escalation vector T12 closed, and it
#      is the one thing that must never come back. Read verbs are required and
#      allowed: without get/list/watch the RenderedRelease status read-back
#      aborts and every component reports NotReady forever (T73).
#
#   2. The data-plane agent CAN write the objects a Cell is made of, cluster-wide.
#      Cells (dp-<ns>-<project>-<environment>-<hash>) are created at runtime, so a
#      namespaced Role in the release namespace can never cover them. T12 moved
#      those grants into such a Role and silently bricked every deployment; T73
#      found it with the first real workload. resourcequotas/limitranges are
#      asserted explicitly because ClusterProjectType/idp-default emits them and
#      upstream never granted them at all.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

for bin in helm yq; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "missing dependency: $bin" >&2
    exit 1
  fi
done

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

fail=0

check_empty() {
  title=$1
  output=$2
  if [ -n "$output" ]; then
    printf 'FAIL %s\n%s\n' "$title" "$output"
    fail=1
  else
    printf 'OK   %s\n' "$title"
  fi
}

check_present() {
  title=$1
  output=$2
  if [ -z "$output" ]; then
    printf 'FAIL %s\n' "$title"
    fail=1
  else
    printf 'OK   %s\n' "$title"
  fi
}

render_and_check() {
  chart=$1
  expect_serviceaccounts=$2
  namespace="verify-${chart#openchoreo-}"
  rendered="$tmpdir/$chart.yaml"
  values="$tmpdir/$chart-values.yaml"

  case "$chart" in
    openchoreo-data-plane)
      cat > "$values" <<'EOF'
gateway:
  tls:
    hostname: "*.example.com"
EOF
      ;;
    openchoreo-observability-plane)
      cat > "$values" <<'EOF'
observer:
  secretName: verify-observer-secret
  controlPlaneApiUrl: http://api.example.com:8080
  extraEnvs:
    - name: OBSERVER_BASE_URL
      value: http://observer.example.com:11080
    - name: AUTHZ_TIMEOUT
      value: 30s
gateway:
  tls:
    hostname: "*.example.com"
EOF
      ;;
    *)
      : > "$values"
      ;;
  esac

  if [ "$chart" = "openchoreo-workflow-plane" ]; then
    helm dependency build "install/helm/$chart" >/dev/null
  fi

  helm template "verify-$chart" "install/helm/$chart" --namespace "$namespace" -f "$values" > "$rendered"

  # Any verb outside get/list/watch on the RBAC apiGroup is a finding.
  bad_cluster_rbac=$(
    yq -r '
      select(.kind == "ClusterRole" or .kind == "Role") |
      select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
      .kind as $kind |
      .metadata.name as $name |
      .rules[]? |
      select(.apiGroups[]? == "rbac.authorization.k8s.io") |
      select([.verbs[]? | select(. != "get" and . != "list" and . != "watch")] | length > 0) |
      $kind + "/" + $name + " grants " + (.verbs // [] | join(",")) + " on " + (.resources // [] | join(","))
    ' "$rendered"
  )
  check_empty "$chart: cluster-agent RBAC grants are read-only (no write verbs)" "$bad_cluster_rbac"

  if [ "$chart" = "openchoreo-data-plane" ]; then
    for cell_resource in resourcequotas limitranges deployments services configmaps secrets networkpolicies; do
      granted=$(
        CELL_RESOURCE="$cell_resource" yq -r '
          select(.kind == "ClusterRole") |
          select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
          .rules[]? |
          select(.resources[]? == strenv(CELL_RESOURCE)) |
          strenv(CELL_RESOURCE)
        ' "$rendered"
      )
      check_present "$chart: ClusterRole grants $cell_resource (Cell namespaces are runtime-created)" "$granted"
    done
  fi

  role=$(
    yq -r '
      select(.kind == "Role") |
      select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
      .metadata.name
    ' "$rendered"
  )
  check_present "$chart: renders a namespaced cluster-agent Role" "$role"

  rolebinding=$(
    yq -r '
      select(.kind == "RoleBinding") |
      select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
      select(.roleRef.kind == "Role") |
      .metadata.name
    ' "$rendered"
  )
  check_present "$chart: renders a namespaced cluster-agent RoleBinding" "$rolebinding"

  role_secrets=$(
    yq -r '
      select(.kind == "Role") |
      select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
      .rules[]? |
      select((.apiGroups[]? == "") and (.resources[]? == "secrets")) |
      .resources[]?
    ' "$rendered"
  )
  check_present "$chart: secrets are granted by Role" "$role_secrets"

  if [ "$expect_serviceaccounts" = "yes" ]; then
    role_serviceaccounts=$(
      yq -r '
        select(.kind == "Role") |
        select(.metadata.labels."app.kubernetes.io/component" == "cluster-agent") |
        .rules[]? |
        select((.apiGroups[]? == "") and (.resources[]? == "serviceaccounts")) |
        .resources[]?
      ' "$rendered"
    )
    check_present "$chart: serviceaccounts are granted by Role" "$role_serviceaccounts"
  fi
}

render_and_check openchoreo-data-plane yes
render_and_check openchoreo-observability-plane no
render_and_check openchoreo-workflow-plane yes

if [ "$fail" = 0 ]; then
  echo "verify-cluster-agent-rbac: VERDE"
else
  echo "verify-cluster-agent-rbac: ROJO"
fi

exit "$fail"
