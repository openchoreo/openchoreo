# Log Analysis & Debugging with OpenChoreo MCP Server

This guide walks you through a real debugging scenario using the OpenChoreo MCP server and AI assistants. You'll intentionally break a multi-service application, then use logs, traces, and metrics to diagnose and fix the issue.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **GCP Microservices Demo deployed** — follow the [GCP Microservices Demo](../../gcp-microservices-demo/) sample to deploy the Online Boutique application
2. **Observability plane** configured and running — see [Observability & Alerting](https://openchoreo.dev/docs/platform-engineer-guide/observability-alerting/) for setup instructions
3. **Both MCP servers configured** — you need both the Control Plane and Observability Plane MCP servers connected to your AI assistant. See the [Configuration Guide](https://openchoreo.dev/docs/ai/mcp-servers)

## What You'll Learn

- How to use AI assistants to investigate errors across a distributed microservices application
- How to query and analyze component logs filtered by severity and time range
- How to trace requests across service boundaries to find the root cause
- How to correlate logs, traces, and metrics for a complete picture

## Scenario: Breaking the Product Catalog

The GCP Microservices Demo (Online Boutique) has interconnected services. The **product catalog** service is a critical dependency — **checkout**, **recommendation**, and **frontend** all depend on it. We'll scale it down to zero replicas to simulate an outage, then debug the cascading failures.

### Architecture context

```
frontend ──→ productcatalog   ← we'll break this
frontend ──→ checkout ──→ productcatalog
frontend ──→ recommendation ──→ productcatalog
```

## Step 1: Introduce the Failure

Scale the product catalog service to zero replicas by patching its ReleaseBinding. This is the OpenChoreo-native way to change replica count — patching the deployment directly would be overwritten by the controllers.

```bash
kubectl patch releasebinding productcatalog-development -n default \
  --type=merge -p '{"spec": {"componentTypeEnvironmentConfigs": {"replicas": 0}}}'
```

Now visit the frontend in your browser. You should see errors when browsing products or attempting checkout. Let it run for a minute or two so logs and traces accumulate.

## Step 2: Discover the Problem Through Logs

Start your debugging session by asking the AI assistant to check for errors.

```
Are there any errors in the GCP microservices demo application? Check the logs
for components in the "default" namespace, "gcp-microservice-demo" project,
"development" environment. Look at the last 15 minutes.
```

**What agent will do:**
1. Call `list_components` (Control Plane MCP) to discover all components in the project
2. Call `query_component_logs` (Observability MCP) for each component, filtering by error severity and the last 15 minutes
3. Report which components are logging errors and summarize the error messages

**Expected:** The assistant should report errors in **frontend**, **checkout**, and **recommendation** — all complaining about failed connections to the product catalog service.

## Step 3: Trace a Failed Request

Pick a failing request and trace it across service boundaries.

```
Find traces for failed requests in the frontend component over the last 15 minutes.
Show me the spans for one of the failed traces so I can see where the request broke.
```

**What agent will do:**
1. Call `query_traces` (Observability MCP) filtered to the frontend component and recent time window
2. Identify traces with error status
3. Call `query_trace_spans` with a specific trace ID to get the full span tree
4. Display the request flow showing which span failed and in which service

**Expected:** The trace should show the frontend calling the product catalog service (or checkout calling it), with the final span showing a connection failure — gRPC `UNAVAILABLE` or connection refused.

## Step 4: Check Deployment State

Confirm the root cause by checking the deployment state.

```
Show me the release binding details for the "productcatalog" component in the
"development" environment. Is it running? How many replicas are configured?
```

**What agent will do:**
1. Call `get_release_binding` (Control Plane MCP) for the productcatalog component's development release binding
2. Display the current replica count, resource configuration, and status

**Expected:** The assistant should report that the product catalog has **0 replicas** — confirming why all dependent services are failing.

## Step 5: Fix and Verify

Ask the AI assistant to fix the issue and verify recovery.

```
Update the productcatalog release binding to set replicas back to 1.
```

**What agent will do:**
1. Call `update_release_binding` (Control Plane MCP) to set the productcatalog replicas to 1
2. Confirm the update was applied

Wait a minute for the service to come back up, then verify:

```
Check the logs again for the last 2 minutes. Are the errors resolved?
Are there any new traces showing successful requests?
```

**What agent will do:**
1. Call `query_component_logs` for the affected components over the last 2 minutes
2. Call `query_traces` to find recent successful traces
3. Confirm the errors have stopped and requests are completing successfully

## MCP Tools Used

| Tool | MCP Server | Purpose |
|------|------------|---------|
| `list_components` | Control Plane | Discover services in the project |
| `query_component_logs` | Observability | Query logs filtered by severity and time |
| `query_traces` | Observability | Find distributed traces for failed requests |
| `query_trace_spans` | Observability | Inspect individual spans within a trace |
| `get_release_binding` | Control Plane | Check component replica count and deployment status |
| `update_release_binding` | Control Plane | Fix the replica count to restore the service |

## Next Steps

- Try the [Build Failure Diagnosis](../build-failures/) guide to troubleshoot CI/CD issues
- Try the [Resource Optimization](../resource-optimization/) guide to right-size your deployments
