#!/usr/bin/env bash
# =============================================================================
# kiac-install.sh — install OpenChoreo on a kiac cluster
#   kiac = Kubernetes in Apple Containers (https://github.com/saiyam1814/kiac)
#
# One-liner:
#   curl -fsSL <raw>/kiac-install.sh | bash -s -- --version 1.1.2
#
# Mirrors OpenChoreo's install/k3d/k3d-install.sh, adapted for kiac:
#   * cluster via `kiac create cluster` (VM-isolated nodes), context kiac-<name>
#   * Gateway API EXPERIMENTAL channel (kgateway v2.3.1 watches TLSRoute)
#   * reachability via kiac-lb's real LoadBalancer IP (no host.k3d.internal,
#     no host port-maps)
#
# Requires: Apple silicon Mac, macOS 26+, apple/container 1.0.0+, kiac 0.3.0+.
# Validated end-to-end on macOS 26.2 / apple/container 1.0.0 / kiac v0.3.0 /
# OpenChoreo 1.1.2 (fresh cluster -> sample Component -> endpoint reachable).
# =============================================================================
set -euo pipefail

# ---- pinned versions (keep in sync with OpenChoreo install/k3d/single-cluster) ----
GATEWAY_API_VERSION="v1.5.1"
CERT_MANAGER_VERSION="v1.19.4"
ESO_VERSION="2.0.1"
KGATEWAY_VERSION="v2.3.1"
OPENBAO_CHART_VERSION="0.25.6"
THUNDER_VERSION="0.28.0"
HELM3_FALLBACK="v3.16.4"

# ---- defaults / flags ----
CLUSTER_NAME="openchoreo"
OPENCHOREO_VERSION="1.1.2"
WORKERS=2
CNI="kindnet"            # kindnet | cilium  (cilium enables NetworkPolicy enforcement)
K8S_VERSION="1.33"
CPUS="4"; CP_MEMORY="6G"; WORKER_MEMORY="8G"
USE_EXISTING=false       # --use-existing: install into the current kiac cluster, don't create one

CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
THUNDER_NS="thunder"
OPENBAO_NS="openbao"
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"
LB_IP=""

usage() {
  cat <<EOF
Usage: kiac-install.sh [OPTIONS]
  --version VER          OpenChoreo version (default: ${OPENCHOREO_VERSION})
  --cluster-name NAME    kiac cluster name (default: ${CLUSTER_NAME})
  --workers N            worker node VMs (default: ${WORKERS})
  --cni kindnet|cilium   pod network (cilium => NetworkPolicy enforcement; default: ${CNI})
  --use-existing         don't create a cluster; install into current kiac context
  -h, --help

Installs the OpenChoreo control plane and data plane (single-cluster). The
workflow (build) and observability planes are not yet wired for kiac.
EOF
}
while [[ $# -gt 0 ]]; do case $1 in
  --version) OPENCHOREO_VERSION="$2"; shift 2;;
  --cluster-name) CLUSTER_NAME="$2"; shift 2;;
  --workers) WORKERS="$2"; shift 2;;
  --cni) CNI="$2"; shift 2;;
  --use-existing) USE_EXISTING=true; shift;;
  -h|--help) usage; exit 0;;
  *) echo "unknown option: $1" >&2; usage >&2; exit 1;;
esac; done

RAW="https://raw.githubusercontent.com/openchoreo/openchoreo/v${OPENCHOREO_VERSION}"
CTX="kiac-${CLUSTER_NAME}"
K="kubectl --context ${CTX}"
CV="${OPENCHOREO_VERSION}"

step() { echo; echo "==> $1"; }
info() { echo "    $1"; }
fail() { echo "ERROR: $1" >&2; exit 1; }

