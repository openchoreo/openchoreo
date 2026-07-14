#!/usr/bin/env bash
set -euo pipefail

# Installs OpenChoreo on a local k3d cluster end to end: cluster, prerequisites,
# control plane, data plane, and optionally the workflow and observability planes.
# Run from a checkout and it reads assets (values files, samples, templates) from
# disk; run via curl | bash and it fetches them for the requested --version.

# -- versions (keep in sync with install/k3d/single-cluster/README.md) --
GATEWAY_API_VERSION="v1.5.1"
CERT_MANAGER_VERSION="v1.19.4"
ESO_VERSION="2.0.1"
KGATEWAY_VERSION="v2.3.1"
OPENBAO_CHART_VERSION="0.25.6"
THUNDER_VERSION="0.28.0"
LOGS_OPENSEARCH_VERSION="0.5.3"
TRACES_OPENSEARCH_VERSION="0.4.2"
METRICS_PROMETHEUS_VERSION="0.6.1"
EVENTS_OTEL_COLLECTOR_VERSION="0.1.1"

# -- config --
CLUSTER_NAME="${CLUSTER_NAME:-openchoreo}"
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"
CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
WORKFLOW_PLANE_NS="openchoreo-workflow-plane"
OBSERVABILITY_NS="openchoreo-observability-plane"
THUNDER_NS="thunder"
OPENBAO_NS="openbao"

# User-facing version (e.g. 1.1.1, latest-dev). Drives the chart version and,
# when fetching remotely, the git ref that assets are pulled from.
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-}"

WITH_BUILD=false
WITH_OBSERVABILITY=false

usage() {
    cat <<EOF
Usage: k3d-install.sh [OPTIONS]

Options:
  --with-build             Also install the workflow plane (Argo Workflows + registry)
  --with-observability     Also install the observability plane (OpenSearch logs/traces + metrics)
  --version VER            OpenChoreo version to install, e.g. 1.1.1 or latest-dev
                           (required when not running from a checkout)
  --cluster-name NAME     k3d cluster name (default: openchoreo)
  -h, --help              Show this help
EOF
}

need_arg() { [[ $# -ge 2 ]] || { echo "ERROR: $1 requires an argument" >&2; exit 1; }; }

while [[ $# -gt 0 ]]; do
    case $1 in
        --with-build) WITH_BUILD=true; shift ;;
        --with-observability) WITH_OBSERVABILITY=true; shift ;;
        --version) need_arg "$@"; OPENCHOREO_VERSION="$2"; shift 2 ;;
        --cluster-name) need_arg "$@"; CLUSTER_NAME="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

KUBECTL="kubectl --context k3d-${CLUSTER_NAME}"
HELM="helm --kube-context k3d-${CLUSTER_NAME}"

# Assets come from a local checkout when the script is a real file sitting in one,
# otherwise they're fetched. Anchor on the script's own path (not the cwd) so a
# `curl | bash` run from inside a checkout still resolves as remote.
LOCAL_ROOT=""
_script="${BASH_SOURCE[0]:-}"
if [[ -f "$_script" ]]; then
    _root="$(cd "$(dirname "$_script")/../.." 2>/dev/null && pwd || true)"
    [[ -n "$_root" && -f "$_root/install/k3d/single-cluster/config.yaml" ]] && LOCAL_ROOT="$_root"
fi

if [[ -n "$LOCAL_ROOT" ]]; then
    SOURCE_MODE=local
else
    SOURCE_MODE=remote
fi

# Derive the chart version and (for remote mode) the git ref from --version:
#   1.1.1      -> chart 1.1.1              ref v1.1.1
#   latest-dev -> chart 0.0.0-latest-dev  ref main
#   <sha>      -> chart 0.0.0-<sha>        ref <sha>
OPENCHOREO_CHART_VERSION=""
_ver="${OPENCHOREO_VERSION#v}"
_ref=""
case "$_ver" in
    "")                             _ref="main" ;;
    latest)                         _ref="main" ;;
    latest-dev|0.0.0-latest-dev)    OPENCHOREO_CHART_VERSION="0.0.0-latest-dev"; _ref="main" ;;
    0.0.0-*)                        OPENCHOREO_CHART_VERSION="$_ver"; _ref="${_ver#0.0.0-}" ;;
    [0-9]*.[0-9]*.[0-9]*)           OPENCHOREO_CHART_VERSION="$_ver"; _ref="v$_ver" ;;
    *)                              OPENCHOREO_CHART_VERSION="$_ver"; _ref="$_ver" ;;
