# Multi-Cluster Setup

Production-like setup with each OpenChoreo plane running in its own k3d cluster.

Planes communicate via **cluster agents** over WebSocket. Data/Build/Observability
plane agents connect to the Control Plane's cluster-gateway. Secured with mTLS,
no need to expose Kubernetes APIs externally.

> [!IMPORTANT]
> Running all 4 clusters requires raising the inotify limit. Without this, k3s nodes
> fail with "too many open files" errors.
> ```bash
> # On Linux (persists until reboot)
> sudo sysctl -w fs.inotify.max_user_instances=1024
> sudo sysctl -w fs.inotify.max_user_watches=524288
>
> # On macOS with Colima/Docker Desktop, run inside the VM:
> docker run --rm --privileged alpine sysctl -w fs.inotify.max_user_instances=1024
> docker run --rm --privileged alpine sysctl -w fs.inotify.max_user_watches=524288
> ```

> [!IMPORTANT]
> If you're using Colima, set `K3D_FIX_DNS=0` when creating clusters.
> See [k3d-io/k3d#1449](https://github.com/k3d-io/k3d/issues/1449).

> [!TIP]
> For faster setup, consider using [Image Preloading](#image-preloading) after creating clusters.

## 1. Control Plane

```bash
k3d cluster create --config install/k3d/multi-cluster/config-cp.yaml
```

### Prerequisites

```bash
# Gateway API CRDs
kubectl apply --context k3d-openchoreo-cp --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml

# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-cp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-cp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --kube-context k3d-openchoreo-cp \
  --version v2.1.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --kube-context k3d-openchoreo-cp \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --version v2.1.1
```

### Thunder (Identity Provider)

```bash
helm upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
  --kube-context k3d-openchoreo-cp \
  --namespace openchoreo-control-plane \
  --version 0.21.0 \
  --values install/k3d/common/values-thunder.yaml
```

### CoreDNS Rewrite

```bash
kubectl apply --context k3d-openchoreo-cp -f install/k3d/common/coredns-custom.yaml
```

### Backstage Secrets

```bash
kubectl --context k3d-openchoreo-cp create namespace openchoreo-control-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-cp apply -f -

kubectl --context k3d-openchoreo-cp create secret generic backstage-secrets \
  -n openchoreo-control-plane \
  --from-literal=backend-secret="$(head -c 32 /dev/urandom | base64)" \
  --from-literal=client-secret="backstage-portal-secret" \
  --from-literal=jenkins-api-key="placeholder-not-in-use"
```

### Install Control Plane

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --kube-context k3d-openchoreo-cp \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-cp.yaml
```

```bash
kubectl --context k3d-openchoreo-cp wait -n openchoreo-control-plane \
  --for=condition=available --timeout=300s deployment --all
```

### Gateway Patch

Workaround for envoy `/tmp` crash ([kgateway#9800](https://github.com/kgateway-dev/kgateway/issues/9800)).

```bash
kubectl --context k3d-openchoreo-cp patch deployment gateway-default \
  -n openchoreo-control-plane \
  --type='json' -p='[{"op":"add","path":"/spec/template/spec/volumes/-","value":{"name":"tmp","emptyDir":{}}},{"op":"add","path":"/spec/template/spec/containers/0/volumeMounts/-","value":{"name":"tmp","mountPath":"/tmp"}}]'
```

## 2. Install Default Resources

```bash
kubectl --context k3d-openchoreo-cp apply -f samples/getting-started/all.yaml
kubectl --context k3d-openchoreo-cp label namespace default openchoreo.dev/controlplane-namespace=true
```

## 3. Data Plane

```bash
k3d cluster create --config install/k3d/multi-cluster/config-dp.yaml

docker exec k3d-openchoreo-dp-server-0 sh -c \
  "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
```

### Prerequisites

```bash
# Gateway API CRDs
kubectl apply --context k3d-openchoreo-dp --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml

# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-dp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-dp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-dp \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-dp wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --kube-context k3d-openchoreo-dp \
  --version v2.1.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --kube-context k3d-openchoreo-dp \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --version v2.1.1
```

### CoreDNS Rewrite and Certificates

```bash
kubectl apply --context k3d-openchoreo-dp -f install/k3d/common/coredns-custom.yaml

kubectl --context k3d-openchoreo-dp create namespace openchoreo-data-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-dp apply -f -

# Copy cluster-gateway CA from control plane
kubectl --context k3d-openchoreo-cp get secret cluster-gateway-ca \
  -n openchoreo-control-plane \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/server-ca.crt

kubectl --context k3d-openchoreo-dp create configmap cluster-gateway-ca \
  --from-file=ca.crt=/tmp/server-ca.crt \
  -n openchoreo-data-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-dp apply -f -
```

### Secret Store

```bash
kubectl --context k3d-openchoreo-dp apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    fake:
      data:
      - key: npm-token
        value: "fake-npm-token-for-development"
      - key: docker-username
        value: "dev-user"
      - key: docker-password
        value: "dev-password"
      - key: github-pat
        value: "fake-github-token-for-development"
      - key: username
        value: "dev-user"
      - key: password
        value: "dev-password"
      - key: RCA_LLM_API_KEY
        value: "fake-llm-api-key-for-development"
EOF
```

### Install Data Plane

```bash
helm upgrade --install openchoreo-data-plane install/helm/openchoreo-data-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-dp \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-dp.yaml
```

```bash
kubectl --context k3d-openchoreo-dp wait -n openchoreo-data-plane \
  --for=condition=available --timeout=300s deployment --all
```

### Gateway Patch

```bash
kubectl --context k3d-openchoreo-dp patch deployment gateway-default \
  -n openchoreo-data-plane \
  --type='json' -p='[{"op":"add","path":"/spec/template/spec/volumes/-","value":{"name":"tmp","emptyDir":{}}},{"op":"add","path":"/spec/template/spec/containers/0/volumeMounts/-","value":{"name":"tmp","mountPath":"/tmp"}}]'
```

### Register Data Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-dp get secret cluster-agent-tls \
  -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
  secretStoreRef:
    name: default
  gateway:
    publicVirtualHost: openchoreoapis.localhost
    publicHTTPPort: 19080
    publicHTTPSPort: 19443
EOF
```

## 4. Build Plane (Optional)

```bash
k3d cluster create --config install/k3d/multi-cluster/config-bp.yaml

docker exec k3d-openchoreo-bp-server-0 sh -c \
  "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
```

### Prerequisites

```bash
# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-bp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-bp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-bp \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-bp wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s
```

### CoreDNS Rewrite and Certificates

```bash
kubectl apply --context k3d-openchoreo-bp -f install/k3d/common/coredns-custom.yaml

kubectl --context k3d-openchoreo-bp create namespace openchoreo-build-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-bp apply -f -

# Copy cluster-gateway CA from control plane
kubectl --context k3d-openchoreo-cp get secret cluster-gateway-ca \
  -n openchoreo-control-plane \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/server-ca.crt

kubectl --context k3d-openchoreo-bp create configmap cluster-gateway-ca \
  --from-file=ca.crt=/tmp/server-ca.crt \
  -n openchoreo-build-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-bp apply -f -
```

### Container Registry

```bash
helm repo add twuni https://twuni.github.io/docker-registry.helm
helm repo update

helm install registry twuni/docker-registry \
  --kube-context k3d-openchoreo-bp \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-registry.yaml
```

### Install Build Plane

```bash
helm upgrade --install openchoreo-build-plane install/helm/openchoreo-build-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-bp \
  --namespace openchoreo-build-plane \
  --values install/k3d/multi-cluster/values-bp.yaml
```

### Register Build Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-bp get secret cluster-agent-tls \
  -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
  secretStoreRef:
    name: openbao
EOF
```

## 5. Observability Plane (Optional)

```bash
k3d cluster create --config install/k3d/multi-cluster/config-op.yaml

docker exec k3d-openchoreo-op-server-0 sh -c \
  "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
```

### Prerequisites

```bash
# Gateway API CRDs
kubectl apply --context k3d-openchoreo-op --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml

# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-op \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-op wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-op \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-op wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --kube-context k3d-openchoreo-op \
  --version v2.1.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --kube-context k3d-openchoreo-op \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --version v2.1.1
```

### CoreDNS Rewrite and Certificates

```bash
kubectl apply --context k3d-openchoreo-op -f install/k3d/common/coredns-custom.yaml

kubectl --context k3d-openchoreo-op create namespace openchoreo-observability-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-op apply -f -

# Copy cluster-gateway CA from control plane
kubectl --context k3d-openchoreo-cp get secret cluster-gateway-ca \
  -n openchoreo-control-plane \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/server-ca.crt

kubectl --context k3d-openchoreo-op create configmap cluster-gateway-ca \
  --from-file=ca.crt=/tmp/server-ca.crt \
  -n openchoreo-observability-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-op apply -f -
```

### OpenSearch Credentials

```bash
kubectl --context k3d-openchoreo-op create secret generic observer-opensearch-credentials \
  -n openchoreo-observability-plane \
  --from-literal=username="admin" \
  --from-literal=password="ThisIsTheOpenSearchPassword1"
```

### Install Observability Plane

```bash
helm upgrade --install openchoreo-observability-plane install/helm/openchoreo-observability-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-op \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-op.yaml \
  --timeout 10m
```

```bash
kubectl --context k3d-openchoreo-op wait -n openchoreo-observability-plane \
  --for=condition=available --timeout=600s deployment --all
```

### Gateway Patch

```bash
kubectl --context k3d-openchoreo-op patch deployment gateway-default \
  -n openchoreo-observability-plane \
  --type='json' -p='[{"op":"add","path":"/spec/template/spec/volumes/-","value":{"name":"tmp","emptyDir":{}}},{"op":"add","path":"/spec/template/spec/containers/0/volumeMounts/-","value":{"name":"tmp","mountPath":"/tmp"}}]'
```

### Register Observability Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-op get secret cluster-agent-tls \
  -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
  observerURL: http://observer.openchoreo.localhost:11080
EOF
```

### Link Other Planes

```bash
kubectl --context k3d-openchoreo-cp patch dataplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'

# If build plane is installed:
kubectl --context k3d-openchoreo-cp patch buildplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'
```

## Port Mappings

| Plane               | Cluster           | Kube API | Port Range |
|---------------------|-------------------|----------|------------|
| Control Plane       | k3d-openchoreo-cp | 6550     | 8xxx       |
| Data Plane          | k3d-openchoreo-dp | 6551     | 19xxx      |
| Build Plane         | k3d-openchoreo-bp | 6552     | 10xxx      |
| Observability Plane | k3d-openchoreo-op | 6553     | 11xxx      |

All ports are mapped 1:1 (host:container) unless noted.

| Port  | Plane         | Service                |
|-------|---------------|------------------------|
| 8080  | Control       | Gateway HTTP           |
| 8443  | Control       | Gateway HTTPS          |
| 19080 | Data          | Gateway HTTP           |
| 19443 | Data          | Gateway HTTPS          |
| 10081 | Build         | Argo Workflows UI      |
| 10082 | Build         | Container Registry     |
| 11080 | Observability | Observer API (HTTP)    |
| 11081 | Observability | OpenSearch Dashboards* |
| 11082 | Observability | OpenSearch API*        |
| 11084 | Observability | Prometheus*            |
| 11086 | Observability | OpenTelemetry*         |

*Not 1:1 mappings (11081:5601, 11082:9200, 11084:9091, 11086:4317).

## Access Services

| Service              | URL                                           |
|----------------------|-----------------------------------------------|
| OpenChoreo Console   | http://openchoreo.localhost:8080               |
| OpenChoreo API       | http://api.openchoreo.localhost:8080           |
| Thunder Admin        | http://thunder.openchoreo.localhost:8080       |
| Argo Workflows UI    | http://localhost:10081                         |
| Observer API         | http://observer.openchoreo.localhost:11080     |

## Verification

```bash
# All pods
kubectl --context k3d-openchoreo-cp get pods -n openchoreo-control-plane
kubectl --context k3d-openchoreo-dp get pods -n openchoreo-data-plane
kubectl --context k3d-openchoreo-bp get pods -n openchoreo-build-plane
kubectl --context k3d-openchoreo-op get pods -n openchoreo-observability-plane

# Plane resources
kubectl --context k3d-openchoreo-cp get dataplane,buildplane,observabilityplane -n default

# Agent connections
kubectl --context k3d-openchoreo-dp logs -n openchoreo-data-plane -l app.kubernetes.io/component=cluster-agent --tail=5
kubectl --context k3d-openchoreo-bp logs -n openchoreo-build-plane -l app.kubernetes.io/component=cluster-agent --tail=5
kubectl --context k3d-openchoreo-op logs -n openchoreo-observability-plane -l app.kubernetes.io/component=cluster-agent --tail=5
```

## Image Preloading

Pull images to your host first, then import into each cluster.

```bash
# Control Plane
install/k3d/preload-images.sh \
  --cluster openchoreo-cp \
  --local-charts \
  --control-plane --cp-values install/k3d/multi-cluster/values-cp.yaml \
  --parallel 4

# Data Plane
install/k3d/preload-images.sh \
  --cluster openchoreo-dp \
  --local-charts \
  --data-plane --dp-values install/k3d/multi-cluster/values-dp.yaml \
  --parallel 4

# Build Plane
install/k3d/preload-images.sh \
  --cluster openchoreo-bp \
  --local-charts \
  --build-plane --bp-values install/k3d/multi-cluster/values-bp.yaml \
  --parallel 4

# Observability Plane
install/k3d/preload-images.sh \
  --cluster openchoreo-op \
  --local-charts \
  --observability-plane --op-values install/k3d/multi-cluster/values-op.yaml \
  --parallel 4
```

Run after creating clusters but before installing anything.

## Cleanup

```bash
k3d cluster delete openchoreo-cp openchoreo-dp openchoreo-bp openchoreo-op
```
