## Quick Start Guide

Setting up Open Source Choreo in a Kubernetes cluster involves multiple steps and tools. This guide provides a fast and simple way to install a fully functional Open Source Choreo instance on your local machine with minimal prerequisites and effort by using a pre-configured dev container.
This dev container has all the necessary tools installed for setting up Open Source Choreo and is ready to be used. Once the installation is complete, you can explore the underlying setup to understand how it works.
When you're done, you can fully clean up the setup, leaving your machine clutter-free.

#### Prerequisites
The only requirement is Docker — just have it installed on your machine, and you're good to go. We recommend using [docker engine version 26.0+](https://docs.docker.com/engine/release-notes/26.0/).

#### Start the Dev Container

Run the following command to start the dev container and launch a terminal session within it:

```shell
docker run --rm -it --name choreo-quick-idp \
-v /var/run/docker.sock:/var/run/docker.sock \
-v choreo-state:/state \
-v tf-state:/app/terraform \
--network bridge \
-p 8443:8443 \
ghcr.io/choreo-idp/quick-start:v0.1.0

```

#### Install Open Source Choreo
This process sets up a [KinD](https://kind.sigs.k8s.io/) (Kubernetes-in-Docker) cluster in your Docker environment and installs Open Source Choreo along with its dependencies.

To begin the installation, run:

```shell
./install.sh
```

Once the installation is complete, you will see the following confirmation message:
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
🎉 Choreo has been successfully installed and is ready to use!
```

#### Deploying a Web Application with Open Source Choreo

You now have Open Source Choreo fully setup in your docker environment.
Next, lets deploy a sample Web Application by running the following command:

```shell
./deploy_web_application.sh
```

Once the deployment is complete, you will receive the following message together with a URL to access the application:

```text
✅ Endpoint is ready!
🌍 You can now access the Sample Web Application at: https://react-starter-image-development.choreo.localhost:8443
```

### Understanding What Happens Behind the Scenes
By following the install and deploy web application commands, you first, setup Open Source Choreo and then, successfully deployed and accessed a fully functional Web Application.

Let’s now explore what happens after each command.

#### 1. The Install Command
- A dev container with all the necessary tools for Open Source Choreo to run is set up in a local Docker environment.
- A KinD Kubernetes cluster is created, where the Open Source Choreo IDP and its dependencies were installed using Helm charts.

#### Foundation Resources Created by Open Source Choreo

The installation process, by default, sets up several essential abstractions. These are:
- Organization
- [Dataplane](https://github.com/choreo-idp/choreo/tree/main/docs#dataplane)
- [Environments](https://github.com/choreo-idp/choreo/tree/main/docs#environment) (e.g., Development, Staging, Production)
- [Deployment Pipeline](https://github.com/choreo-idp/choreo/tree/main/docs#deploymentpipeline) for the environments
- [Project](https://github.com/choreo-idp/choreo/tree/main/docs#project)

To access the artifacts created in Open Source Choreo you can use choreoctl as shown in the following commands:

First you can get the current context
```shell
choreoctl config current-context
```
Next you can check each resource type

```shell
choreoctl get organizations
```
```shell
choreoctl get dataplanes
```
```shell
choreoctl get environments
```
```shell
choreoctl get projects
```
To get more details on any of these abstractions, you can use a similar command to the following command:

```shell
choreoctl get project default-project -oyaml
```

#### 2. The Deploy Web Application Command
The deploy script creates a sample Web Application [Component](https://github.com/choreo-idp/choreo/tree/main/docs#component), along with a [Deployment](https://github.com/choreo-idp/choreo/tree/main/docs#component) for the sample web application.

To inspect these resources in more detail, run the following commands:

```shell
choreoctl get components
```
```shell
choreoctl get deployment --component react-starter-image
```

Open Source Choreo generates a [DeployableArtifact](https://github.com/choreo-idp/choreo/tree/main/docs#deployableartifact) and an [Endpoint](https://github.com/choreo-idp/choreo/tree/main/docs#endpoint) to access the running application:

```shell
choreoctl get deployableartifact --component react-starter-image
```
```shell
choreoctl get endpoint --component react-starter-image
```

You can also check out the logs for the sample Web Application Deployment with the following command:
```shell
choreoctl logs --type deployment --component react-starter-image --deployment react-starter-image-deployment --follow
```

### Cleaning up
To remove all resources and clean up the environment, run:

```shell
./uninstall.sh
```

Then exit the dev container by running:

```shell
exit
```

To clean up your Docker environment, run:

```shell
docker volume rm choreo-state tf-state
```

That's it!

Now you understand how Open Source Choreo simplifies the deployment and management of cloud-native applications.
