# Single-Cluster Setup

All-in-one OpenChoreo setup with all planes running in a single k3d cluster.

> [!TIP]
> For a detailed walkthrough with explanations, see the [public getting started guide](https://openchoreo.dev/docs/getting-started/try-it-out/locally/).

> [!IMPORTANT]
> If you're using Colima, set `K3D_FIX_DNS=0` when creating clusters.
> See [k3d-io/k3d#1449](https://github.com/k3d-io/k3d/issues/1449).

## 1. Create Cluster

```bash
k3d cluster create --config install/k3d/single-cluster/config.yaml

docker exec k3d-openchoreo-server-0 sh -c \
  "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
```

> [!TIP]
> For faster setup, consider using [Image Preloading](#image-preloading) after creating the cluster.

## 2. Install Prerequisites

### Gateway API CRDs

```bash
kubectl apply --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml
```

### cert-manager

```bash
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true
```

```bash
kubectl wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s
```

### External Secrets Operator

```bash
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true
```

```bash
kubectl wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s
```

### kgateway

Single install, watches Gateway resources across all namespaces.

```bash
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --create-namespace --namespace openchoreo-control-plane \
  --version v2.2.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --namespace openchoreo-control-plane --create-namespace \
  --version v2.2.1 \
  --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true
```

## 3. Setup Control Plane

### Thunder (Identity Provider)

Bootstrap scripts auto-configure the org, users, groups, and OAuth apps on first startup.

```bash
helm upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --version 0.23.0 \
  --values install/k3d/common/values-thunder.yaml
```

### CoreDNS Rewrite

```bash
kubectl apply -f install/k3d/common/coredns-custom.yaml
```

### Backstage Secrets

```bash
kubectl create namespace openchoreo-control-plane --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic backstage-secrets \
  -n openchoreo-control-plane \
  --from-literal=backend-secret="$(head -c 32 /dev/urandom | base64)" \
  --from-literal=client-secret="backstage-portal-secret" \
  --from-literal=jenkins-api-key="placeholder-not-in-use"
```

### Install Control Plane

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-cp.yaml
```

```bash
kubectl wait -n openchoreo-control-plane \
  --for=condition=available --timeout=300s deployment --all
```

## 4. Install Default Resources

```bash
kubectl apply -f samples/getting-started/all.yaml
kubectl label namespace default openchoreo.dev/controlplane-namespace=true
```

## 5. Setup Data Plane

### Namespace and Certificates

```bash
kubectl create namespace openchoreo-data-plane --dry-run=client -o yaml | kubectl apply -f -

CA_CRT=$(kubectl get configmap cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}')

kubectl create configmap cluster-gateway-ca \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-data-plane

TLS_CRT=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.crt}' | base64 -d)
TLS_KEY=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.key}' | base64 -d)

kubectl create secret generic cluster-gateway-ca \
  --from-literal=tls.crt="$TLS_CRT" \
  --from-literal=tls.key="$TLS_KEY" \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-data-plane
```

### Secret Store

```bash
kubectl apply -f - <<EOF
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
EOF
```

### Install Data Plane

```bash
helm upgrade --install openchoreo-data-plane install/helm/openchoreo-data-plane \
  --dependency-update \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-dp.yaml
```

### Register Data Plane

```bash
AGENT_CA=$(kubectl get secret cluster-agent-tls \
  -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl apply -f - <<EOF
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

## 6. Setup Build Plane (Optional)

### Namespace and Certificates

```bash
kubectl create namespace openchoreo-build-plane --dry-run=client -o yaml | kubectl apply -f -

CA_CRT=$(kubectl get configmap cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}')

kubectl create configmap cluster-gateway-ca \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-build-plane

TLS_CRT=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.crt}' | base64 -d)
TLS_KEY=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.key}' | base64 -d)

kubectl create secret generic cluster-gateway-ca \
  --from-literal=tls.crt="$TLS_CRT" \
  --from-literal=tls.key="$TLS_KEY" \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-build-plane
```

### Container Registry

```bash
helm repo add twuni https://twuni.github.io/docker-registry.helm
helm repo update

helm install registry twuni/docker-registry \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-registry.yaml
```

### Install Build Plane

```bash
helm upgrade --install openchoreo-build-plane install/helm/openchoreo-build-plane \
  --dependency-update \
  --namespace openchoreo-build-plane \
  --values install/k3d/single-cluster/values-bp.yaml
```

### Workflow Templates

The build pipeline is composed of shared ClusterWorkflowTemplates that each handle one step (checkout, build, publish, generate workload). The checkout and publish templates are applied separately so you can replace them to use your own git auth or container registry.

```bash
kubectl apply -f samples/getting-started/workflow-templates/checkout-source.yaml
kubectl apply -f samples/getting-started/workflow-templates.yaml
kubectl apply -f samples/getting-started/workflow-templates/publish-image-k3d.yaml
```

`publish-image-k3d.yaml` pushes images to the local k3d registry at `host.k3d.internal:10082`. To use a different registry, replace this with your own `publish-image` ClusterWorkflowTemplate.

### Buildpack Cache (Optional)

Pre-populates the local registry with buildpack images so Ballerina and Google Cloud Buildpacks workflows don't pull from remote registries on every build. Skip this if you only use Docker or React builds.

```bash
kubectl apply -f install/k3d/common/push-buildpack-cache-images.yaml
```

### Register Build Plane

```bash
AGENT_CA=$(kubectl get secret cluster-agent-tls \
  -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl apply -f - <<EOF
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

## 7. Setup Observability Plane (Optional)

### Namespace and Certificates

```bash
kubectl create namespace openchoreo-observability-plane --dry-run=client -o yaml | kubectl apply -f -

CA_CRT=$(kubectl get configmap cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}')

kubectl create configmap cluster-gateway-ca \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-observability-plane

TLS_CRT=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.crt}' | base64 -d)
TLS_KEY=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.tls\.key}' | base64 -d)

