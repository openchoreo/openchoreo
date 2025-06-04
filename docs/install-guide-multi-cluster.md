# OpenChoreo Installation

This guide walks you through the installation and setup of OpenChoreo on a multi-cluster Kubernetes environment. 
The process involves creating and configuring two Kubernetes clusters: one for the **Control Plane** and another for the **Data Plane**. 
After configuring the clusters, you will install OpenChoreo using Helm, verify the installation, and install the `choreoctl` CLI tool to manage OpenChoreo components.

By the end of this guide, you'll have a fully functional OpenChoreo deployment running on a multi-cluster setup.


## Create Compatible Kubernetes Clusters

You need two Kubernetes clusters: one for the Control Plane and one for the Data Plane. The Data Plane cluster should have Cilium installed to be compatible with OpenChoreo.

If you don’t have compatible Kubernetes clusters yet, you can create them using the following guide on your local machine.

### Kind

In this section, you'll learn how to set up two [kind](https://kind.sigs.k8s.io/) clusters and install Cilium in the Data Plane cluster to make it compatible with OpenChoreo.

#### Prerequisites

1. Make sure you have installed [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), version v0.27.0+.
   To verify the installation:
    ```shell
    kind version
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

#### Create Kind Clusters

Create your Kind cluster for the **Control Plane** using ([kind config](../install/kind/multi-cluster-setup/kind-config-cp.yaml)):

```shell
curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/kind/multi-cluster-setup/kind-config-cp.yaml | kind create cluster --config=-
```

Next, create your Kind cluster for the **Data Plane** using ([kind config](../install/kind/multi-cluster-setup/kind-config-dp.yaml)):

```shell
curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/kind/multi-cluster-setup/kind-config-dp.yaml | kind create cluster --config=-
```

> [!NOTE]
> This setup assigns a specific label to one node in the Data Plane cluster to schedule Argo Workflows.
> If you're creating Data Plane clusters manually, ensure that only one node is labeled with:
> `core.choreo.dev/noderole: workflow-runner`.
> Additionally, create a DataPlane kind named `default-dataplane` pointing to this labeled cluster, which will serve 
> as the default target cluster for Argo Workflows until support is added for running them on any Data Plane.

#### Install Cilium

Cilium must be installed on the Data Plane cluster to work with OpenChoreo. To do so, use the Helm chart provided with the minimal Cilium configuration.

Run the following command to install Cilium in the **DataPlane cluster**:
```shell
helm install cilium oci://ghcr.io/openchoreo/helm-charts/cilium --kube-context kind-choreo-dp --namespace "choreo-system" --create-namespace --timeout 30m
```

[//]: # (Todo: Test this properly on k3d and include the steps in the following section.)

[//]: # (### k3d)

[//]: # ()
[//]: # (#### steps for creating the kind cluster)

[//]: # ()
[//]: # (#### Exposing the OpenChoreo Gateway)


## Install OpenChoreo

Now you can proceed to install OpenChoreo on both the Control Plane and Data Plane clusters using Helm.

1. Install OpenChoreo Control Plane

Install the Control Plane using Helm:

```shell
helm install choreo-control-plane oci://ghcr.io/openchoreo/helm-charts/choreo-control-plane \
--kube-context kind-choreo-cp --namespace "choreo-system" --create-namespace --timeout 30m --version 0.0.0-latest-dev
```

 2. Install OpenChoreo DataPlane

Install the Data Plane using Helm:

```shell
helm install choreo-dataplane oci://ghcr.io/openchoreo/helm-charts/choreo-dataplane \
--kube-context kind-choreo-dp --namespace "choreo-system" --create-namespace --timeout 30m --version 0.0.0-latest-dev
```

> [!TIP]
> To install the DataPlane without Argo Workflows, append the following flag: `--set argo-workflows.enabled=false`.

3. Verify the Installation

Once OpenChoreo is installed, you can verify the installation status using the provided script ([script](../install/check-status.sh)).

- I. Run the verification script:

```shell
bash <(curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/check-status.sh) --multi-cluster 
```

- II. Follow the prompts:
  - 'Enter DataPlane kubernetes context (default: kind-choreo-dp):' - Press `Enter` if you are using the cluster created earlier, or provide your context.
  - 'Enter Control Plane kubernetes context (default: kind-choreo-cp):' - Press `Enter` if you are using the cluster created earlier, or provide your context.

The script will display the current status of OpenChoreo components across both clusters.

Once you are done with the installation, you can try out our [samples](../samples) to get a better understanding of OpenChoreo.

## Add Default DataPlane

OpenChoreo requires a DataPlane to deploy and manage its resources. You can add the default DataPlane by running the script provided in the repository ([script](../install/add-default-dataplane.sh)).

Run the following command:

```shell
bash <(curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/add-default-dataplane.sh) --multi-cluster 
```

- Follow the prompts:
  -  'Enter DataPlane kubernetes context (default: kind-choreo-dp):' - Press `Enter` to use the default Kubernetes context for the DataPlane cluster created earlier.
  -  'Enter DataPlane kind name (default: default-dataplane):' - Press `Enter` to proceed.

> [!NOTE]
> If you're using a cluster that was not created with Kind, you'll need to manually gather the API server 
> credentials and create the DataPlane kind yourself.

## Install the choreoctl

[//]: # (TODO: Refine this once we properly release the CLI as a binary.)

`choreoctl` is the command-line interface for OpenChoreo. With that, you can seamlessly interact with OpenChoreo and manage your resources.

### Prerequisites

1. Make sure you have installed [Go](https://golang.org/doc/install), version 1.23.5.
2. Make sure to clone the repository into your local machine.
   ```shell
   git clone https://github.com/openchoreo/openchoreo.git
   ```


### Step 1 - Build `choreoctl`
From the root level of the repo, run:

```shell
make choreoctl-release
```

Once this is completed, it will have a `dist` directory created in the project root directory.

### Step 2 - Install `choreoctl` into your host machine

Run the following command to install the `choreoctl` CLI into your host machine.

```shell
./install/choreoctl-install.sh
````

