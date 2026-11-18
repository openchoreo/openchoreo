#!/usr/bin/env bash
# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0
#
# Counts which backend answers an Endpoint, by the X-Served-By header the
# sample app sets: once through the gateway (the binding's external URL), and
# once from a pod in the environment calling the stable backend's own address,
# which the mesh splits.
#
#   ./tally.sh trips                         # 100 requests each way
#   N=200 ./tally.sh trips -H 'x-canary: true'
#   ./tally.sh trips --copies trips-go       # requests trips-go received, for a mirror
#
# NS and ENV_NAME pick the namespace (default) and environment (development).

set -euo pipefail

EP="${1:?usage: tally.sh ENDPOINT [--copies BACKEND] [CURL_ARGS...]}"
shift
NS="${NS:-default}"
ENV_NAME="${ENV_NAME:-development}"
N="${N:-100}"
COPIES=""
if [[ "${1:-}" == "--copies" ]]; then
  COPIES="${2:?--copies needs a backend name}"
  shift 2
fi

binding() { kubectl get endpointreleasebinding "$EP-$ENV_NAME" -n "$NS" -o jsonpath="$1"; }
count() { tr -d '\r' | awk -F': ' 'tolower($1) == "x-served-by" {print $2}' | sort | uniq -c; }
service_host() { binding "{.status.backends[?(@.name==\"$1\")].serviceURL.host}"; }
service_port() { binding "{.status.backends[?(@.name==\"$1\")].serviceURL.port}"; }

stable="$(kubectl get endpoints.openchoreo.dev "$EP" -n "$NS" -o jsonpath='{.spec.rollout.stable}')"
[[ -n "$stable" ]] || stable="$(kubectl get endpoints.openchoreo.dev "$EP" -n "$NS" -o jsonpath='{.spec.backendRefs[0].name}')"
host="$(service_host "$stable")"
dpns="$(cut -d. -f2 <<<"$host")"

# A client pod with a sidecar, so its calls go through the mesh.
# A pod whose sleep has ended doesn't restart, so start a new one.
client() {
  phase="$(kubectl get pod pd-client -n "$dpns" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [[ -n "$phase" && "$phase" != Running && "$phase" != Pending ]]; then
    kubectl delete pod pd-client -n "$dpns" --wait >/dev/null
    phase=""
  fi
  if [[ -z "$phase" ]]; then
    kubectl run pd-client -n "$dpns" --image=curlimages/curl:8.11.1 --restart=Never --command -- sleep 86400 >/dev/null
  fi
  kubectl wait pod pd-client -n "$dpns" --for=condition=Ready --timeout=120s >/dev/null
}

if [[ -n "$COPIES" ]]; then
  # A port-forward reaches the backend's pod directly, past the mesh.
  kubectl port-forward -n "$dpns" "svc/$(cut -d. -f1 <<<"$(service_host "$COPIES")")" 18099:"$(service_port "$COPIES")" >/dev/null 2>&1 &
  pf=$!
  trap 'kill $pf' EXIT
  sleep 2
  before="$(curl -s localhost:18099/stats)"
fi

# Requests sent, for the share of them a mirror received as copies.
sent=0
gw_host="$(binding '{.status.urls.external.host}')"
if [[ -n "$gw_host" ]]; then
  sent=$((sent + N))
  url="http://$gw_host:$(binding '{.status.urls.external.port}')$(binding '{.status.urls.external.path}')"
  echo "through the gateway, $url:"
  for _ in $(seq "$N"); do curl -s -o /dev/null -D - "$@" "$url"; done | count
fi

args=""
[[ $# -gt 0 ]] && args="$(printf '%q ' "$@")"
client
echo "in-cluster, http://$host:$(service_port "$stable")/:"
sent=$((sent + N))
kubectl exec -n "$dpns" pd-client -c pd-client -- sh -c \
  "for i in \$(seq $N); do curl -s -o /dev/null -D - $args http://$host:$(service_port "$stable")/; done" | count

if [[ -n "$COPIES" ]]; then
  after="$(curl -s localhost:18099/stats)"
  num() { tr -dc '0-9' <<<"$1"; }
  echo "$COPIES received $(( $(num "$after") - $(num "$before") )) of the $sent requests as copies"
fi