kubectl create secret generic cluster-gateway-ca \
  --from-literal=tls.crt="$TLS_CRT" \
  --from-literal=tls.key="$TLS_KEY" \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-observability-plane
```

### OpenSearch Credentials

```bash
kubectl create secret generic opensearch-admin-credentials \
  -n openchoreo-observability-plane \
  --from-literal=username="admin" \
  --from-literal=password="ThisIsTheOpenSearchPassword1" 

kubectl create secret generic observer-opensearch-credentials \
  -n openchoreo-observability-plane \
  --from-literal=username="admin" \
  --from-literal=password="ThisIsTheOpenSearchPassword1"
```

### Install Observability Plane

```bash
helm upgrade --install openchoreo-observability-plane install/helm/openchoreo-observability-plane \
  --dependency-update \
  --namespace openchoreo-observability-plane \
  --values install/k3d/single-cluster/values-op.yaml \
  --timeout 10m
```

#### Install Observability Modules

Install the required logs, metrics and tracing modules. Refer https://openchoreo.dev/modules for more details


### Register Observability Plane

```bash
AGENT_CA=$(kubectl get secret cluster-agent-tls \
  -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl apply -f - <<EOF
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
kubectl patch dataplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'

# If build plane is installed:
kubectl patch buildplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'
```

## Port Mappings

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
| 11085 | Observability | Gateway HTTPS          |
| 11081 | Observability | OpenSearch Dashboards* |
| 11082 | Observability | OpenSearch API*        |

*OpenSearch ports are not 1:1 (11081:5601, 11082:9200) since those services don't support port overrides.

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
kubectl get pods -n openchoreo-control-plane
kubectl get pods -n openchoreo-data-plane
kubectl get pods -n openchoreo-build-plane
kubectl get pods -n openchoreo-observability-plane

# Plane resources
kubectl get dataplane,buildplane,observabilityplane -n default

# Agent connections
kubectl logs -n openchoreo-data-plane -l app=cluster-agent --tail=5
kubectl logs -n openchoreo-build-plane -l app=cluster-agent --tail=5
kubectl logs -n openchoreo-observability-plane -l app=cluster-agent --tail=5
```

## Image Preloading

Pull images to your host first, then import into the cluster. Useful for slow networks or frequent cluster recreation.

```bash
# All planes
install/k3d/preload-images.sh \
  --cluster openchoreo \
  --local-charts \
  --control-plane --cp-values install/k3d/single-cluster/values-cp.yaml \
  --data-plane --dp-values install/k3d/single-cluster/values-dp.yaml \
  --build-plane --bp-values install/k3d/single-cluster/values-bp.yaml \
  --observability-plane --op-values install/k3d/single-cluster/values-op.yaml \
  --parallel 4
```

Run after creating the cluster but before installing anything.

## Cleanup

```bash
k3d cluster delete openchoreo
```
