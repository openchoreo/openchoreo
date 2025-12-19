# Service Deployment with OpenChoreo MCP Server

This guide demonstrates how to deploy a REST API service from source code to production on OpenChoreo using the MCP server and AI assistants.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, for service deployment you need:

1. **OpenChoreo instance** with at least two environments configured (development and production)
   - If you need to create environments, see the [platform configuration samples](../../platform-config/new-environments/)
2. **Component workflows** configured (Docker or Buildpacks)
   - Make sure you've installed the build plane. See [Setup Build Plane](https://openchoreo.dev/docs/getting-started/production/single-cluster/#step-3-setup-build-plane-optional) guide.

## Deployment Guides

Choose the approach that best fits your workflow:

### 1. [Step-by-Step Guide](./step-by-step/)

A guided walkthrough with explicit prompts for each step. Perfect if you want:
- Clear, structured instructions
- Explicit prompts to copy and paste
- Step-by-step verification checkpoints

**Time:** 15-20 minutes

### 2. [Developer Chat](./developer-chat/)

A natural conversation-based deployment. Perfect if you want:
- Conversational interaction with the AI assistant
- More flexible, exploratory workflow
- Natural language deployment process

**Time:** 15-20 minutes

## What You'll Learn

Both guides will teach you how to:
- Create projects and components using AI assistants
- Configure Docker build workflows from source repositories
- Trigger and monitor builds programmatically
- Deploy services to development environments
- Promote services through the deployment pipeline (development → staging → production)
- Monitor component health and status
- Manage the complete service lifecycle through natural language

## Scenario Overview

In both guides, you'll deploy a simple "greeter" service:
- **Backend**: Go REST API service that responds with greeting messages
- **Source**: GitHub repository with Dockerfile
- **Deployment**: From source code to production through multiple environments