# OpenChoreo charts target Helm 3.12+. If the system helm is v4 (or missing),
# transparently use a pinned Helm 3 in a cache dir. Sandbox plugins either way.
resolve_helm() {
  export HELM_PLUGINS="${TMPDIR:-/tmp}/kiac-oc/helm-plugins"; mkdir -p "$HELM_PLUGINS"
  local major=""
  if command -v helm >/dev/null 2>&1; then
    major="$(helm version --template '{{.Version}}' 2>/dev/null | sed 's/^v//' | cut -d. -f1 || true)"
    [[ "$major" == "3" ]] && { HELM="helm"; return; }
  fi
  local dir="${TMPDIR:-/tmp}/kiac-oc"; mkdir -p "$dir"
  if [[ ! -x "$dir/helm" ]]; then
    info "system helm is v${major:-none}; fetching pinned Helm ${HELM3_FALLBACK}"
    curl -fsSL "https://get.helm.sh/helm-${HELM3_FALLBACK}-darwin-arm64.tar.gz" \
      | tar xz -C "$dir" --strip-components=1 "darwin-arm64/helm"
  fi
  HELM="$dir/helm"
}
H() { $HELM --kube-context "${CTX}" "$@"; }

# kubectl apply that tolerates the brief window right after the control plane
# installs, where cert-manager rotates the admission-webhook serving cert and the
# controller-manager reloads it. During that window the API server rejects applies
# with "failed calling webhook ... x509: certificate signed by unknown authority".
kapply_retry() {   # kapply_retry <file-or-url>
  local src="$1" out i
  for i in $(seq 1 40); do
    if out=$($K apply -f "$src" 2>&1); then echo "$out"; return 0; fi
    echo "$out" | grep -qiE 'failed calling webhook|unknown authority|connection refused|no endpoints available' \
      && { info "control-plane webhook not ready yet, retrying ($i)..."; sleep 5; continue; }
    echo "$out" >&2; return 1
  done
  echo "$out" >&2; return 1
}

require_tools() {
  # Preflight: kiac is Apple-silicon/macOS only and needs the Apple container runtime.
  [[ "$(uname -s)" == "Darwin" && "$(uname -m)" == "arm64" ]] \
    || fail "kiac requires macOS on Apple silicon (got $(uname -s)/$(uname -m))"
  for t in kiac kubectl curl; do command -v "$t" >/dev/null 2>&1 || fail "missing tool: $t"; done
  kiac doctor >/dev/null 2>&1 || fail "kiac preflight failed — run 'kiac doctor' (is the Apple container service running?)"
  resolve_helm
  info "using helm: $($HELM version --short 2>/dev/null | head -1)"
}

create_cluster() {
  if $USE_EXISTING; then info "using existing cluster (context ${CTX})"; return; fi
  if kubectl config get-contexts -o name 2>/dev/null | grep -qx "${CTX}"; then
    info "context ${CTX} already exists, skipping cluster creation"; return
  fi
  step "Creating kiac cluster '${CLUSTER_NAME}' (${WORKERS} workers, ${CNI})"
  local extra=(); [[ "$CNI" == "cilium" ]] && extra=(--cni cilium --kernel full)
  kiac create cluster --name "$CLUSTER_NAME" --workers "$WORKERS" \
    --cpus "$CPUS" --cp-memory "$CP_MEMORY" --memory "$WORKER_MEMORY" \
    --k8s-version "$K8S_VERSION" "${extra[@]}"
}

install_gateway_api() {
  step "Installing Gateway API (EXPERIMENTAL channel ${GATEWAY_API_VERSION})"
  # kgateway ${KGATEWAY_VERSION} watches TLSRoute, which lives in the experimental
  # channel. A ValidatingAdmissionPolicy blocks layering experimental over standard,
  # so remove it first, then apply the experimental channel (a superset of standard).
  $K delete validatingadmissionpolicybinding safe-upgrades.gateway.networking.k8s.io --ignore-not-found
  $K delete validatingadmissionpolicy        safe-upgrades.gateway.networking.k8s.io --ignore-not-found
  $K apply --server-side -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"
}

