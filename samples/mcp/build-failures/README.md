# Build Failure Diagnosis with OpenChoreo MCP Server

This guide walks you through a build failure debugging scenario using the OpenChoreo MCP server and AI assistants. You'll trigger a build with a misconfigured Dockerfile path, then use MCP tools to find the failure, inspect the logs, and diagnose the root cause.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **Go Docker Greeter sample deployed** — follow the [Go Docker Greeter](../../from-source/services/go-docker-greeter/) sample to deploy the greeting service from source. Wait for the initial build (`greeting-service-build-01`) to complete successfully before proceeding.
2. **Workflow plane** installed and running — see [Setup Workflow Plane](https://openchoreo.dev/docs/getting-started/production/single-cluster/#step-3-setup-workflow-plane-optional) guide
3. **Both MCP servers configured** — you need both the Control Plane and Observability Plane MCP servers connected to your AI assistant. See the [Configuration Guide](https://openchoreo.dev/docs/ai/mcp-servers)

## What You'll Learn

- How to use MCP tools to list and inspect workflow runs
- How to retrieve and analyze build logs from a failed workflow
- How to use AI assistants to diagnose build failures and suggest fixes

## Scenario: Misconfigured Dockerfile Path

The Go Docker Greeter service builds from source using the `dockerfile-builder` workflow. We'll trigger a new build with an incorrect Dockerfile path, simulating a common misconfiguration that causes Docker builds to fail.

### Setup context

```
greeting-service (working build: /service-go-greeter/Dockerfile)
                          ↓
  Trigger new build with: /service-go-greeter/Dockerfile.broken  ← does not exist
```

## Step 1: Introduce the Build Failure

Trigger a new workflow run with a wrong Dockerfile path using `kubectl`:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: greeting-service-build-02
  labels:
    openchoreo.dev/project: "default"
    openchoreo.dev/component: "greeting-service"
spec:
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder
    parameters:
      repository:
        url: "https://github.com/openchoreo/sample-workloads"
        revision:
          branch: "main"
        appPath: "/service-go-greeter"
      docker:
        context: "/service-go-greeter"
        filePath: "/service-go-greeter/Dockerfile.broken"
EOF
```

Wait a couple of minutes for the build to run and fail.

## Step 2: Discover the Failed Build

Ask the AI assistant to check on recent builds.

```
List the recent workflow runs for the "greeting-service" component in the
"default" namespace and "default" project. Are there any failures?
```

**What agent will do:**
1. Call `list_workflow_runs` (Control Plane MCP) filtered to the greeting-service component
2. Display each workflow run with its status and timestamps
3. Flag the failed run

**Expected:** The assistant should show two workflow runs — `greeting-service-build-01` (succeeded) and `greeting-service-build-02` (failed).

## Step 3: Inspect the Failed Build

Get the details of the failed build to understand which step broke.

```
Show me the details of the "greeting-service-build-02" workflow run.
Which tasks failed?
```

**What agent will do:**
1. Call `get_workflow_run` (Control Plane MCP) with the workflow run name
2. Display the task-level breakdown showing which tasks succeeded and which failed
3. Identify the exact step where the build broke (the Docker build task)

**Expected:** The task breakdown should show that the Docker build step failed while earlier steps like source checkout succeeded.

## Step 4: Retrieve Build Logs

Get the actual error output from the failed build.

```
Get the build logs for the failed "greeting-service-build-02" workflow run.
Show me the error output.
```

**What agent will do:**
1. Call `query_workflow_logs` (Observability MCP) for the failed workflow run
2. Display the relevant log output focusing on the error messages
3. Highlight the key failure lines

**Expected:** The logs should show a Docker build error indicating that the Dockerfile at `/service-go-greeter/Dockerfile.broken` was not found.

## Step 5: Diagnose and Fix

Ask the AI assistant to analyze the failure and recommend a fix.

```
Analyze this build failure. What's the root cause and how should I fix it?
Also look at "greeting-service-build-01" (the previous successful run) to compare
the Dockerfile path it used.
```

**What agent will do:**
1. Compare the failed build's configuration with the successful `greeting-service-build-01`
2. Call `get_workflow_run` (Control Plane MCP) for the successful build to get its parameters
3. Identify that the Dockerfile path was changed from `Dockerfile` to `Dockerfile.broken`
4. Recommend correcting the path back to `/service-go-greeter/Dockerfile`

## Step 6: Trigger a Fixed Build

Apply the fix by triggering a new build with the correct Dockerfile path.

```
Trigger a new workflow run for the greeting-service component using the
dockerfile-builder workflow with the correct Dockerfile path
"/service-go-greeter/Dockerfile". Use the same repository and branch as before.
```

**What agent will do:**
1. Call `create_workflow_run` (Control Plane MCP) with the corrected parameters
2. Confirm the workflow run was created

Then verify:

```
Check the status of the latest greeting-service build. Has it completed successfully?
```

**What agent will do:**
1. Call `list_workflow_runs` or `get_workflow_run` (Control Plane MCP) to check the new build status
2. Confirm it completed successfully

**Expected:** The new build should complete successfully, confirming the fix.

## MCP Tools Used

| Tool | MCP Server | Purpose |
|------|------------|---------|
| `list_workflow_runs` | Control Plane | List builds and find failures |
| `get_workflow_run` | Control Plane | Inspect task-level build details |
| `query_workflow_logs` | Observability | Retrieve build log output |
| `create_workflow_run` | Control Plane | Trigger a new build with corrected config |

## Clean Up

Remove the failed workflow run:

```bash
kubectl delete workflowrun greeting-service-build-02 -n default
```

To remove the entire greeting service, follow the clean up steps in the [Go Docker Greeter](../../from-source/services/go-docker-greeter/) sample.

## Next Steps

- Try the [Log Analysis & Debugging](../log-analysis/) guide to debug cascading failures
- Try the [Resource Optimization](../resource-optimization/) guide to right-size your workloads
