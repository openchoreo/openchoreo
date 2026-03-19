# Build Failure Diagnosis with OpenChoreo MCP Server

This guide walks you through diagnosing build failures using the OpenChoreo MCP server and AI assistants. You'll learn how to find failed builds, inspect their logs, and get AI-powered fix suggestions.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **Component workflows** configured (Docker or Buildpacks)
   - Make sure you've installed the workflow plane. See [Setup Workflow Plane](https://openchoreo.dev/docs/getting-started/production/single-cluster/#step-3-setup-workflow-plane-optional) guide.
2. **At least one workflow run** (successful or failed) to inspect

## What You'll Learn

In this guide, you'll learn:
- How to list and filter workflow runs to find failed builds
- How to inspect build details at the task level
- How to retrieve and analyze build logs
- How to use AI assistants to diagnose failures and suggest fixes

## Step 1: List Recent Workflow Runs

Start by listing recent workflow runs to identify any failures.

```
Show me the recent workflow runs for the "greeter-service" component in the "default" namespace and "my-project" project. Highlight any failures.
```

**What agent will do:**
1. Call `list_workflow_runs` with the component, namespace, and project details
2. Display each workflow run with its status, trigger time, and duration
3. Flag any runs with a failed status

## Step 2: Inspect a Failed Build

Pick a failed workflow run and get detailed information about what went wrong.

```
Show me the details of the most recent failed workflow run. What tasks failed?
```

**What agent will do:**
1. Call `get_workflow_run` with the specific workflow run identifier
2. Display the task-level breakdown showing which tasks succeeded and which failed
3. Identify the exact step where the build broke

## Step 3: Retrieve Build Logs

Get the actual log output from the failed build step.

```
Get the build logs for the failed task. Show me the error output.
```

**What agent will do:**
1. Call `query_workflow_logs` for the failed workflow run and task
2. Display the relevant log output focusing on the error messages
3. Highlight the key failure lines

## Step 4: Analyze Failure and Suggest Fixes

Ask the AI assistant to diagnose the root cause and recommend fixes.

```
Analyze this build failure. What's the root cause and how do I fix it?
```

**What agent will do:**
1. Review the build logs and task details from previous steps
2. Identify the root cause (e.g., dependency issue, syntax error, Docker build failure, resource limit)
3. Provide specific, actionable fix steps
4. Suggest any preventive measures to avoid similar failures

**Expected:** A clear diagnosis of the build failure with step-by-step instructions to resolve the issue and get the build passing again.

## Next Steps

- Try the [Log Analysis](../log-analysis/) guide to monitor runtime errors
- Try the [Resource Optimization](../resource-optimization/) guide to right-size your workloads
