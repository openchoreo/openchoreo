# Deploying Service Component from Source - Developer Chat Guide

This guide demonstrates a natural conversation-based approach to deploying a service using the OpenChoreo MCP server. Instead of following explicit step-by-step instructions, you'll have a conversational interaction with your AI assistant to deploy your application.

## Prerequisites

Before starting, ensure you have completed all prerequisites from the [Service Deployment guide](../README.md).

**Time required:** 15-20 minutes

## Approach

This guide uses a conversational workflow where you interact naturally with your AI assistant. The assistant will:
- Ask clarifying questions when needed
- Use only OpenChoreo MCP tools (no curl, kubectl, or other external commands)
- Execute MCP tools efficiently and avoid unnecessary calls
- Reply concisely and keep the conversation focused
- Guide you through the deployment process through natural dialogue


### Step 1: Enter the Agent's system prompt

```
You're a useful agent to help make OpenChoreo tasks easier for OpenChoreo users. You can use only the OpenChoreo MCP server. Specifically, you can't use curl and kubectl. You may refer to OpenChoreo docs: https://openchoreo.dev/docs/next/. You may ask back the user questions when you want further input for a task.

You execute the MCP tools efficiently and avoid unnecessary tool calls. You reply concisely and keep the conversation clean. You don't generate .md files and other artifacts unless you're asked to.

To confirm, let me know whether you have access to the OpenChoreo MCP server.
```

The Agent will respond with the available MCP tools

### Step 2: Request Deployment

```
I want to deploy my Go REST API service on OpenChoreo. Source: https://github.com/openchoreo/sample-workloads/tree/main/service-go-greeter
The source contains a Dockerfile. You can use it for the deployment.
```

The agent will guide you through:
- Creating the project (if needed)
- Creating the component with Docker workflow configuration
- Triggering the build
- Monitoring build progress
- Verifying deployment
- Promoting to production (if desired)

The conversation will flow naturally based on your responses and the agent's questions.

## Tips for Best Results

1. **Be specific**: Provide clear answers to the agent's questions
2. **Verify steps**: Ask the agent to confirm what it's doing at each stage
3. **Monitor progress**: Request status updates on builds and deployments
4. **Ask questions**: If something is unclear, ask the agent to explain
