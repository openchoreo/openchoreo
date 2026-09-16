#!/usr/bin/env bash
# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0
#
# Re-runs ThunderID's bootstrap setup Job against an already-installed cluster
# so the e2e overlay's bootstrap resources take effect — in particular
# 52-abac-user.yaml (test/e2e/k3d/values-thunder.yaml), which provisions the
# ABAC-restricted identity the abac-ui Playwright suite signs in as.
#
# Why a Job re-run instead of calling the admin API directly: users are
# seeded by the setup Job's in-process bootstrap, which writes to the SQLite
# store without an authenticated subject. Fresh installs (make e2e.setup) run
# the Job automatically as a helm pre-install hook; this script exists for
# clusters installed before the overlay changed.
#
# Usage:
#   test/ui/scripts/seed-idp-users.sh
#   E2E_KUBECONTEXT=k3d-openchoreo-e2e THUNDER_VERSION=1.0.1 test/ui/scripts/seed-idp-users.sh
#
# Requires: helm, kubectl, yq. ThunderID is briefly scaled to zero while the
# Job holds the RWO PVCs — sign-ins fail during that window.

set -euo pipefail

KCTX="${E2E_KUBECONTEXT:-k3d-openchoreo-e2e}"
THUNDER_NS="${THUNDER_NS:-thunder}"
# Keep in sync with THUNDER_VERSION in make/e2e.mk.
THUNDER_VERSION="${THUNDER_VERSION:-1.0.1}"
THUNDER_CHART="oci://ghcr.io/thunder-id/helm-charts/thunderid"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RERUN_JOB="thunder-setup-rerun"

log_info()  { printf '[seed-idp-users] %s\n' "$*"; }
log_error() { printf '[seed-idp-users] ERROR: %s\n' "$*" >&2; }

KUBECTL=(kubectl --context "${KCTX}" -n "${THUNDER_NS}")
helm_template() {
  helm --kube-context "${KCTX}" template thunder "${THUNDER_CHART}" \
    --namespace "${THUNDER_NS}" --version "${THUNDER_VERSION}" \
    --values "${REPO_ROOT}/install/k3d/common/values-thunder.yaml" \
    --values "${REPO_ROOT}/test/e2e/k3d/values-thunder.yaml" \
    "$@"
}

restore_thunder() {
  log_info "Scaling thunder-deployment back to 1"
  "${KUBECTL[@]}" scale deploy thunder-deployment --replicas=1 || true
}

log_info "Patching the bootstrap ConfigMap with the e2e overlay scripts"
helm_template --show-only templates/bootstrap-configmap.yaml \
  | "${KUBECTL[@]}" apply -f -

log_info "Scaling thunder-deployment to 0 to free the RWO PVCs"
# Install the restore trap before touching the replica count so any early
# exit (including a failed wait below) scales ThunderID back up.
trap restore_thunder EXIT
"${KUBECTL[@]}" scale deploy thunder-deployment --replicas=0
"${KUBECTL[@]}" wait --for=delete pod \
  -l app.kubernetes.io/name=thunderid --timeout=2m

log_info "Re-running the setup Job (renamed, helm hooks stripped)"
"${KUBECTL[@]}" delete job "${RERUN_JOB}" --ignore-not-found
helm_template --show-only templates/setup-job.yaml \
  | yq ".metadata.name = \"${RERUN_JOB}\" | del(.metadata.annotations)" \
  | "${KUBECTL[@]}" apply -f -
if ! "${KUBECTL[@]}" wait --for=condition=complete "job/${RERUN_JOB}" --timeout=5m; then
  log_error "setup Job did not complete; logs follow"
  "${KUBECTL[@]}" logs "job/${RERUN_JOB}" --tail=100 || true
  exit 1
fi
"${KUBECTL[@]}" logs "job/${RERUN_JOB}" --tail=20 || true
"${KUBECTL[@]}" delete job "${RERUN_JOB}"

restore_thunder
trap - EXIT
"${KUBECTL[@]}" wait --for=condition=available deploy/thunder-deployment --timeout=2m

log_info "done — abac-dev@openchoreo.dev / abac-developers seeded"
