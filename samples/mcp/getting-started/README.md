# Getting Started with OpenChoreo MCP Server

This guide walks you through basic operations using the OpenChoreo MCP server. You'll learn how to discover and explore your OpenChoreo resources using AI assistants.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

## What You'll Learn

In this guide, you'll learn:
- How to prompt AI assistants to interact with OpenChoreo using MCP tools
- How to discover and explore your OpenChoreo resources through natural language

## Step 1: Verify MCP Connection

First, verify that your AI assistant can connect to the OpenChoreo MCP server. In your AI assistant, try:

```
Do you have access to the OpenChoreo MCP server? List the available tools.
```

**Expected:** The assistant should confirm MCP access and list available OpenChoreo tools like `list_namespaces`, `list_projects`, `create_component`, etc.

If the connection isn't working, review the [Configuration Guide](../configs/) to ensure your setup is correct.

## Step 2: Explore OpenChoreo Resources

### Prompt 1: List namespaces

```
List available namespaces in my openchoreo cluster.
```

**What agent will do:**
1. Call `list_namespaces` to list namespaces. Note that this will return only the namespaces which contains the label: `openchoreo.dev/controlplane-namespace=true`
2. Display each namespace details including name, display name, and status

### Prompt 2: Get Project Details

```
Show me details of all projects in the "default" namespace
```

**What agent will do:**
1. Call `list_projects` with the namespace name
2. Display project details including description, status, component count, and creation date
3. Offer to show component details for any project

## Next Steps

You've successfully set up and tested the OpenChoreo MCP server! Continue with the [Service Deployment](../service-deployment/) guide to learn how to deploy applications using the MCP server.
