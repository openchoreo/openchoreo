# Resource Optimization with OpenChoreo MCP Server

This guide walks you through a resource optimization scenario using the OpenChoreo MCP server and AI assistants. You'll intentionally over-provision services in the GCP Microservices Demo, then use AI-assisted analysis to detect the waste and right-size the deployments.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **GCP Microservices Demo deployed** — follow the [GCP Microservices Demo](../../gcp-microservices-demo/) sample to deploy the Online Boutique application
2. **Observability plane** configured and running — see [Observability & Alerting](https://openchoreo.dev/docs/platform-engineer-guide/observability-alerting/) for setup instructions
3. **Both MCP servers configured** — you need both the Control Plane and Observability Plane MCP servers connected to your AI assistant. See the [Configuration Guide](https://openchoreo.dev/docs/ai/mcp-servers)

## What You'll Learn

- How to inspect current resource allocation for release bindings using MCP tools
- How to query actual CPU and memory usage metrics
- How to use AI assistants to compare allocation vs usage and detect waste
- How to get right-sizing recommendations and apply them via MCP

## Scenario: Over-Provisioned Microservices

In real clusters, services often get allocated far more CPU and memory than they actually need — either from copy-pasting defaults or cautious initial sizing. We'll simulate this by patching several services with excessive resource requests, then use the AI assistant to find and fix the waste.

### Architecture context

```
frontend (over-provisioned: 2 CPU, 2Gi memory)
checkout (over-provisioned: 2 CPU, 2Gi memory)
cart     (over-provisioned: 1 CPU, 1Gi memory)
```

These lightweight services typically use a fraction of these resources.

## Step 1: Introduce Over-Provisioning

Patch the ReleaseBindings with excessive resource allocations. This is the OpenChoreo-native way to configure resources — patching deployments directly would be overwritten by the controllers.

```bash
# Over-provision frontend: 2 CPU / 2Gi memory
kubectl patch releasebinding frontend-development -n default \
  --type=merge -p '{
  "spec": {"componentTypeEnvironmentConfigs": {
    "resources": {"requests": {"cpu": "2", "memory": "2Gi"},
                  "limits": {"cpu": "2", "memory": "2Gi"}}}}}'

# Over-provision checkout: 2 CPU / 2Gi memory
kubectl patch releasebinding checkout-development -n default \
  --type=merge -p '{
  "spec": {"componentTypeEnvironmentConfigs": {
    "resources": {"requests": {"cpu": "2", "memory": "2Gi"},
                  "limits": {"cpu": "2", "memory": "2Gi"}}}}}'

# Over-provision cart: 1 CPU / 1Gi memory
kubectl patch releasebinding cart-development -n default \
  --type=merge -p '{
  "spec": {"componentTypeEnvironmentConfigs": {
    "resources": {"requests": {"cpu": "1", "memory": "1Gi"},
                  "limits": {"cpu": "1", "memory": "1Gi"}}}}}'
```

Wait a couple of minutes for the pods to restart and for usage metrics to start flowing.

## Step 2: Inspect Resource Allocation

Ask the AI assistant to check the current resource configuration across the project.

```
Show me the resource configuration (CPU and memory requests and limits) from
the release bindings for all components in the "default" namespace,
"gcp-microservice-demo" project.
```

**What agent will do:**
1. Call `list_components` (Control Plane MCP) to discover all components in the project
2. Call `list_release_bindings` (Control Plane MCP) to discover release bindings of each component
3. Call `get_release_binding` (Control Plane MCP) for each release binding
4. Display a summary of resource allocations across all components

**Expected:** The assistant should show that **frontend**, **checkout**, and **cart** have significantly higher resource allocations than the other services.

## Step 3: Query Actual Resource Usage

Now compare the allocations against what the services are actually consuming.

```
Query the CPU and memory usage metrics for the frontend, checkout, and cart
components in the "default" namespace, "gcp-microservice-demo" project,
"development" environment over the last 15 minutes. Compare with their
configured limits.
```

**What agent will do:**
1. Call `query_resource_metrics` (Observability MCP) for each of the three components
2. Display actual CPU and memory consumption alongside configured requests/limits
3. Highlight the gap between allocation and usage

**Expected:** The metrics should show that actual usage is a small fraction of the allocated resources — e.g., frontend using ~50m CPU out of 2000m allocated, or ~100Mi memory out of 2Gi allocated.

## Step 4: Get Right-Sizing Recommendations

Ask the AI assistant to analyze the waste and suggest optimal values.

```
Based on the actual usage data, these services look over-provisioned.
Suggest optimal CPU and memory requests and limits for frontend, checkout,
and cart. Include a safety buffer and explain your reasoning.
```

**What agent will do:**
1. Compare actual usage patterns against current allocations
2. Calculate recommended values with appropriate headroom (e.g., 2x peak usage for requests, 3x for limits)
3. Provide specific values for each service
4. Estimate the resource savings

## Step 5: Apply the Recommendations

Apply the optimized resource configuration using the MCP server.

```
Update the release bindings for frontend, checkout, and cart with the recommended resource values.
```

**What agent will do:**
1. Call `update_release_binding` (Control Plane MCP) for each component with the new resource values
2. Confirm each update was applied successfully

## Step 6: Verify the Changes

Confirm the new configuration is in place and the services are healthy.

```
Show me the updated release binding details for frontend, checkout, and cart.
Confirm the resource values were applied correctly and check if the components are running.
```

**What agent will do:**
1. Call `get_release_binding` (Control Plane MCP) for each component to confirm the new values
2. Report on the deployment status and whether the components are running

**Expected:** All three services should be running with the optimized resource allocations and no resource-related restarts.

## MCP Tools Used

| Tool | MCP Server | Purpose |
|------|------------|---------|
| `list_components` | Control Plane | Discover services in the project |
| `list_release_bindings` | Control Plane | Discover release bindings for each component |
| `get_release_binding` | Control Plane | Inspect resource allocation per component |
| `query_resource_metrics` | Observability | Query actual CPU and memory usage |
| `update_release_binding` | Control Plane | Apply optimized resource values |

## Next Steps

- Try the [Log Analysis & Debugging](../log-analysis/) guide to debug cascading failures
- Try the [Build Failure Diagnosis](../build-failures/) guide to troubleshoot CI/CD issues
