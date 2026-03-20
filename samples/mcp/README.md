# OpenChoreo MCP Server - AI Assistant Examples

This directory contains practical guides for using the OpenChoreo MCP (Model Context Protocol) server with AI assistants like Claude Code, Cursor, VS Code, and Claude Desktop.

## Prerequisites

Before you begin, ensure you have completed the following prerequisites:

### 1. Running OpenChoreo Instance

You need a running OpenChoreo instance. If you haven't set one up yet:

- Complete the [Quick Start Guide](https://openchoreo.dev/docs/getting-started/quick-start-guide/)
- Or follow the [Installation Guide](https://openchoreo.dev/docs/getting-started/production/deployment-planning/) for your preferred setup method

### 2. Configure MCP Server

The OpenChoreo MCP server must be configured and accessible. See the [Configuration Guide](https://openchoreo.dev/docs/next/ai/mcp-servers) for detailed instructions to obtain:

- MCP server endpoint
- An access token

### 3. AI Assistant Setup

Install an AI Assistant (AI Agent) and configure the OpenChoreo MCP server.
- Transport: `http`
- Authorization header

## Guides

### 1. [Getting Started](./getting-started/)
Learn the basics of connecting your AI assistant to OpenChoreo and performing simple operations like listing namespaces and projects.

**Prerequisites:** All prerequisites above must be completed.

**Time:** 2 minutes

### 2. [Service Deployment](./service-deployment/)
Deploy a complete service from source code to production using the OpenChoreo MCP server. Choose from:

- **[Step-by-Step Guide](./service-deployment/step-by-step/)** - Guided walkthrough with explicit prompts
- **[Developer Chat](./service-deployment/developer-chat/)** - Natural conversation-based deployment

**Prerequisites:** All prerequisites above, plus:
- At least two environments configured (development and production)
- Component workflows configured (Docker or Buildpacks)

**Time:** 15-20 minutes

### 3. [Log Analysis & Debugging](./log-analysis/)
Debug a cascading failure in the GCP Microservices Demo (Online Boutique) using logs, traces, and workload inspection. You'll intentionally break the product catalog service, then use AI-assisted observability to diagnose and fix the issue.

**Prerequisites:** All prerequisites above, plus:
- [GCP Microservices Demo](../gcp-microservices-demo/) deployed and running
- Observability plane configured and running
- Both Control Plane and Observability Plane MCP servers configured

**Time:** ~10 minutes

### 4. [Build Failure Diagnosis](./build-failures/)
Debug a Docker build failure in the Go Greeter service. You'll trigger a build with a misconfigured Dockerfile path, then use AI-assisted workflow inspection and log analysis to diagnose and fix the issue.

**Prerequisites:** All prerequisites above, plus:
- [Go Docker Greeter](../from-source/services/go-docker-greeter/) sample deployed with a successful initial build
- Workflow plane installed and running
- Both Control Plane and Observability Plane MCP servers configured

**Time:** ~10 minutes

### 5. [Resource Optimization](./resource-optimization/)
Detect and fix over-provisioned workloads in the GCP Microservices Demo. You'll intentionally allocate excessive CPU and memory, then use AI-assisted analysis to compare allocation vs actual usage and apply right-sized configurations.

**Prerequisites:** All prerequisites above, plus:
- [GCP Microservices Demo](../gcp-microservices-demo/) deployed and running
- Observability plane configured and running
- Both Control Plane and Observability Plane MCP servers configured

**Time:** ~10 minutes

## Getting Help

- [OpenChoreo Documentation](https://openchoreo.dev/)
- [Slack Community](https://cloud-native.slack.com/archives/C0ABYRG1MND)
