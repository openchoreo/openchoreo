# Choreo
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![GitHub last commit](https://img.shields.io/github/last-commit/ballerina-platform/ballerina-lang.svg)](https://github.com/ballerina-platform/ballerina-lang/commits/master)

<img src="./logo/choreo-logo-black.svg" width="100">

## What is Choreo?
Choreo is an internal developer platform that simplifies the build, deployment, and management of applications. It provides a unified interface for developers to create, test, and deploy cloud-native applications with ease. Choreo offers features such as automated CI/CD pipelines, integrated API management, and seamless integration with various cloud services, enabling developers to focus on writing code while the platform handles the operational complexities.

## Table of Contents
- [Introduction](#introduction)
  - [Choreo Abstractions](#choreo-abstractions)
- [Quick Start Guide](#quick-start-guide)
  - [Prerequisites](#_prerequisites_)
  - [Generic guide to Install Choreo using Helm Chart](#generic-guide-to-install-choreo-using-helm-chart)
  - [Install from Scratch Using Kind Cluster](#install-from-scratch-using-kind-cluster)
  - [Deploy your first component in choreo](#deploy-your-first-component-in-choreo)
- [Contributor Guide](#contributor-guide)
  - [Prerequisites for Contributors](#prerequisites-for-contributors)
  - [Build and Use Binaries](#build-and-use-binaries)
  - [To Deploy on the Cluster](#to-deploy-on-the-cluster)
  - [To Uninstall](#to-uninstall)
- [Project Distribution](#project-distribution)
  - [Implement Custom Resources](#implement-custom-resources)
- [License](#license)

## Introduction

The Choreo repository is a collection of Kubernetes CRDs that enables application development functionalities. These CRDs enable cloud-native deployments, seamless API management, and integration workflows by defining declarative configurations for Kubernetes.

### Choreo Abstractions

This repository defines Choreo abstractions in the form of Kubernetes CRDs, enabling developers to use these abstractions to create projects, components, builds, deployments, and more. By leveraging these CRDs, developers can declaratively manage their application's lifecycle and infrastructure, ensuring consistency and repeatability across environments.

- **DataPlane**: Represents a Data Plane in Choreo, responsible for maintaining the health status of the data plane and providing data plane information to other resources.
- **Environment**: Represents an environment bound to a specific data plane in Choreo, with a reference to an existing `DataPlane` resource.
- **DeploymentPipeline**: Represents an ordered set of environments that a deployment will go through to reach a critical environment, with a default deployment pipeline for each organization.
- **Project**: Represents a project in Choreo, enforcing a promotion order for the components within the project, with an optional reference to a deployment pipeline.
- **Component**: Represents a deployable unit in Choreo, managing the entire lifecycle of the component from source to deployment, with various deployment architectures.
- **DeploymentTrack**: Represents a deployment path for a component, managing the deployment of the component across environments and handling auto deployment and build management.
- **Build**: Represents a source code to artifact transformation, managed by the deployment track controller, responsible for configuring build parameters and tracking build artifacts.
- **DeployableArtifact**: Represents a build artifact with environment-independent configurations, ready to be deployed to an environment, created by the build controller or manually by the user.
- **Deployment**: Represents a deployment in an environment bound to a deployment track, managing deployment revisions, deploying artifacts, and monitoring deployment status.
- **DeploymentRevision**: Represents a snapshot of the deployment resource at a given time, created by the deployment controller to track deployment history and restore deployment specs during revert operations.
- **Endpoint**: Represents an endpoint exposed by the component, responsible for updating Kubernetes resources, creating managed APIs, and configuring API settings.
- **Secret**: Represents configuration parameters stored in a key vault, used for storing both system secrets and environment-specific secrets, with various secret types like GitHub, Bitbucket, GitLab, and DockerHub.

These abstractions simplify the development and deployment process, allowing developers to focus on writing code while Choreo handles the underlying infrastructure and operational tasks.

For more details about the abstractions, refer to [Choreo Resource Kinds](docs/README.md).

## Quick Start Guide
This guide will help users set up the necessary prerequisites, download and install the Choreo Helm chart, verify the setup and deploy the sample application.
### _Prerequisites_
- [Helm](https://helm.sh/docs/intro/install/) version v3.15+
  > Choreo use the Helm as the package manager to install the required artifacts into the kubernetes cluster.
- [Cilium](https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/#install-cilium) installed kubernetes cluster
    - Cilium version v1.15.10
    - kubernetes version v1.22.0+
  > Cilium is a dependency for choreo to operate. It uses the Cilium CNI plugin to manage the network policies and security for the pods in the cluster.

### Generic guide to Install Choreo using Helm Chart
You can directly install Choreo using the Helm chart provided in our registry.

```shell
helm install choreo oci://choreov3testacr.azurecr.io/choreo-v3/choreo \
--version 0.1.0 --namespace "choreo-system" --create-namespace --timeout 30m
```

### Install from Scratch Using a kind(k8s in docker) Cluster

This section guides you through setting up a [kind](https://kind.sigs.k8s.io/) cluster and installing Cilium and Choreo from scratch.

#### 1. Install Kind

Make sure you have installed [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), version v0.25.0+.

> We use Kind to quickly create a Kubernetes cluster, primarily for testing purposes.

To verify the installation:

```shell
kind version
```

#### 2. Create a Kind cluster

```shell
kind create cluster --config=install/kind/kind-config.yaml
```

#### 3. Install Cilium

The following helm chart provided by us installs Cilium with minimal configurations required for Choreo.

```shell
helm install cilium oci://choreov3testacr.azurecr.io/choreo-v3/cilium  --version 0.1.0 --namespace "choreo-system" --create-namespace --timeout 30m
```

#### 4. Install Choreo

```shell
helm install choreo oci://choreov3testacr.azurecr.io/choreo-v3/choreo  --version 0.1.0 --namespace "choreo-system" --create-namespace --timeout 30m
```

#### 5. Verify installation status

```shell
sh install/check-status.sh
```

You should see the following output if the installation is successful.

```text
Choreo Installation Status:

Component                 Status         
------------------------  ---------------
cilium                    ✅ ready
vault                     ✅ ready
argo                      ✅ ready
cert_manager              ✅ ready
choreo_controller         ✅ ready
choreo_image_registry     ✅ ready
envoy_gateway             ✅ ready
redis                     ✅ ready
external_gateway          ✅ ready
internal_gateway          ✅ ready

Overall Status: ✅ READY
🎉 Choreo has been successfully installed and is ready to use! 🚀
```

### Deploy your first component in choreo

This section guides you through deploying a sample Web Application and invoking it. Go through the following steps to deploy the
sample Web Application component in Choreo.

#### 1. Create the sample Web Application component

For this, you will be using the samples we provided in the repository.
Apply the sample WebApp component using the following command.

```shell
choreoctl create component --name hello-world --type WebApplication --git-repository-url https://github.com/docker/awesome-compose --branch master --path /react-nginx --buildpack-name React --buildpack-version 18.20.6
```

#### 2. Build the created sample component
Create a build resource for hello-world component using Choreo CLI interactive mode.

```shell
choreoctl create build -i
```
Use the build name as 'build1' and keep other inputs as defaults.
```shell
./choreoctl create build -i                                                           config-context  ✭ ✱
Selected inputs:
- organization: default-org
- project: default-project
- component: hello-world
- deployment track: default
- name: build1
- revision: latest
Enter git revision (optional, press Enter to use latest):
Build 'build1' created successfully in project 'default-project' of organization 'default-org'

```

#### 3. View build logs and status
```shell
choreoctl logs --type build --component hello-world --build build1 --follow
```
> Note: The build step will take around 5 minutes to get all the dependencies and complete the build.

See the build status using get build resource command.
```shell
choreoctl get build --component hello-world  build1
```
> Note: Proceed to the next step after build  is in 'Completed' status.

#### 4. View the generated deployable artifact
As part of the successful build, a deployment artifact resource is created to trigger a deployment.
```shell
choreoctl get deployableartifact --component hello-world
```
#### 5. Create a deployment resource
For this option, we will explore the interactive mode which guide you to create the deployment resource.
```shell
choreoctl create deployment -i
```
Name the deployment as 'dev-deployment'. Following is a sample CLI output.
```shell
choreoctl create deployment -i                                                        config-context  ✭ ✱
Selected resources:
- organization: default-org
- project: default-project
- component: hello-world
- deployment track: default
- environment: development
- deployable artifact: default-org-default-project-hello-world-default-foo-0c5ff1ee
- name: dev-deployment
Enter deployment name:
Deployment 'dev-deployment' created successfully in component 'hello-world' of project 'default-project' in organization 'default-org'
```

#### 6. View the generated endpoint resource
```shell
choreoctl get endpoint --component hello-world
```
You should see a similar output as follows.
``` shell
NAME     ADDRESS                                                                             AGE   ORGANIZATION
webapp   https:/default-org-default-project-hello-world-ea384b50-development.choreo.local   14h   default-org
```
#### 7. Test the deployed WebApp

You have two options to test your WebApp component.

1. Option 1: Access the WebApp by exposing the external-gateway as a LoadBalancer to your host machine.
2. Option 2: port-forward from your host machine to external-gateway service.

##### Option 1: Expose the external-gateway as a LoadBalancer

The following steps will guide you through exposing the external-gateway service as a LoadBalancer to your host machine.
In this you will be using the [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind/tree/main) to
expose the LoadBalancer service(external-gateway) to your host machine.

First, [install](https://github.com/kubernetes-sigs/cloud-provider-kind/tree/main?tab=readme-ov-file#install) the cloud-provider-kind tool to your host machine.

Then, run this tool in sudo mode, and it will automatically assign LoadBalancer IP to your external-gateway service.

```shell
# run this command in a separate terminal and keep it running.
$ sudo $(which cloud-provider-kind)
```

Then you could find the load balancer IP for your external-gateway service as follows.

```shell
# to find the external-gateway service name
$ kubectl get svc -n choreo-system | grep gateway-external
```

```shell
# to find the LoadBalancer-IP
# <name> should be replaced with the service name found in the previous step.
$ kubectl get svc/<name> -n choreo-system -o=jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Then add this IP to your /etc/hosts file as follows.

```text
<LoadBalancer-IP> react-starter-development.choreo.local
```

Now you can access the WebApp using following URL.

https://default-org-default-project-hello-world-ea384b50-development.choreo.local

##### Option 2: Port-forward the external-gateway service

The following steps will guide you through port-forwarding from your host machine to the external-gateway service.

First, find the external-gateway service using the following command.

```shell
kubectl get svc -n choreo-system | grep gateway-external
```

Then port-forward the service to your host machine using the following command.

```shell
# <name> should be replaces with the service name found in the previous step.
kubectl port-forward svc/<name> -n choreo-system 443:443
```

Then add the following entry to your /etc/hosts file.

```
127.0.0.1 default-org-default-project-hello-world-ea384b50-development.choreo.local
```

Now you can access the WebApp using the following URL.

https://default-org-default-project-hello-world-ea384b50-development.choreo.local

#### 8. View deployment logs
```shell
choreoctl logs --type deployment --component hello-world --deployment dev-deployment --follow
```

## Contributor Guide

This section provides a comprehensive guide for contributors to set up their development environment, build and use the binaries, and deploy Choreo on a Kubernetes cluster for testing and development purposes.

### Prerequisites for Contributors
- Go version v1.23.0+
- Docker version 17.03+
- Kubernetes cluster with version v1.30.0+

### Build and Use Binaries

1. Clone the repository:
                ```sh
                git clone https://github.com/<org>/choreo.git
                cd choreo
                ```

2. Build the binaries:
                ```sh
                make build
                ```

3. Run the binaries:
                ```sh
                ./bin/manager
                ```

4. Follow the deployment steps mentioned below under "To Deploy on the cluster" section.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/choreo:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/choreo:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Code Generation and Linting

After updating the Custom Resource Definitions (CRDs) or the controller code, run the following commands to generate necessary code and lint the codebase before committing the changes.

1. Run the linter:
    ```sh
    make lint
    ```
2. Run the code generator:
    ```sh
    make code.gen
    ```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/choreo:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/choreo/<tag or branch>/dist/install.yaml
```
### Implement Custom Resources
**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
