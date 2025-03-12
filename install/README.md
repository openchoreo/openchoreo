[//]: # (&#40;Todo&#41;: Refactor this)
# Quick choreo installation

This guide will wals you through the quick default installation of choreo. Choreo uses Kubernetes custom resource definitions (CRDs) to store all its state. 

This is using [helm](https://helm.sh/) to install required artifacts into the kubernetes cluster.

## Creating a k8s cluster

If you don't have a Kubernetes cluster yet, you can use the following instructions to create your kubernetes cluster. Since we are using [cilium](https://cilium.io/) as a dependency for this choreo version, the following instructions include the cluster level changes required for cilium to operate.

### Kind (Kubernetes in docker)

#### Step 1 - Installing Kind

Make sure you have installed kind : https://kind.sigs.k8s.io/docs/user/quick-start/#installation

On macos via Homebrew

```shell
$ brew install kind
```

#### Step 2 - Creating a Kind cluster

```shell
$ kind create cluster --config=kind/kind-config.yaml
```

##### Build and load controller latest image to kind cluster

Use the following command from the root directory of this project to build the controller image.

```shell
IMG="choreo-controller:latest" make docker-build
```

Load the image to the kind cluster

```shell
kind load docker-image choreo-controller:latest --name choreo
```

> Note: If you are using kind for testing, you could speedup the installation process by pre-loading the images required for the installation. You can use the following script to load images to your kind cluster.
> ```shell
> $ ./kind/load-images.sh --kind-cluster-name choreo
> ```


### k3d (k3s in docker)

#### Step 1 - Installing k3d

Install a stable version of k3d : https://k3d.io/stable/#installation

On macos via Homebrew

```shell
$ brew install k3d
```

#### Step 2 - Creating a k3d cluster

```shell
$ k3d cluster create --config k3d/k3d-config.yaml
```

## Installing choreo helm chart

This is installing two helm charts to your cluster to install the followings.

1. cilium
2. Other dependencies & choreo controllers

## Install in one-go

Run the provided shell script to install the required artifacts.

```shell
$ ./install.sh
```

This process may take some time to bring the components into a running state, as it involves downloading images from public repositories.

## Check installation status

To check the installation status, you can run

```shell
$ ./check-status.sh

Installation status:
✅ cilium-agent : ready 
✅ cilium-operator : ready 
✅ vault : ready 
✅ vault-agent-injector : ready 
🕑 argo-workflows-server : pending 
🕑 argo-workflows-workflow-controller : pending 
✅ cert-manager : ready 
✅ cainjector : ready 
🕑 webhook : pending 
🕑 choreo-controllers : pending 
✅ gateway-helm : ready 
```

## Uninstalling choreo

```shell
$ ./uninstall.sh
```

This action will completely remove all installed artifacts associated with the Choreo Helm chart.
