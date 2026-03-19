# Log Analysis with OpenChoreo MCP Server

This guide walks you through analyzing application logs to identify errors and suggest fixes using the OpenChoreo MCP server and AI assistants.

## Prerequisites

Before starting this guide, ensure you have completed all [prerequisites](../README.md#prerequisites)

Additionally, you need:

1. **Deployed components** with active workloads generating logs
2. **Observability plane** configured and running
   - See [Observability & Alerting](https://openchoreo.dev/docs/operations/observability-alerting/) for setup instructions

## What You'll Learn

In this guide, you'll learn:
- How to discover components and query their logs using MCP tools
- How to filter logs by severity and time range
- How to use AI assistants to analyze error patterns and suggest fixes
- How to correlate logs with distributed traces for deeper analysis

## Step 1: Discover Components

First, list the components in your project to identify which ones to analyze.

```
List all components in the "default" namespace and "my-project" project.
```

**What agent will do:**
1. Call `list_components` with the namespace and project name
2. Display each component's name, type, and status

## Step 2: Query Recent Error Logs

Now query the logs for a specific component, filtering for errors.

```
Show me the error logs from the "greeter-service" component in the last 1 hour.
```

**What agent will do:**
1. Call `query_component_logs` with the component name, severity filter set to error level, and a time range of the last hour
2. Display the log entries with timestamps, severity, and message content
3. Highlight any recurring error patterns

## Step 3: Analyze Errors and Suggest Fixes

Ask the AI assistant to analyze the errors and provide actionable recommendations.

```
Analyze these errors and suggest possible fixes. Group them by root cause if there are multiple issues.
```

**What agent will do:**
1. Review the log entries from the previous step
2. Group errors by root cause or pattern
3. Provide specific fix suggestions for each error category
4. Recommend priority based on frequency and severity

## Step 4 (Optional): Correlate with Traces

For deeper analysis, correlate error logs with distributed traces to understand the full request flow.

```
Find traces related to these errors. Show me the spans for any failed requests.
```

**What agent will do:**
1. Call `query_traces` to find traces matching the error time window and component
2. Call `query_trace_spans` to get detailed span information for failed traces
3. Display the end-to-end request flow showing where failures occurred
4. Identify upstream or downstream dependencies involved in the errors

**Expected:** A complete picture of the error context including which services are affected, where in the request chain the failure occurs, and targeted fix recommendations.

## Next Steps

- Try the [Build Failure Diagnosis](../build-failures/) guide to troubleshoot CI/CD issues
- Try the [Resource Optimization](../resource-optimization/) guide to right-size your workloads
