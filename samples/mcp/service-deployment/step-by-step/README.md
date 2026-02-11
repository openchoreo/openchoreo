
# Deploying Service Component from Source - Step-by-Step Guide

This comprehensive guide walks you through the complete lifecycle of deploying an application using the OpenChoreo MCP server - from creating a project to deploying a REST API service to production.

## Prerequisites

Before starting, ensure you have completed all prerequisites from the [Service Deployment guide](../README.md).

**Time required:** 15-20 minutes

## Scenario Overview

In this scenario, you'll deploy a simple "greeter" service:
- **Backend**: Go REST API service that responds with greeting messages

You'll learn to:
1. Verify prerequisites and infrastructure
2. Create a new project
3. Create a service component and configure Docker build workflow
4. Trigger builds from source
5. Deploy to development environment
6. Test the application
7. Promote to production
8. Monitor deployment status

## Step 1: Verify Prerequisites

First, verify that your OpenChoreo instance has the necessary infrastructure.
Prompt:
```
List the environments in my "default" namespace
```

**Expected:** You should see at least "development" environment. If not, create one following the [platform configuration samples](../../../platform-config/new-environments/).

## Step 2: Create a New Project

Let's create a project called "greeter" for our application.

Prompt:
```
Create a new project called "greeter" in the "default" namespace with description "Simple greeter service in Go"
```

**What agent will do:**
1. Call `create_project` with the provided details
2. Confirm project creation
3. Show the project details

## Step 3: Create Component and Configure Build Workflow

Now let's create the greeter service component and configure its build workflow.

Prompt:
```
Create a component called "greeter-service" in the greeter project. 
It should be a deployment/service type, listening on port 9090, 
with 1 replica, and publicly exposed. Configure it to build from 
the GitHub repository https://github.com/openchoreo/sample-workloads,
using the Docker workflow, with the source in /service-go-greeter directory.
```

**What agent will do:**
1. Check `create_component` tool schema to understand required parameters
2. Call `list_component_types`, `list_component_workflows`, `get_component_workflow_schema` to identify available component types and workflow schemas to generate component creation request
3. Call `create_component` with generated configuration
4. Show the component details 

**Checkpoint:** The greeter-service component should be created with its build workflow configured.

## Step 4: Trigger Build

Now let's build the container image for our greeter service.

### Build Greeter Service

Prompt:
```
Trigger a build for the greeter-service component in the greeter project using the main branch
```

**What agent will do:**
1. Call `trigger_component_workflow` with the component details
2. Return a build ID for tracking
3. Provide commands to monitor build progress

### Monitor Build Progress

Prompt:
```
What's the status of the greeter-service build? Check periodically until the build finishes
```

**What agent will do:**
1. Call `list_component_workflow_runs` for the component
2. Show build status, duration, and outcome
3. Alert if the build failed

**Checkpoint:** The build should complete successfully. If it fails, review build logs and fix issues.
**Checkpoint:** Since the component creation tool enables automatic deployment, a release should be automatically triggered on the development environment

## Step 5: Verify Deployment

Let's verify that the greeter service is running correctly.

### Check Component Status

Prompt:
```
Show me the deployment status of the greeter-service in the greeter project
```

**What agent will do:**
1. Call `get_component_workloads`, `list_release_bindings` for the component
2. Show pod status, health, and resource usage
3. Highlight any issues

### Get Access URL

Prompt:
```
What is the endpoint URL for my greeter service in development? The gateway is running on the port 19080
```

**What agent will do:**
1. Extract endpoint information from component details
2. Provide formatted URL for accessing the service

### Test the Service

**Sample Curl**
```
curl http://development-default.openchoreoapis.localhost:19080/greeter/greet?name=World
```

**Expected response:**
```
Hello, World
```

**Checkpoint:** The greeter service should be accessible and responding correctly.

## Step 6: Promote to Production

After testing in development, let's promote the greeter service to production.

### Promote the Service

Prompt:
```
Promote the greeter-service up to the production environment
```

**What agent will do:**
1. Call `promote_component` with source and target environments 2 times (development --> staging --> production)
2. Confirm the promotion
3. Show deployment status in production

**Checkpoint:** The greeter service should be running in production.

## Congratulations! 🎉

You've successfully deployed a service using the OpenChoreo MCP server! You now know how to:

- Create projects and components using AI assistants
- Configure Docker build workflows from source repositories
- Trigger and monitor builds programmatically
- Promote services through the deployment pipeline
- Monitor component health and status
- Manage the complete service lifecycle through natural language

**Share your success** with the community on [Discord](https://discord.gg/asqDFC8suT)!
