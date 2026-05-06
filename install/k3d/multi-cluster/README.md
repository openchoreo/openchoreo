# Multi-Cluster Setup

Production-like setup with each OpenChoreo plane running in its own k3d cluster.

> [!NOTE]
> This guide is for **contributors and developers** working from a local checkout.
> It uses local Helm charts (`install/helm/...`) and standalone setup scripts.
> If you just want to try OpenChoreo, follow the [public getting-started guide](https://openchoreo.dev/docs/getting-started/try-it-out/on-your-environment/) instead.

Planes communicate via **cluster agents** over WebSocket. Data/Build/Observability
plane agents connect to the Control Plane's cluster-gateway. Secured with mTLS,
no need to expose Kubernetes APIs externally.

> [!IMPORTANT]
> Running all 4 clusters requires raising the inotify limit. Without this, k3s nodes
> fail with "too many open files" errors.
>
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
  --version v1.19.4 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-cp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --create-namespace --namespace openchoreo-control-plane --kube-context k3d-openchoreo-cp \
  --version v2.2.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --namespace openchoreo-control-plane --create-namespace --kube-context k3d-openchoreo-cp \
  --version v2.2.1 \
  --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true
```

### Thunder (Identity Provider)

```bash
helm upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
  --kube-context k3d-openchoreo-cp \
  --namespace thunder \
  --create-namespace \
  --version 0.28.0 \
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

## 2. Install Default Resources

```bash
kubectl --context k3d-openchoreo-cp apply -f samples/getting-started/all.yaml
kubectl --context k3d-openchoreo-cp label namespace default openchoreo.dev/control-plane=true
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
  --version v1.19.4 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-dp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-dp \
  --namespace external-secrets \
  --create-namespace \
  --version 2.0.1 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-dp wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --create-namespace --namespace openchoreo-data-plane --kube-context k3d-openchoreo-dp \
  --version v2.2.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --namespace openchoreo-data-plane --create-namespace --kube-context k3d-openchoreo-dp \
  --version v2.2.1 \
  --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true
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

### OpenBao (Secret Store)

Install [OpenBao](https://openbao.org/) as the secret backend on the data plane cluster:

```bash
install/prerequisites/openbao/setup.sh --dev --seed-dev-secrets --kube-context k3d-openchoreo-dp
```

This installs OpenBao in dev mode into the `openbao` namespace, configures Kubernetes auth, seeds placeholder development secrets, and creates a `ClusterSecretStore` named `default`.

To use a different secret backend, skip this and create your own `ClusterSecretStore` named `default` following the [ESO provider docs](https://external-secrets.io/latest/provider/).

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

### Register Data Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-dp get secret cluster-agent-tls \
  -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
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
```

## 4. Workflow Plane (Optional)

```bash
k3d cluster create --config install/k3d/multi-cluster/config-wp.yaml

docker exec k3d-openchoreo-wp-server-0 sh -c \
  "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
```

### Prerequisites

```bash
# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-wp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.4 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-wp wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-wp \
  --namespace external-secrets \
  --create-namespace \
  --version 2.0.1 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-wp wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s
```

### CoreDNS Rewrite and Certificates

```bash
kubectl apply --context k3d-openchoreo-wp -f install/k3d/common/coredns-custom.yaml

kubectl --context k3d-openchoreo-wp create namespace openchoreo-workflow-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-wp apply -f -

# Copy cluster-gateway CA from control plane
kubectl --context k3d-openchoreo-cp get secret cluster-gateway-ca \
  -n openchoreo-control-plane \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/server-ca.crt

kubectl --context k3d-openchoreo-wp create configmap cluster-gateway-ca \
  --from-file=ca.crt=/tmp/server-ca.crt \
  -n openchoreo-workflow-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-wp apply -f -
```

### Container Registry

```bash
helm repo add twuni https://twuni.github.io/docker-registry.helm
helm repo update

helm install registry twuni/docker-registry \
  --kube-context k3d-openchoreo-wp \
  --namespace openchoreo-workflow-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-registry.yaml
```

### Install Workflow Plane

```bash
helm upgrade --install openchoreo-workflow-plane install/helm/openchoreo-workflow-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-wp \
  --namespace openchoreo-workflow-plane \
  --values install/k3d/multi-cluster/values-wp.yaml
```

### Workflow Templates

The build pipeline is composed of shared ClusterWorkflowTemplates that each handle one step (checkout, build, publish, generate workload). The checkout and publish templates are applied separately so you can replace them to use your own git auth or container registry.

```bash
kubectl --context k3d-openchoreo-wp apply -f samples/getting-started/workflow-templates/checkout-source.yaml
kubectl --context k3d-openchoreo-wp apply -f samples/getting-started/workflow-templates.yaml
kubectl --context k3d-openchoreo-wp apply -f samples/getting-started/workflow-templates/publish-image-k3d.yaml
kubectl --context k3d-openchoreo-wp apply -f samples/getting-started/workflow-templates/generate-workload-k3d.yaml
```

`publish-image-k3d.yaml` pushes images to the local k3d registry at `host.k3d.internal:10082`. To use a different registry, replace this with your own `publish-image` ClusterWorkflowTemplate.

### Buildpack Cache (Optional)

Pre-populates the local registry with buildpack images so Ballerina and Google Cloud Buildpacks workflows don't pull from remote registries on every build. Skip this if you only use Docker or React builds.

```bash
kubectl --context k3d-openchoreo-wp apply -f install/k3d/common/push-buildpack-cache-images.yaml
```

### Register Workflow Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-wp get secret cluster-agent-tls \
  -n openchoreo-workflow-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterWorkflowPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
  secretStoreRef:
    name: default
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

# kgateway
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --create-namespace --namespace openchoreo-observability-plane --kube-context k3d-openchoreo-op \
  --version v2.2.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --namespace openchoreo-observability-plane --create-namespace --kube-context k3d-openchoreo-op \
  --version v2.2.1 \
  --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true

# cert-manager
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-op \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.4 \
  --set crds.enabled=true

kubectl --context k3d-openchoreo-op wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# External Secrets Operator
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-op \
  --namespace external-secrets \
  --create-namespace \
  --version 2.0.1 \
  --set installCRDs=true

kubectl --context k3d-openchoreo-op wait --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s

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

### OpenBao (Secret Store)

Install [OpenBao](https://openbao.org/) as the secret backend on the observability plane cluster:

```bash
install/prerequisites/openbao/setup.sh --dev --seed-dev-secrets --kube-context k3d-openchoreo-op
```

This installs OpenBao in dev mode into the `openbao` namespace, configures Kubernetes auth, seeds placeholder development secrets, and creates a `ClusterSecretStore` named `default`.

To use a different secret backend, skip this and create your own `ClusterSecretStore` named `default` following the [ESO provider docs](https://external-secrets.io/latest/provider/).

### Observability Plane Secrets

```bash
kubectl --context k3d-openchoreo-op apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-secret
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-secret
  data:
  - secretKey: OPENSEARCH_USERNAME
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: OPENSEARCH_PASSWORD
    remoteRef:
      key: opensearch-password
      property: value
  - secretKey: UID_RESOLVER_OAUTH_CLIENT_SECRET
    remoteRef:
      key: observer-oauth-client-secret
      property: value
EOF

kubectl --context k3d-openchoreo-op wait -n openchoreo-observability-plane \
  --for=condition=Ready externalsecret/observer-secret --timeout=60s
```

### Install Observability Plane

```bash
helm upgrade --install openchoreo-observability-plane install/helm/openchoreo-observability-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-op \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-op.yaml
```

```bash
kubectl --context k3d-openchoreo-op wait -n openchoreo-observability-plane \
  --for=condition=available --timeout=600s deployment --all
```

#### Install Observability Modules

Install the logs, metrics, and tracing community modules. The steps below use OpenSearch for logs/tracing and Prometheus for metrics. To use alternative modules visit https://openchoreo.dev/modules.

##### Pre-requisites

Create the `opensearch-admin-credentials` secret needed by the logs and tracing modules:

```bash
kubectl --context k3d-openchoreo-op apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: opensearch-admin-credentials
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: opensearch-admin-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: password
    remoteRef:
      key: opensearch-password
      property: value
EOF
```

Create the `openchoreo-observability-plane` namespace and `opensearch-admin-credentials` secret in the data plane cluster:

```bash
kubectl --context k3d-openchoreo-dp create namespace openchoreo-observability-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-dp apply -f -

kubectl --context k3d-openchoreo-dp apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: opensearch-admin-credentials
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: opensearch-admin-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: password
    remoteRef:
      key: opensearch-password
      property: value
EOF
```

If the workflow plane is installed, create the namespace and secret there too:

> **Note:** The workflow plane does not have a `ClusterSecretStore`, so the secret is created directly.

```bash
kubectl --context k3d-openchoreo-wp create namespace openchoreo-observability-plane \
  --dry-run=client -o yaml | kubectl --context k3d-openchoreo-wp apply -f -

kubectl --context k3d-openchoreo-wp create secret generic opensearch-admin-credentials \
  -n openchoreo-observability-plane \
  --from-literal=username=admin \
  --from-literal=password=ThisIsTheOpenSearchPassword1
```

##### Logs (observability-logs-opensearch)

Install the OpenSearch operator:

```bash
helm repo add opensearch-operator https://opensearch-project.github.io/opensearch-k8s-operator/
helm repo update
helm upgrade --install opensearch-operator opensearch-operator/opensearch-operator \
  --kube-context k3d-openchoreo-op \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 2.8.0 \
  --set kubeRbacProxy.image.repository=quay.io/brancz/kube-rbac-proxy \
  --set kubeRbacProxy.image.tag=v0.15.0
```

Install the OpenSearch logs module

```bash
helm upgrade --install observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --kube-context k3d-openchoreo-op \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.0 \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
  --set openSearchCluster.credentialsSecretName="opensearch-admin-credentials" \
  --set adapter.openSearchSecretName="opensearch-admin-credentials" \
  --set openSearch.enabled=false \
  --set openSearchCluster.enabled=true
```

Enable Fluent Bit in the data plane cluster:

```bash
helm upgrade --install observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --kube-context k3d-openchoreo-dp \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.0 \
  --set openSearch.enabled=false \
  --set openSearchCluster.enabled=false \
  --set openSearchSetup.enabled=false \
  --set adapter.enabled=false \
  --set fluent-bit.enabled=true \
  --set fluent-bit.openSearchHost=host.k3d.internal \
  --set fluent-bit.openSearchPort=11085 \
  --set fluent-bit.openSearchVHost=opensearch.observability.openchoreo.localhost
```

If the workflow plane is installed, enable Fluent Bit there too:

```bash
helm upgrade --install observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --kube-context k3d-openchoreo-wp \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.0 \
  --set openSearch.enabled=false \
  --set openSearchCluster.enabled=false \
  --set openSearchSetup.enabled=false \
  --set adapter.enabled=false \
  --set fluent-bit.enabled=true \
  --set fluent-bit.openSearchHost=host.k3d.internal \
  --set fluent-bit.openSearchPort=11085 \
  --set fluent-bit.openSearchVHost=opensearch.observability.openchoreo.localhost
```

##### Tracing (observability-tracing-opensearch)

Install the tracing receiver in the observability plane cluster. Since the logs module already installed OpenSearch, disable it here:

```bash
helm upgrade --install observability-tracing-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
  --kube-context k3d-openchoreo-op \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.1 \
  --set global.installationMode="multiClusterReceiver" \
  --set openSearch.enabled=false \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
  --set-json opentelemetryCollectorCustomizations.http.hostnames='["opentelemetry.observability.openchoreo.localhost", "host.k3d.internal"]'
```

Install the tracing exporter in the data plane cluster:

```bash
helm upgrade --install observability-tracing-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
  --kube-context k3d-openchoreo-dp \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.1 \
  --set global.installationMode="multiClusterExporter" \
  --set openSearch.enabled=false \
  --set openSearchCluster.enabled=false \
  --set openSearchSetup.enabled=false \
  --set-json opentelemetry-collector.extraEnvs='[]' \
  --set opentelemetryCollectorCustomizations.http.observabilityPlaneUrl="http://host.k3d.internal:11080" \
  --set opentelemetryCollectorCustomizations.http.observabilityPlaneVirtualHost="opentelemetry.observability.openchoreo.localhost"
```

##### Metrics (observability-metrics-prometheus)

Install the metrics receiver in the observability plane cluster:

```bash
helm upgrade --install observability-metrics-prometheus \
  oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
  --kube-context k3d-openchoreo-op \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.2 \
  --set global.installationMode="multiClusterReceiver" \
  --set-json 'prometheusCustomizations.http.hostnames=["prometheus.observability.openchoreo.localhost", "host.k3d.internal"]'
```

Install the metrics exporter in the data plane cluster:

```bash
helm upgrade --install observability-metrics-prometheus \
  oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
  --kube-context k3d-openchoreo-dp \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.4.2 \
  --set global.installationMode="multiClusterExporter" \
  --set prometheusCustomizations.http.observabilityPlaneUrl=http://host.k3d.internal:11080/api/v1/write \
  --set kube-prometheus-stack.prometheus.enabled=false \
  --set kube-prometheus-stack.alertmanager.enabled=false
```

### Register Observability Plane

```bash
AGENT_CA=$(kubectl --context k3d-openchoreo-op get secret cluster-agent-tls \
  -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl --context k3d-openchoreo-cp apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterObservabilityPlane
metadata:
  name: default
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
kubectl --context k3d-openchoreo-cp patch clusterdataplane default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'

# If cluster workflow plane is installed:
kubectl --context k3d-openchoreo-cp patch clusterworkflowplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'
```

## Port Mappings

| Plane               | Cluster           | Kube API | Port Range |
| ------------------- | ----------------- | -------- | ---------- |
| Control Plane       | k3d-openchoreo-cp | 6550     | 8xxx       |
| Data Plane          | k3d-openchoreo-dp | 6551     | 19xxx      |
| Workflow Plane      | k3d-openchoreo-wp | 6552     | 10xxx      |
| Observability Plane | k3d-openchoreo-op | 6553     | 11xxx      |

All ports are mapped 1:1 (host:container) unless noted.

| Port  | Plane         | Service                 |
| ----- | ------------- | ----------------------- |
| 8080  | Control       | Gateway HTTP            |
| 8443  | Control       | Gateway HTTPS           |
| 19080 | Data          | Gateway HTTP            |
| 19443 | Data          | Gateway HTTPS           |
| 10081 | Build         | Argo Workflows UI       |
| 10082 | Build         | Container Registry      |
| 11080 | Observability | Observer API (HTTP)     |
| 11081 | Observability | OpenSearch Dashboards\* |
| 11082 | Observability | OpenSearch API\*        |
| 11084 | Observability | Prometheus\*            |
| 11086 | Observability | OpenTelemetry\*         |

\*Not 1:1 mappings (11081:5601, 11082:9200, 11084:9091, 11086:4317).

## Access Services

| Service            | URL                                        |
| ------------------ | ------------------------------------------ |
| OpenChoreo Console | http://openchoreo.localhost:8080           |
| OpenChoreo API     | http://api.openchoreo.localhost:8080       |
| Thunder Admin      | http://thunder.openchoreo.localhost:8080   |
| Argo Workflows UI  | http://localhost:10081                     |
| Observer API       | http://observer.openchoreo.localhost:11080 |

## Verification

```bash
# All pods
kubectl --context k3d-openchoreo-cp get pods -n openchoreo-control-plane
kubectl --context k3d-openchoreo-dp get pods -n openchoreo-data-plane
kubectl --context k3d-openchoreo-wp get pods -n openchoreo-workflow-plane
kubectl --context k3d-openchoreo-op get pods -n openchoreo-observability-plane

# Plane resources
kubectl --context k3d-openchoreo-cp get clusterdataplane,clusterworkflowplane,clusterobservabilityplane

# Agent connections
kubectl --context k3d-openchoreo-dp logs -n openchoreo-data-plane -l app.kubernetes.io/component=cluster-agent --tail=5
kubectl --context k3d-openchoreo-wp logs -n openchoreo-workflow-plane -l app.kubernetes.io/component=cluster-agent --tail=5
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

# Workflow Plane
install/k3d/preload-images.sh \
  --cluster openchoreo-wp \
  --local-charts \
  --workflow-plane --wp-values install/k3d/multi-cluster/values-wp.yaml \
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
k3d cluster delete openchoreo-cp openchoreo-dp openchoreo-wp openchoreo-op
```