install_prerequisites() {
  step "cert-manager ${CERT_MANAGER_VERSION}"
  H upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
    -n cert-manager --create-namespace --version "$CERT_MANAGER_VERSION" \
    --set crds.enabled=true --wait --timeout 300s
  step "External Secrets Operator ${ESO_VERSION}"
  H upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
    -n external-secrets --create-namespace --version "$ESO_VERSION" \
    --set installCRDs=true --wait --timeout 300s
  step "kgateway ${KGATEWAY_VERSION}"
  H upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
    --create-namespace -n "$CONTROL_PLANE_NS" --version "$KGATEWAY_VERSION"
  H upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
    -n "$CONTROL_PLANE_NS" --create-namespace --version "$KGATEWAY_VERSION"
  $K -n "$CONTROL_PLANE_NS" rollout status deploy/kgateway --timeout=300s || true
  step "OpenBao ${OPENBAO_CHART_VERSION}"
  H upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
    -n "$OPENBAO_NS" --create-namespace --version "$OPENBAO_CHART_VERSION" \
    --values "${RAW}/install/k3d/common/values-openbao.yaml" --wait --timeout 420s
  step "ClusterSecretStore"
  $K apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata: { name: external-secrets-openbao, namespace: ${OPENBAO_NS} }
---
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata: { name: default }
spec:
  provider:
    vault:
      server: "http://openbao.${OPENBAO_NS}.svc:8200"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "openchoreo-secret-writer-role"
          serviceAccountRef: { name: "external-secrets-openbao", namespace: "${OPENBAO_NS}" }
EOF
  $K wait --for=condition=Ready clustersecretstore/default --timeout=180s || true
}

seed_optional_secrets() {
  step "Seeding optional platform secrets"
  # OpenChoreo's backstage ExternalSecret references optional GitHub-integration keys
  # that aren't seeded by default; without them ESO fails the whole sync and backstage
  # never starts. Placeholders are fine unless you wire GitHub Actions/OAuth login.
  for kkey in backstage-github-actions-token backstage-github-oauth-client-secret; do
    $K exec -n "$OPENBAO_NS" openbao-0 -- sh -c \
      "bao kv get secret/${kkey} >/dev/null 2>&1 || bao kv put secret/${kkey} value=placeholder-unused" \
      >/dev/null 2>&1 || info "could not seed ${kkey} (continuing)"
  done
}

install_control_plane() {
  step "Thunder (identity provider) ${THUNDER_VERSION}"
  H upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
    -n "$THUNDER_NS" --create-namespace --version "$THUNDER_VERSION" \
    --values "${RAW}/install/k3d/common/values-thunder.yaml"
  $K wait -n "$THUNDER_NS" --for=condition=available --timeout=300s deployment -l app.kubernetes.io/name=thunder || true

  step "backstage ExternalSecret"
  $K apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata: { name: backstage-secrets, namespace: ${CONTROL_PLANE_NS} }
spec:
  refreshInterval: 1h
  secretStoreRef: { kind: ClusterSecretStore, name: default }
  target: { name: backstage-secrets }
  data:
    - { secretKey: backend-secret,             remoteRef: { key: backstage-backend-secret,             property: value } }
    - { secretKey: client-secret,              remoteRef: { key: backstage-client-secret,              property: value } }
    - { secretKey: jenkins-api-key,            remoteRef: { key: backstage-jenkins-api-key,            property: value } }
    - { secretKey: github-actions-token,       remoteRef: { key: backstage-github-actions-token,       property: value } }
    - { secretKey: github-oauth-client-secret, remoteRef: { key: backstage-github-oauth-client-secret, property: value } }
EOF
  $K wait -n "$CONTROL_PLANE_NS" --for=condition=Ready externalsecret/backstage-secrets --timeout=120s || true

  step "Control plane chart ${CV}"
  H upgrade --install openchoreo-control-plane "${HELM_REPO}/openchoreo-control-plane" \
    --version "$CV" -n "$CONTROL_PLANE_NS" --create-namespace \
    --values "${RAW}/install/k3d/single-cluster/values-cp.yaml" --timeout 600s
  $K wait -n "$CONTROL_PLANE_NS" --for=condition=available --timeout=420s deployment --all || true

  step "cluster-gateway CA -> ConfigMap"
  $K wait -n "$CONTROL_PLANE_NS" --for=condition=Ready certificate/cluster-gateway-ca --timeout=180s || true
  local ca; ca=$($K get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.ca\.crt}' | base64 -d)
  $K create configmap cluster-gateway-ca --from-literal=ca.crt="$ca" \
    -n "$CONTROL_PLANE_NS" --dry-run=client -o yaml | $K apply -f -
}

install_default_resources() {
  step "Default resources (project, environments, component types, pipeline, traits)"
  $K label namespace default openchoreo.dev/control-plane=true --overwrite
  kapply_retry "${RAW}/samples/getting-started/all.yaml"
}