To verify the installation, run:

```shell
choreoctl
```

You should see the following output:

```text
Welcome to Choreo CLI, the command-line interface for Open Source Internal Developer Platform

Usage:
  choreoctl [command]

Available Commands:
  apply       Apply Choreo resource configurations
  completion  Generate the autocompletion script for the specified shell
  config      Manage Choreo configuration contexts
  create      Create Choreo resources
  get         Get Choreo resources
  help        Help about any command
  logs        Get Choreo resource logs

Flags:
  -h, --help   help for choreoctl

Use "choreoctl [command] --help" for more information about a command.
```

Now `choreoctl` is all setup.

### Uninstall choreoctl

If you want to uninstall `choreoctl` from your host machine, you can use the [script](../install/choreoctl-uninstall.sh) that we have provided.

Run the following command to uninstall `choreoctl`:

```shell
curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/install/choreoctl-uninstall.sh | bash
```

## Expose the OpenChoreo Gateway

To enable end-to-end access to the OpenChoreo components you deploy, you need to expose the external gateway service of the Data Plane cluster to your host machine. This allows you to interact with your deployed services seamlessly from outside the cluster.

> [!NOTE]
> To expose deployments within this cluster, we route traffic through the external gateway service which is in the DatPlane.

### Kind

In this section, we will guide you on how to expose the OpenChoreo external gateway service to your host machine in a [kind](https://kind.sigs.k8s.io/) cluster.

Once you successfully [installed OpenChoreo](#install-openchoreo) into your cluster, you will see a LoadBalancer service created for our external gateway.

You can see the service using the following command.

```shell
kubectl --context=kind-choreo-dp get svc choreo-external-gateway -n choreo-system
```

You will see an output similar to the following:

```text
NAME                      TYPE           CLUSTER-IP     EXTERNAL-IP   PORT(S)         AGE
choreo-external-gateway   LoadBalancer   10.96.75.106   <pending>     443:30807/TCP   55m
```

You have two options to expose the DataPlane choreo-external-gateway service to your host machine.

1. Option 1: Use [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind/tree/main) to expose the service.
2. Option 2: port-forward from your host machine to choreo-external-gateway service.

##### Option 1: Use cloud-provider-kind to expose the service.

The following steps will guide you through using the [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind/tree/main) tool for exposing the external-gateway service.

First, [install](https://github.com/kubernetes-sigs/cloud-provider-kind/tree/main?tab=readme-ov-file#install) the cloud-provider-kind tool to your host machine.

Then, run this tool in sudo mode, and it will automatically assign LoadBalancer IP to your choreo-external-gateway service.

```shell
# run this command in a separate terminal and keep it running.
sudo $(which cloud-provider-kind)
```

Then you could find the load balancer IP for your external-gateway service as follows.

```shell
kubectl --context=kind-choreo-dp get svc -n choreo-system | grep choreo-external-gateway
```

```shell
# to find the LoadBalancer-IP
# <name> should be replaced with the service name found in the previous step.
$ kubectl --context=kind-choreo-dp get svc/<name> -n choreo-system -o=jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Then you can use this IP address to access the components you create in OpenChoreo via the external gateway.

##### Option 2: Port-forward the external-gateway service

Run the following command to do port-forwarding from your host machine to the `choreo-external-gateway` service.

```shell
kubectl --context=kind-choreo-dp port-forward svc/choreo-external-gateway -n choreo-system 443:443
```

> [!TIP]
> If you have an existing service listening on port 443, or any permission issues, you may encounter issues when attempting port forwarding. To avoid conflicts, consider changing the port as needed.
> Ex: `kubectl port-forward svc/choreo-external-gateway -n choreo-system 8443:443`

> [!NOTE]
> You may need to add entries to `/etc/hosts` to access components through the external gateway, as it relies on the hostname for request routing.
> For example, if your endpoint URL is `https://default-org-default-project-hello-world-ea384b50-development.choreo.localhost`, and your load balancer IP is `172.19.0.4` you need to add the following entry to your /etc/hosts file.
> `172.19.0.4 default-org-default-project-hello-world-ea384b50-development.choreoapps.localhost`
