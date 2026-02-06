# OpenChoreo Installation

This guide walks you through the installation and setup of OpenChoreo on a multi-cluster Kubernetes environment. 
The process involves creating and configuring two Kubernetes clusters: one for the **Control Plane** and another for the **Data Plane**. 
After configuring the clusters, you will install OpenChoreo using Helm, verify the installation, and install the `occ` CLI tool to manage OpenChoreo components.

By the end of this guide, you'll have a fully functional OpenChoreo deployment running on a multi-cluster setup.


## Create Compatible Kubernetes Clusters

You need two Kubernetes clusters: one for the Control Plane and one for the Data Plane. The Data Plane cluster should have Cilium installed to be compatible with OpenChoreo.

If you don't have compatible Kubernetes clusters yet, you can create them using k3d on your local machine.

### k3d Setup

In this section, you'll learn how to set up two k3d clusters and install Cilium in the Data Plane cluster to make it compatible with OpenChoreo.

#### Prerequisites

1. Make sure you have installed [k3d](https://k3d.io/), version v5.8+.
   To verify the installation:
    ```shell
    k3d version
    ```

2. Make sure you have installed [Helm](https://helm.sh/docs/intro/install/), version v3.15+.
   To verify the installation:

    ```shell
    helm version
    ```

3. Make sure you have installed [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl), version v1.32.0.
   To verify the installation:

    ```shell
    kubectl version --client
    ```

4. Make sure you have [Docker](https://docs.docker.com/engine/install/) installed and running.

#### Create k3d Clusters

Create the **Control Plane** cluster:

```shell
k3d cluster create --config install/k3d/multi-cluster/config-cp.yaml
```

Create the **Data Plane** cluster:

```shell
k3d cluster create --config install/k3d/multi-cluster/config-dp.yaml
```

> [!TIP]
> If you're using Colima, set the `K3D_FIX_DNS=0` environment variable before creating clusters:
> ```shell
> K3D_FIX_DNS=0 k3d cluster create --config install/k3d/multi-cluster/config-cp.yaml
> ```
> See [k3d-io/k3d#1449](https://github.com/k3d-io/k3d/issues/1449) for more details.

#### Install Cilium

Cilium must be installed on the Data Plane cluster to work with OpenChoreo. To do so, use the Helm chart provided with the minimal Cilium configuration.

Run the following command to install Cilium in the **Data Plane cluster**:
```shell
helm install cilium oci://ghcr.io/openchoreo/helm-charts/cilium \
  --kube-context k3d-openchoreo-dp \
  --namespace "openchoreo-system" \
  --create-namespace \
  --timeout 30m
```


## Install Prerequisites

### Gateway API CRDs

OpenChoreo uses the Kubernetes Gateway API for traffic management. Install the experimental Gateway API CRDs on each cluster:

```shell
# Control Plane cluster
kubectl apply --context k3d-openchoreo-cp --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml

# Data Plane cluster
kubectl apply --context k3d-openchoreo-dp --server-side \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml
```

### cert-manager

cert-manager is required for TLS certificate management. Install it on each cluster that needs it:

```shell
# Control Plane cluster
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-cp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl wait --context k3d-openchoreo-cp \
  --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s

# Data Plane cluster
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --kube-context k3d-openchoreo-dp \
  --namespace cert-manager \
  --create-namespace \
  --version v1.19.2 \
  --set crds.enabled=true

kubectl wait --context k3d-openchoreo-dp \
  --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=180s
```

### External Secrets Operator

External Secrets Operator (ESO) is required for syncing secrets from external secret stores. Install it on each cluster that needs secret management:

```shell
# Control Plane cluster
helm install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-cp \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true

kubectl wait --context k3d-openchoreo-cp \
  --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s

# Data Plane cluster
helm install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
  --kube-context k3d-openchoreo-dp \
  --namespace external-secrets \
  --create-namespace \
  --version 1.3.2 \
  --set installCRDs=true

kubectl wait --context k3d-openchoreo-dp \
  --for=condition=Available deployment/external-secrets \
  -n external-secrets --timeout=180s
```

## Install OpenChoreo

Now you can proceed to install OpenChoreo on both the Control Plane and Data Plane clusters using Helm.

### 1. Install OpenChoreo Control Plane

Install the Control Plane using Helm:

```shell
helm install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-cp \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-cp.yaml
```

### 2. Install OpenChoreo Data Plane

Install the Data Plane using Helm:

```shell
helm install openchoreo-data-plane install/helm/openchoreo-data-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-dp \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-dp.yaml
```

### 3. Install OpenChoreo Build Plane (Optional)

Install the Build Plane using Helm:

```shell
helm install openchoreo-build-plane install/helm/openchoreo-build-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-bp \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-bp.yaml
```

> [!TIP]
> To install the Build Plane without Argo Workflows, append the following flag: `--set argo-workflows.enabled=false`.

### 4. Verify the Installation

Once OpenChoreo is installed, verify that all components are running:

```bash
# Control Plane
kubectl --context k3d-openchoreo-cp get pods -n openchoreo-control-plane

# Data Plane
kubectl --context k3d-openchoreo-dp get pods -n openchoreo-data-plane

# Build Plane (if installed)
kubectl --context k3d-openchoreo-bp get pods -n openchoreo-build-plane
```

Once you are done with the installation, you can try out our [samples](../samples) to get a better understanding of OpenChoreo.

## Add Data Plane Resource

OpenChoreo requires a DataPlane resource to deploy and manage workloads. You can add the Data Plane resource by running the script provided in the repository.

Run the following command:

```shell
./install/add-data-plane.sh \
  --control-plane-context k3d-openchoreo-cp \
  --target-context k3d-openchoreo-dp \
  --server https://host.k3d.internal:6551
```

> [!NOTE]
> If you're using clusters created with k3d, the above command should work automatically. If you're using different cluster names or contexts, adjust the `--control-plane-context` and `--target-context` flags accordingly.

## Install the occ

[//]: # (TODO: Refine this once we properly release the CLI as a binary.)

`occ` is the command-line interface for OpenChoreo. With that, you can seamlessly interact with OpenChoreo and manage your resources.

### Prerequisites

1. Make sure you have installed [Go](https://golang.org/doc/install), version 1.23.5.
2. Make sure to clone the repository into your local machine.
   ```shell
   git clone https://github.com/openchoreo/openchoreo.git
   ```


### Step 1 - Build `occ`
From the root level of the repo, run:

```shell
make occ-release
```

Once this is completed, it will have a `dist` directory created in the project root directory.

### Step 2 - Install `occ` into your host machine

Run the following command to install the `occ` CLI into your host machine.

```shell
./install/occ-install.sh
````

To verify the installation, run:

```shell
occ
```

You should see the following output:

```text
Welcome to Choreo CLI, the command-line interface for Open Source Internal Developer Platform

Usage:
  occ [command]

Available Commands:
  apply       Apply Choreo resource configurations
  completion  Generate the autocompletion script for the specified shell
  config      Manage Choreo configuration contexts
  create      Create Choreo resources
  get         Get Choreo resources
  help        Help about any command
  logs        Get Choreo resource logs

Flags:
  -h, --help   help for occ

Use "occ [command] --help" for more information about a command.
```

Now `occ` is all setup.

### Uninstall occ

If you want to uninstall `occ` from your host machine, you can use the [script](../install/occ-uninstall.sh) that we have provided.

Run the following command to uninstall `occ`:

```shell
curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/install/occ-uninstall.sh | bash
```

## Access Your Deployed Services

With k3d, services from the Data Plane are automatically accessible via localhost port mappings. The port mappings are configured in the k3d cluster configuration:

- **Port 19080**: HTTP access to workloads (kgateway)
- **Port 19443**: HTTPS access to workloads

You can access your deployed services using:

```shell
# Access a service deployed on the Data Plane
curl http://localhost:19080/<service-path>
```

> [!NOTE]
> The exact port mapping depends on your k3d cluster configuration in `install/k3d/multi-cluster/config-dp.yaml`. Check the configuration file to verify the mapped ports.

## Optional: Observe logs in the OpenChoreo setup

Once you have setup OpenChoreo, you could use the observability logs feature in OpenChoreo to explore the setup. 

You can follow the [Observability Logs guide](observability-logging.md) for this purpose.
