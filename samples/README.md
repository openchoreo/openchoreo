# OpenChoreo Samples

This directory contains sample implementations to help you understand, configure, and use OpenChoreo effectively. These samples demonstrate various deployment patterns and platform capabilities.

## Getting Started Resources

**[Getting Started](./getting-started)** contains the default resources needed to use OpenChoreo. Apply these after installing the control plane:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/getting-started/all.yaml
```

This creates a default project, three environments (development, staging, production), component types (service, web-application, scheduled-task), build workflows, and traits. See the [getting-started README](./getting-started/README.md) for details.

## Sample Categories

### [Deploy from Pre-built Images](./from-image)
Deploy applications using pre-built Docker images. This approach is ideal when you have existing CI systems that build and push container images to registries. These samples show how to deploy your containerized applications directly to OpenChoreo.

**Available Samples:**
- **[Go Greeter Service](./from-image/go-greeter-service/)** - Simple HTTP service demonstrating service deployment
- **[React Starter Web App](./from-image/react-starter-web-app/)** - Web application deployment example
- **[GitHub Issue Reporter](./from-image/issue-reporter-schedule-task/)** - Scheduled task deployment example

### [Build from Source](./from-source)
Build and deploy applications directly from source code using OpenChoreo's built-in CI system. OpenChoreo supports both BuildPacks (for automatic detection and containerization) and Docker (using your Dockerfile) to build applications from source code.

**Services:**
- **[Go Greeter Service (Docker)](./from-source/services/go-docker-greeter/)** - Build from source using Dockerfile
- **[Reading List Service (Buildpack)](./from-source/services/go-google-buildpack-reading-list/)** - Build from source using Google Cloud Buildpacks
- **[Patient Management Service (Buildpack)](./from-source/services/ballerina-buildpack-patient-management/)** - Ballerina service built with Buildpacks

**Web Applications:**
- **[React Starter](./from-source/web-apps/react-starter/)** - React web application built from source

### [Component Types](./component-types)
Low-level examples demonstrating how to define and use custom component types with OpenChoreo's ComponentType CRD. These samples show the underlying mechanics of how components work.

**Available Samples:**
- **[HTTP Service Component](./component-types/component-http-service/)** - Define a reusable HTTP service component type
- **[Web App Component](./component-types/component-web-app/)** - Define a reusable web application component type
- **[Component with Configs](./component-types/component-with-configs/)** - Demonstrate configuration management
- **[Component with Traits](./component-types/component-with-traits/)** - Demonstrate trait composition
- **[Component with Embedded Traits](./component-types/component-with-embedded-traits/)** - Demonstrate PE-defined embedded traits
- **[Component with API Management](./component-types/component-with-api-management/)** - Demonstrate API management via traits

### [Workflows](./workflows)
Reusable Workflow definitions organized by purpose. Workflows use the same `Workflow` CRD but serve two distinct roles.

**CI Workflows** (`workflows/ci/`) — define how applications are built and containerized as part of the Component lifecycle:
- **[Docker Workflow](./workflows/ci/docker.yaml)** - Build workflow for applications with Dockerfiles
- **[Google Cloud Buildpacks](./workflows/ci/google-cloud-buildpacks.yaml)** - Build workflow using Google Cloud Buildpacks
- **[React Workflow](./workflows/ci/react.yaml)** - Specialized build workflow for React applications

**Generic Workflows** (`workflows/generic/`) — standalone automation workflows independent of any Component:
- **[GitHub Stats Report](./workflows/generic/github-stats-report/)** - Fetch GitHub repository statistics, transform the data, and generate a formatted report

### [GCP Microservices Demo](./gcp-microservices-demo)
A complete microservices application based on Google's popular [microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo). This sample showcases how to deploy a full e-commerce application with multiple interconnected services using OpenChoreo.

### [MCP Server - AI Assistant Integration](./mcp)
Learn how to use OpenChoreo with AI assistants through the Model Context Protocol (MCP). Deploy and manage applications using natural language with AI assistants like Claude Code, Cursor, VS Code, and Claude Desktop.

**Prerequisites:**
- Running OpenChoreo instance
- MCP server configured
- AI assistant installed and configured

**Available Guides:**
- **[Getting Started](./mcp/getting-started/)** - Connect your AI assistant and explore OpenChoreo resources (10-15 min)
- **[Service Deployment](./mcp/service-deployment/)** - Deploy services from source code to production using AI assistants (30-45 min)
  - [Step-by-Step Guide](./mcp/service-deployment/step-by-step/) - Guided walkthrough with explicit prompts
  - [Developer Chat](./mcp/service-deployment/developer-chat/) - Natural conversation-based deployment
- **[Configuration Examples](./mcp/configs/)** - Setup instructions for different AI assistants

### [Platform Configuration](./platform-config)
Configuration samples targeted at Platform Engineers. Learn how to set up deployment pipelines, configure environments, and establish platform governance using OpenChoreo's abstractions.

**Available Configurations:**
- **[Deployment Pipeline](./platform-config/new-deployment-pipeline/)** - Define promotion pipelines across environments
- **[Environments](./platform-config/new-environments/)** - Configure development, QA, pre-production, and production environments
