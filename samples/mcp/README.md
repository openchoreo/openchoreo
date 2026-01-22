# OpenChoreo MCP Server - AI Assistant Examples

This directory contains practical guides for using the OpenChoreo MCP (Model Context Protocol) server with AI assistants like Claude Code, Cursor, VS Code, and Claude Desktop.

## Prerequisites

Before you begin, ensure you have completed the following prerequisites:

### 1. Running OpenChoreo Instance

You need a running OpenChoreo instance. If you haven't set one up yet:

- Complete the [Quick Start Guide](https://openchoreo.dev/docs/getting-started/quick-start-guide/)
- Or follow the [Installation Guide](https://openchoreo.dev/docs/getting-started/production/deployment-planning/) for your preferred setup method

### 2. Configure MCP Server

The OpenChoreo MCP server must be configured and accessible. See the [Configuration Guide](./configs/) for detailed instructions to obtain:

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

## Getting Help

- [OpenChoreo Documentation](https://openchoreo.dev/)
- [Discord Community](https://discord.gg/asqDFC8suT)
