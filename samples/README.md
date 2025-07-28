# OpenChoreo Samples

This directory contains sample implementations to help you understand, configure, and use OpenChoreo effectively. These samples demonstrate various deployment patterns and platform capabilities.

## Sample Categories

### [Deploy from Pre-built Images](./from-image)
Deploy applications using pre-built Docker images. This approach is ideal when you have existing CI systems that build and push container images to registries. These samples show how to deploy your containerized applications directly to OpenChoreo.

### [Build from Source](./from-source)
Build and deploy applications directly from source code using OpenChoreo's built-in CI system. OpenChoreo supports both BuildPacks (for automatic detection and containerization) and Docker (using your Dockerfile) to build applications from source code.

### [API Management Features](./apim-samples)
Demonstrate API management capabilities that can enhance your services. Learn how to add authentication, rate limiting, circuit breakers, and CORS policies to your APIs using OpenChoreo's API management features.

### [GCP Microservices Demo](./gcp-microservices-demo)
A complete microservices application based on Google's popular [microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo). This sample showcases how to deploy a full e-commerce application with multiple interconnected services using OpenChoreo.

### [Platform Configuration](./platform-config)
Configuration samples targeted at Platform Engineers. Learn how to set up deployment pipelines, configure environments, and establish platform governance using OpenChoreo's abstractions.
