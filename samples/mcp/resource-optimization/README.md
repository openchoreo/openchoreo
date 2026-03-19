# Resource Optimization with OpenChoreo MCP Server

This guide walks you through analyzing resource allocation versus actual usage and getting AI-powered right-sizing recommendations using the OpenChoreo MCP server.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **Deployed components** with active workloads running for at least 24 hours (for meaningful usage data)
2. **Observability plane** configured and running
   - See [Observability & Alerting](https://openchoreo.dev/docs/operations/observability-alerting/) for setup instructions

## What You'll Learn

In this guide, you'll learn:
- How to check current resource configuration (CPU and memory limits/requests)
- How to query actual resource usage metrics over time
- How to identify over-provisioned and under-provisioned workloads
- How to get AI-powered right-sizing recommendations
- How to apply optimized resource configurations

## Step 1: Check Current Resource Configuration

Start by examining the current resource allocation for your workload.

```
Show me the workload details for the "greeter-service" component in the "default" namespace and "my-project" project. Include CPU and memory limits and requests.
```

**What agent will do:**
1. Call `get_workload` with the component, namespace, and project details
2. Display the current resource configuration including CPU requests/limits and memory requests/limits
3. Show the number of replicas and other workload settings

## Step 2: Query Actual Resource Usage

Now get the actual resource consumption metrics over the last 24 hours.

```
Show me the CPU and memory usage metrics for the "greeter-service" workload over the last 24 hours.
```

**What agent will do:**
1. Call `query_resource_metrics` for CPU and memory usage over a 24-hour window
2. Display usage patterns including average, peak, and minimum utilization
3. Show how usage compares to the configured limits and requests

## Step 3: Compare and Analyze

Ask the AI assistant to compare the configured resources with actual usage.

```
Compare the resource allocation with actual usage. Are there any over-provisioned or under-provisioned resources?
```

**What agent will do:**
1. Analyze the gap between allocated resources and actual usage from previous steps
2. Identify over-provisioned resources (allocated much more than used)
3. Identify under-provisioned resources (usage approaching or exceeding limits)
4. Calculate potential cost savings from right-sizing

## Step 4: Get Right-Sizing Recommendations

Get specific recommendations for optimal resource values.

```
Suggest optimal CPU and memory requests and limits based on the usage data. Explain the reasoning.
```

**What agent will do:**
1. Calculate recommended values based on actual usage patterns with appropriate headroom
2. Provide specific CPU and memory request/limit values
3. Explain the reasoning behind each recommendation (e.g., peak usage plus 20% buffer)
4. Highlight any risks or trade-offs with the recommendations

## Step 5 (Optional): Apply Changes

If you're satisfied with the recommendations, apply the new resource configuration.

```
Update the "greeter-service" workload with the recommended resource values.
```

**What agent will do:**
1. Call `update_workload` with the new CPU and memory request/limit values
2. Confirm the changes were applied successfully
3. Suggest monitoring the workload after changes to verify stability

**Expected:** Your workload is now right-sized with optimal resource allocation, reducing waste while maintaining sufficient headroom for traffic spikes.

## Next Steps

- Try the [Log Analysis](../log-analysis/) guide to monitor runtime errors
- Try the [Build Failure Diagnosis](../build-failures/) guide to troubleshoot CI/CD issues