install_data_plane() {
  step "Data plane chart ${CV}"
  $K create namespace "$DATA_PLANE_NS" --dry-run=client -o yaml | $K apply -f -
  local ca; ca=$($K get configmap cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.ca\.crt}')
  $K create configmap cluster-gateway-ca --from-literal=ca.crt="$ca" \
    -n "$DATA_PLANE_NS" --dry-run=client -o yaml | $K apply -f -
  H upgrade --install openchoreo-data-plane "${HELM_REPO}/openchoreo-data-plane" \
    --version "$CV" -n "$DATA_PLANE_NS" --create-namespace \
    --values "${RAW}/install/k3d/single-cluster/values-dp.yaml" --timeout 600s

  step "Registering the data plane (ClusterDataPlane)"
  $K wait -n "$DATA_PLANE_NS" --for=condition=Ready certificate/cluster-agent-dataplane-tls --timeout=180s || true
  local dca; dca=$($K get secret cluster-agent-tls -n "$DATA_PLANE_NS" -o jsonpath='{.data.ca\.crt}' | base64 -d)
  local dpfile; dpfile=$(mktemp)
  cat > "$dpfile" <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata: { name: default }
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$dca" | sed 's/^/        /')
  secretStoreRef: { name: default }
  gateway:
    ingress:
      external:
        http: { host: openchoreoapis.localhost, listenerName: http, port: 19080 }
        name: gateway-default
        namespace: ${DATA_PLANE_NS}
EOF
  kapply_retry "$dpfile"; rm -f "$dpfile"
}

wait_for_webhook() {
  step "Waiting for the control-plane admission webhook"
  # The controller-manager may restart once to pick up its freshly issued webhook cert.
  # Wait for it to settle so the first Component you apply isn't rejected mid-restart.
  $K -n "$CONTROL_PLANE_NS" rollout status deploy/controller-manager --timeout=300s || true
  for _ in $(seq 1 20); do
    local ep; ep=$($K get endpoints controller-manager-webhook-service -n "$CONTROL_PLANE_NS" \
      -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
    [[ -n "$ep" ]] && break
    sleep 3
  done
}

detect_lb_ip() {
  step "Detecting kiac-lb LoadBalancer IP"
  # kiac-lb assigns the kgateway Gateways a real, routable LoadBalancer IP. The
  # control-plane (:8080) and data-plane (:19080) gateways share it (ports differ),
  # so component endpoints are reachable from the Mac with no host port mapping.
  local ip=""
  for _ in $(seq 1 30); do
    ip=$($K get svc gateway-default -n "$CONTROL_PLANE_NS" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
    [[ -n "$ip" ]] && break
    sleep 3
  done
  if [[ -n "$ip" ]]; then LB_IP="$ip"; else
    info "LoadBalancer IP not assigned yet; run: kubectl get svc -A | grep LoadBalancer"
  fi
}

print_summary() {
  step "OpenChoreo is installed on kiac 🎉"
  local ip="${LB_IP:-<lb-ip>}"
  info "LoadBalancer IP : ${ip}   (context: ${CTX})"
  info ""
  info "Deploy a sample component:"
  echo  "    kubectl --context ${CTX} apply -f ${RAW}/samples/from-image/go-greeter-service/greeter-service.yaml"
  info ""
  info "Reach its endpoint directly through kiac-lb (no /etc/hosts needed):"
  echo  "    curl -H 'Host: development-default.openchoreoapis.localhost' http://${ip}:19080/greeter-service-http/greeter/greet?name=you"
  info ""
  info "Portal (add the line below to /etc/hosts first):"
  echo  "    ${ip} openchoreo.localhost api.openchoreo.localhost thunder.openchoreo.localhost"
  info "    then open http://openchoreo.localhost:8080  (admin@openchoreo.dev / Admin@123)"
  info "    Note: portal login relies on in-cluster resolution of *.openchoreo.localhost;"
  info "    on the kubeadm distro add a CoreDNS entry mapping those hosts to ${ip}."
  info ""
  info "Delete everything: kiac delete cluster --name ${CLUSTER_NAME}"
}

main() {
  require_tools
  create_cluster
  install_gateway_api
  install_prerequisites
  seed_optional_secrets
  install_control_plane
  wait_for_webhook
  install_default_resources
  install_data_plane
  detect_lb_ip
  print_summary
}
main
