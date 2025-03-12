# Choreo
A Complete, yet Customizable Internal Developer Platform

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![GitHub last commit](https://img.shields.io/github/last-commit/choreo-idp/choreo.svg)](https://github.com/choreo-idp/choreo/commits/main)
[![Go Report Card](https://goreportcard.com/badge/github.com/choreo-idp/choreo)](https://goreportcard.com/report/github.com/choreo-idp/choreo)
[![GitHub issues](https://img.shields.io/github/issues/choreo-idp/choreo.svg)](https://github.com/choreo-idp/choreo/issues)

## What is Open Source Choreo?
Open Source Choreo is an Internal Developer Platform (IDP) designed to empower platform engineers by simplifying infrastructure automation, governance, and security. It provides the foundation for building scalable, self-service developer platforms, reducing operational overhead while ensuring compliance and standardization. With automated CI/CD, security, and cloud integrations, Choreo helps platform teams enforce best practices and streamline development workflows.

At the same time, Choreo provides developers with a unified interface to build, test, and deploy applications without the complexity of managing infrastructure. By embracing platform engineering principles, Choreo enables teams to accelerate software delivery, optimize operations, and free up engineering resources—allowing them to focus on innovation rather than infrastructure management.

[//]: # (Architecture Diagram)

## Choreo Abstractions
This [document](docs/choreo-abstractions.md) outlines Choreo abstractions as Kubernetes [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), allowing developers to manage application lifecycles and infrastructure declaratively. Here, you’ll find details on how to define projects, components, builds, and deployments, ensuring consistency, scalability, and repeatability across environments.

## Quick Start Guide

Setting up Choreo in a Kubernetes cluster involves multiple steps and tools. This tutorial provides a fast and simple way to install a fully functional Choreo instance on your local machine with minimal prerequisites and effort.
This guide uses a pre-configured dev container that comes with all necessary tools installed and ready to use. Once the installation is complete, you can explore the underlying setup to understand how it works.
When you're done, you can fully clean up the setup, leaving your machine clutter-free.

### Prerequisites
The only requirement is Docker — just have it installed on your machine, and you're good to go. We recommend using [docker engine version 26.0+](https://docs.docker.com/engine/release-notes/26.0/).

### Start the Dev Container

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

### Install Choreo
This process sets up a [KinD](https://kind.sigs.k8s.io/) (Kubernetes-in-Docker) cluster in your Docker environment and installs Choreo along with its dependencies.

To begin the installation, run:

```shell
./install.sh
```

Once the installation is complete, you will see the following confirmation message:
```text
>>>> Everything prepared, ready to deploy application.
``` 

### Quick Demo: Deploying a Web Application with Choreo

This quick demo walks you through deploying a sample Web Application on your local Choreo setup.

Run the following command to deploy a demo web application:

```shell
./demo.sh
```

Once the deployment is complete, you will receive a URL to access the application:

```text
✅ Endpoint is ready!
🌍 You can now access the Web app at: https://react-starter-image-development.choreo.localhost:8443
```

### Understanding What Happens Behind the Scenes
Through this demo, you’ve successfully deployed and accessed a fully functional Web Application using Choreo. Now, let’s explore the components behind it using choreoctl.
What Gets Set Up?
- A dev container with all necessary tools to run Choreo IDP in a local Docker environment.
- A KinD Kubernetes cluster, where Choreo IDP and its dependencies are installed via Helm charts.

To check all the installed components, run:
```shell
./check-status.sh
```

#### Key Resources Created by Choreo
The installation process automatically sets up several essential Choreo resources, including:
- Organization
- Dataplane
- Environments (e.g., Development, Staging, Production)
- Deployment Pipeline
- Default Project

You can verify these using the following commands:

```shell
choreoctl get organizations --organization default-org

choreoctl get dataplanes --organization default-org

choreoctl get environments --organization default-org

choreoctl get projects --organization default-org
```

#### Application Deployment Flow
The demo script creates a Web Application component, along with a Deployable Artifact, and a Deployment.

To inspect these resources, run:

```shell
choreoctl get components --organization default-org --project default-project

choreoctl get deployableartifact --organization default-org --project default-project --component react-starter-image

```

Finally, Choreo generates a Deployment and an Endpoint to access the running application:

```shell
choreoctl get deployment --organization default-org --project default-project --component react-starter-image --environment development

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

You’re all set! Now you understand how Choreo simplifies the deployment and management of cloud-native applications.

## Install
To see more Choreo installation options and detailed instructions, check out the [Installation Guide](install/README.md).

## Project roadmap
For information about the detailed project roadmap including delivered milestones, see the Roadmap.

## Community
To engage with our community, you can join the Choreo Open Source [Discord](https://discord.gg/HYCgUacN) channel.


## Contributing
Want to help develop Choreo Open Source? Check out our [contributing documentation](docs/contributing.md).
If you find an issue, please report it on the issue tracker.

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