esac

RAW_BASE=""
if [[ "$SOURCE_MODE" == "remote" ]]; then
    if [[ -z "$OPENCHOREO_VERSION" ]]; then
        echo "ERROR: no local checkout found. Pass --version <ver> (e.g. latest-dev or 1.1.1) so assets can be fetched." >&2
        exit 1
    fi
    RAW_BASE="https://raw.githubusercontent.com/openchoreo/openchoreo/${_ref}"
fi

# asset <repo-relative-path> -> local path or raw URL; both accepted by kubectl -f / helm --values
asset() {
    if [[ "$SOURCE_MODE" == "remote" ]]; then
        echo "${RAW_BASE}/$1"
    else
        echo "${LOCAL_ROOT}/$1"
    fi
}

chart_version_args() {
    if [[ -n "$OPENCHOREO_CHART_VERSION" ]]; then
        echo "--version $OPENCHOREO_CHART_VERSION"
    fi
}

step() { echo ""; echo "==> $1"; }
info() { echo "    $1"; }
fail() { echo "ERROR: $1" >&2; exit 1; }

require_tools() {
    local missing=()
    for t in k3d kubectl helm docker; do
        command -v "$t" >/dev/null 2>&1 || missing+=("$t")
    done
    if [[ "$SOURCE_MODE" == "remote" ]]; then
        command -v curl >/dev/null 2>&1 || missing+=("curl")
    fi
    [[ ${#missing[@]} -eq 0 ]] || fail "missing required tools: ${missing[*]}"
    docker info >/dev/null 2>&1 || fail "docker daemon is not reachable"
}

create_cluster() {
    if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
        info "cluster '${CLUSTER_NAME}' already exists, skipping creation"
    else
        step "Creating k3d cluster '${CLUSTER_NAME}'"
        # Colima's embedded DNS breaks k3d's default DNS fix; disable it there.
        if docker info --format '{{.Name}}' 2>/dev/null | grep -qi colima; then
            info "Colima detected — disabling k3d DNS fix (K3D_FIX_DNS=0)"
            export K3D_FIX_DNS=0
        fi
        # Pass the name positionally so it overrides metadata.name in the config file.
        if [[ "$SOURCE_MODE" == "local" ]]; then
            k3d cluster create "$CLUSTER_NAME" --config "$(asset install/k3d/single-cluster/config.yaml)"
        else
            curl -fsSL "$(asset install/k3d/single-cluster/config.yaml)" | k3d cluster create "$CLUSTER_NAME" --config=-
        fi
    fi
}

install_prerequisites() {
    step "Installing Gateway API CRDs (${GATEWAY_API_VERSION})"
    $KUBECTL apply --server-side \
        -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"

    step "Installing cert-manager (${CERT_MANAGER_VERSION})"
    $HELM upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
        --namespace cert-manager --create-namespace \
        --version "$CERT_MANAGER_VERSION" \
        --set crds.enabled=true --wait --timeout 180s

    step "Installing External Secrets Operator (${ESO_VERSION})"
    $HELM upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
        --namespace external-secrets --create-namespace \
        --version "$ESO_VERSION" \
        --set installCRDs=true --wait --timeout 180s

    step "Installing kgateway (${KGATEWAY_VERSION})"
    $HELM upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
        --create-namespace --namespace "$CONTROL_PLANE_NS" --version "$KGATEWAY_VERSION"
    $HELM upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
        --namespace "$CONTROL_PLANE_NS" --create-namespace --version "$KGATEWAY_VERSION"

    step "Installing OpenBao (${OPENBAO_CHART_VERSION})"
    # values-openbao.yaml seeds the platform secrets and configures auth/policies via its postStart hook.
    $HELM upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
        --namespace "$OPENBAO_NS" --create-namespace \
        --version "$OPENBAO_CHART_VERSION" \
        --values "$(asset install/k3d/common/values-openbao.yaml)" \
        --wait --timeout 300s

    step "Creating ClusterSecretStore"
    $KUBECTL apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-secrets-openbao
  namespace: ${OPENBAO_NS}
---
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
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
          serviceAccountRef:
            name: "external-secrets-openbao"
            namespace: "${OPENBAO_NS}"
EOF

    step "Configuring CoreDNS rewrite"
    $KUBECTL apply -f "$(asset install/k3d/common/coredns-custom.yaml)"
}

install_control_plane() {
    step "Installing ThunderID (identity provider)"
    $HELM upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
        --namespace "$THUNDER_NS" --create-namespace \
        --version "$THUNDER_VERSION" \
        --values "$(asset install/k3d/common/values-thunder.yaml)"
    $KUBECTL wait -n "$THUNDER_NS" \
        --for=condition=available --timeout=300s deployment -l app.kubernetes.io/name=thunder

    step "Creating backstage ExternalSecret"
    $KUBECTL apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: backstage-secrets
  namespace: ${CONTROL_PLANE_NS}
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: backstage-secrets
  data:
    - secretKey: backend-secret
      remoteRef: { key: backstage-backend-secret, property: value }
    - secretKey: client-secret
      remoteRef: { key: backstage-client-secret, property: value }
    - secretKey: jenkins-api-key
      remoteRef: { key: backstage-jenkins-api-key, property: value }
    - secretKey: github-actions-token
      remoteRef: { key: backstage-github-actions-token, property: value }
    - secretKey: github-oauth-client-secret
      remoteRef: { key: backstage-github-oauth-client-secret, property: value }
EOF
    $KUBECTL wait -n "$CONTROL_PLANE_NS" \
        --for=condition=Ready externalsecret/backstage-secrets --timeout=120s

    step "Installing the control plane"
    # shellcheck disable=SC2046
    $HELM upgrade --install openchoreo-control-plane "$HELM_REPO/openchoreo-control-plane" \
        $(chart_version_args) \
        --namespace "$CONTROL_PLANE_NS" --create-namespace \
        --values "$(asset install/k3d/single-cluster/values-cp.yaml)"
    $KUBECTL wait -n "$CONTROL_PLANE_NS" \
        --for=condition=available --timeout=300s deployment --all

    # Populate the cluster-gateway CA ConfigMap the planes' agents verify against.
    step "Extracting cluster-gateway CA"
    $KUBECTL wait -n "$CONTROL_PLANE_NS" \
        --for=condition=Ready certificate/cluster-gateway-ca --timeout=120s
    local ca_crt
    ca_crt=$($KUBECTL get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" \
        -o jsonpath='{.data.ca\.crt}' | base64 -d)
    $KUBECTL create configmap cluster-gateway-ca \
        --from-literal=ca.crt="$ca_crt" \
        -n "$CONTROL_PLANE_NS" --dry-run=client -o yaml | $KUBECTL apply -f -
}

install_default_resources() {
    step "Installing default resources"
    $KUBECTL label namespace default openchoreo.dev/control-plane=true --overwrite
    $KUBECTL apply -f "$(asset samples/getting-started/all.yaml)"
}

# copy_gateway_ca <namespace> — give a plane's agent the cluster-gateway CA
copy_gateway_ca() {
    local ns="$1"
    $KUBECTL create namespace "$ns" --dry-run=client -o yaml | $KUBECTL apply -f -
    local ca_crt
    ca_crt=$($KUBECTL get configmap cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.ca\.crt}')
    $KUBECTL create configmap cluster-gateway-ca \
        --from-literal=ca.crt="$ca_crt" \
        -n "$ns" --dry-run=client -o yaml | $KUBECTL apply -f -
}

# agent_ca <namespace> — echo the plane agent's CA (waits for the TLS secret)
agent_ca() {
    local ns="$1"
    $KUBECTL wait -n "$ns" --for=jsonpath='{.data.ca\.crt}' secret/cluster-agent-tls --timeout=120s >&2
    $KUBECTL get secret cluster-agent-tls -n "$ns" -o jsonpath='{.data.ca\.crt}' | base64 -d
}

install_data_plane() {
    step "Installing the data plane"
    copy_gateway_ca "$DATA_PLANE_NS"
    # shellcheck disable=SC2046
    $HELM upgrade --install openchoreo-data-plane "$HELM_REPO/openchoreo-data-plane" \
        $(chart_version_args) \
        --namespace "$DATA_PLANE_NS" --create-namespace \
        --values "$(asset install/k3d/single-cluster/values-dp.yaml)"

    step "Registering the data plane"
    local ca; ca=$(agent_ca "$DATA_PLANE_NS")
    $KUBECTL apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca" | sed 's/^/        /')
  secretStoreRef:
    name: default
  gateway:
    ingress:
      external:
        http:
          host: openchoreoapis.localhost
          listenerName: http
          port: 19080
        name: gateway-default
        namespace: openchoreo-data-plane
EOF
}

install_workflow_plane() {
    step "Installing the workflow plane"
    copy_gateway_ca "$WORKFLOW_PLANE_NS"

    helm repo add twuni https://twuni.github.io/docker-registry.helm >/dev/null 2>&1 || true
    helm repo update twuni >/dev/null
    $HELM upgrade --install registry twuni/docker-registry \
        --namespace "$WORKFLOW_PLANE_NS" --create-namespace \
        --values "$(asset install/k3d/single-cluster/values-registry.yaml)"

    # shellcheck disable=SC2046
    $HELM upgrade --install openchoreo-workflow-plane "$HELM_REPO/openchoreo-workflow-plane" \
        $(chart_version_args) \
        --namespace "$WORKFLOW_PLANE_NS" \
        --values "$(asset install/k3d/single-cluster/values-wp.yaml)"

    step "Installing workflow templates"
    $KUBECTL apply \
        -f "$(asset samples/getting-started/workflow-templates/checkout-source.yaml)" \
        -f "$(asset samples/getting-started/workflow-templates.yaml)" \
        -f "$(asset samples/getting-started/workflow-templates/publish-image-k3d.yaml)" \
        -f "$(asset samples/getting-started/workflow-templates/generate-workload-k3d.yaml)"

    step "Registering the workflow plane"
    local ca; ca=$(agent_ca "$WORKFLOW_PLANE_NS")
    $KUBECTL apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterWorkflowPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca" | sed 's/^/        /')
  secretStoreRef:
    name: default
EOF
}

install_observability_plane() {
    step "Installing the observability plane"
    copy_gateway_ca "$OBSERVABILITY_NS"

    step "Creating observability ExternalSecrets"
    $KUBECTL apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: opensearch-admin-credentials
  namespace: ${OBSERVABILITY_NS}
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: opensearch-admin-credentials
  data:
    - secretKey: username
      remoteRef: { key: opensearch-username, property: value }
    - secretKey: password
      remoteRef: { key: opensearch-password, property: value }
---
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-secret
  namespace: ${OBSERVABILITY_NS}
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-secret
  data:
    - secretKey: UID_RESOLVER_OAUTH_CLIENT_SECRET
      remoteRef: { key: observer-oauth-client-secret, property: value }
EOF
    $KUBECTL wait -n "$OBSERVABILITY_NS" \
        --for=condition=Ready externalsecret/opensearch-admin-credentials \
        externalsecret/observer-secret --timeout=120s

    # Fluent Bit needs /etc/machine-id; k3d nodes don't have one by default.
    docker exec "k3d-${CLUSTER_NAME}-server-0" sh -c \
        "[ -s /etc/machine-id ] || cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"

    # shellcheck disable=SC2046
    $HELM upgrade --install openchoreo-observability-plane "$HELM_REPO/openchoreo-observability-plane" \
        $(chart_version_args) \
        --namespace "$OBSERVABILITY_NS" \
        --values "$(asset install/k3d/single-cluster/values-op.yaml)" \
        --timeout 25m

    step "Installing observability modules"
    $HELM upgrade --install observability-logs-opensearch \
        oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
        --namespace "$OBSERVABILITY_NS" --version "$LOGS_OPENSEARCH_VERSION" \
        --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
        --set adapter.openSearchSecretName="opensearch-admin-credentials"
    $HELM upgrade --install observability-traces-opensearch \
        oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
        --namespace "$OBSERVABILITY_NS" --version "$TRACES_OPENSEARCH_VERSION" \
        --set openSearch.enabled=false \
        --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials"
    $HELM upgrade --install observability-metrics-prometheus \
        oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
        --namespace "$OBSERVABILITY_NS" --version "$METRICS_PROMETHEUS_VERSION"
    $HELM upgrade observability-logs-opensearch \
        oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
        --namespace "$OBSERVABILITY_NS" --version "$LOGS_OPENSEARCH_VERSION" \
        --reuse-values --set fluent-bit.enabled=true

    # Collect Kubernetes events into the k8s-events OpenSearch index.
    $HELM upgrade --install observability-events-otel-collector \
        oci://ghcr.io/openchoreo/helm-charts/observability-events-otel-collector \
        --namespace "$OBSERVABILITY_NS" --version "$EVENTS_OTEL_COLLECTOR_VERSION" \
        -f - <<'EOF'
collector:
  extraEnv:
    - name: OPENSEARCH_USERNAME
      valueFrom:
        secretKeyRef:
          name: opensearch-admin-credentials
          key: username
    - name: OPENSEARCH_PASSWORD
      valueFrom:
        secretKeyRef:
          name: opensearch-admin-credentials
          key: password
extraExtensions:
  basicauth/opensearch:
    client_auth:
      username: ${env:OPENSEARCH_USERNAME}
      password: ${env:OPENSEARCH_PASSWORD}
exporters:
  opensearch:
    logs_index: "k8s-events"
    logs_index_time_format: "yyyy-MM-dd"
    http:
      endpoint: "https://opensearch:9200"
      tls:
        insecure_skip_verify: true
      auth:
        authenticator: basicauth/opensearch
pipelineExporters:
  - opensearch
EOF

    step "Registering the observability plane"
    local ca; ca=$(agent_ca "$OBSERVABILITY_NS")
    $KUBECTL apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterObservabilityPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca" | sed 's/^/        /')
  observerURL: http://observer.openchoreo.localhost:11080
EOF

    # Link whichever planes exist to the observability plane (not gated on --with-build,
    # so a workflow plane from an earlier run still gets linked on an observability-only re-run).
    if $KUBECTL get clusterdataplane default >/dev/null 2>&1; then
        $KUBECTL patch clusterdataplane default --type merge \
            -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'
    fi
    if $KUBECTL get clusterworkflowplane default >/dev/null 2>&1; then
        $KUBECTL patch clusterworkflowplane default --type merge \
            -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'
    fi
}

print_summary() {
    step "OpenChoreo installation complete"
    info "Console:  http://openchoreo.localhost:8080  (log in with admin@openchoreo.dev / Admin@123)"
    info "API:      http://api.openchoreo.localhost:8080"
    info "Planes:   control, data$([[ "$WITH_BUILD" == "true" ]] && echo ", workflow")$([[ "$WITH_OBSERVABILITY" == "true" ]] && echo ", observability")"
    info "Delete:   k3d cluster delete ${CLUSTER_NAME}"
}

main() {
    require_tools
    create_cluster
    install_prerequisites
    install_control_plane
    install_default_resources
    install_data_plane
    [[ "$WITH_BUILD" == "true" ]] && install_workflow_plane
    [[ "$WITH_OBSERVABILITY" == "true" ]] && install_observability_plane
    print_summary
}

main
