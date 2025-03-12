# Open Source Choreo
A Complete, yet Customizable Internal Developer Platform

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![GitHub last commit](https://img.shields.io/github/last-commit/choreo-idp/choreo.svg)](https://github.com/choreo-idp/choreo/commits/main)
[![Go Report Card](https://goreportcard.com/badge/github.com/choreo-idp/choreo)](https://goreportcard.com/report/github.com/choreo-idp/choreo)
[![GitHub issues](https://img.shields.io/github/issues/choreo-idp/choreo.svg)](https://github.com/choreo-idp/choreo/issues)

## What is Open Source Choreo?
Open Source Choreo is a fully open source Internal Developer Platform (IDP) designed to empower platform engineers by simplifying infrastructure automation, governance, and security. 

With Open Source Choreo, platform engineers can seamlessly setup the foundational components of the organization's IDP effortlessly. With automated CI/CD, security, and cloud tool integrations, Open Source Choreo helps platform teams enforce best practices and streamline development workflows.

At the same time, Open Source Choreo provides developers with a configured platform to build, test, and deploy applications without the complexity of managing infrastructure or foundational enterprise platform artifacts. 

What sets Open Source Choreo apart is its full customizability—organizations have complete control over deployment, configurations, and extensions, enabling them to adapt the platform to their unique needs. By embracing platform engineering principles, Open Source Choreo enables teams to accelerate software delivery, optimize operations, and free up engineering resources—allowing them to focus on innovation rather than infrastructure management.

[//]: # (Architecture Diagram)

##  Open Source Choreo Abstractions 

Open Source Choreo converts typical enterprise abstractions into Kubernetes [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), allowing platform engineers to set up the IDP and developers to manage application artifacts declaratively. This [document](docs/choreo-abstractions.md) outlines the abstractions used in Open Source Choreo
with references on how to define projects, components, builds, and deployments, ensuring consistency, scalability, and repeatability across environments.

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
ghcr.io/choreo-idp/quick-start:latest

```

#### Install Open Source Choreo
This process sets up a [KinD](https://kind.sigs.k8s.io/) (Kubernetes-in-Docker) cluster in your Docker environment and installs Open Source Choreo along with its dependencies.

To begin the installation, run:

```shell
./install.sh
```

Once the installation is complete, you will see the following confirmation message:
```text
>>>> Everything has been setup, ready to deploy application.
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

Let’s now explore what happened after each command. To access the artifacts created in Open Source Choreo we will be using choreoctl.

#### 1. The Install command
- A dev container with all the necessary tools for Open Source Choreo to run in a local Docker environment.
- A KinD Kubernetes cluster, where the Open Source Choreo IDP and its dependencies were installed using Helm charts.

To check all the installed components, run:
```shell
./check-status.sh
```

#### Foundation Resources Created by Open Source Choreo

The installation process, by default, sets up several essential abstractions. These are:
- Organization 
- Dataplane
- Environments (e.g., Development, Staging, Production)
- A Deployment Pipeline for the environments, and a
- Default Project

You can inspect these in more detail using the following commands:

```shell
choreoctl get organizations --organization default-org

choreoctl get dataplanes --organization default-org

choreoctl get environments --organization default-org

choreoctl get projects --organization default-org
```

#### 2. The Deploy Web Application Command
The deploy script creates a sample Web Application component, along with a Deployment for the sample web application.

To inspect these resources in more detail, run the following commands:

```shell
choreoctl get components --organization default-org --project default-project

choreoctl get deployment --organization default-org --project default-project --component react-starter-image --environment development
```

Open Source Choreo generates a Deployable Artifact and an Endpoint to access the running application:

```shell
choreoctl get deployableartifact --organization default-org --project default-project --component react-starter-image

choreoctl get endpoint --organization default-org --project default-project --component react-starter-image --environment development
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

Now you understand how  Open Source Choreo simplifies the deployment and management of cloud-native applications.

## Detailed Setup of Open Source Choreo
To do a deeper dive installation, check out the [Detailed Installation Guide](install/README.md).

## Project roadmap
For information about the detailed project roadmap for Open Source Choreo, including delivered milestones, see the [Roadmap]( https://github.com/orgs/choreo-idp/projects/1).

## Community
To engage with our community, you can join the  Open Source Choreo [Discord](https://discord.gg/HYCgUacN) channel.


## Contributing
Want to help develop Open Source Choreo? Check out our [contributing documentation](docs/contributing.md).
If you find an issue, please report it on the [Github Issue Tracker](https://github.com/choreo-idp/choreo/issues).

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
