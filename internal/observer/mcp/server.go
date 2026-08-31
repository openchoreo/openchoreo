// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHTTPServer creates a new MCP HTTP server for the observer API
func NewHTTPServer(handler *MCPHandler) http.Handler {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "openchoreo-observer",
		Version: "1.0.0",
	}, nil)

	registerTools(server, handler)

	return mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return server
	}, nil)
}

// handleToolResult marshals the result to JSON and wraps it in MCP CallToolResult format
func handleToolResult(result any, err error) (*mcpsdk.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(jsonData)},
		},
	}, result, nil
}

func registerTools(s *mcpsdk.Server, handler *MCPHandler) {
	// Tool 1: query_component_logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_component_logs",
		Description: "Query runtime application logs for components (services, APIs, workers, scheduled tasks) deployed in OpenChoreo. Supports filtering by project, component, environment, time range, log levels, and search phrases.",
		InputSchema: createSchema(map[string]any{
			"namespace":     stringProperty("Organization namespace (required)"),
			"project":       stringProperty("Project name to filter logs"),
			"component":     stringProperty("Component name to filter logs"),
			"environment":   stringProperty("Environment name to filter logs (e.g., 'development', 'production')"),
			"start_time":    stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":      stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"search_phrase": stringProperty("Text to search within log messages"),
			"log_levels":    arrayProperty("Log levels to filter (e.g., ['ERROR', 'WARN', 'INFO', 'DEBUG']). Default: all levels"),
			"limit":         limitLogsProperty(),
			"sort_order":    sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace    string   `json:"namespace"`
		Project      string   `json:"project"`
		Component    string   `json:"component"`
		Environment  string   `json:"environment"`
		StartTime    string   `json:"start_time"`
		EndTime      string   `json:"end_time"`
		SearchPhrase string   `json:"search_phrase"`
		LogLevels    []string `json:"log_levels"`
		Limit        int      `json:"limit"`
		SortOrder    string   `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryComponentLogs(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.SearchPhrase,
			args.LogLevels, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 2: query_workflow_logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_workflow_logs",
		Description: "Query CI/CD workflow run logs in OpenChoreo. Captures build, test, and deployment pipeline execution details. Supports filtering by workflow run name and task name.",
		InputSchema: createSchema(map[string]any{
			"namespace":         stringProperty("Organization namespace (required)"),
			"workflow_run_name": stringProperty("Workflow run name to filter logs for a specific CI/CD run"),
			"task_name":         stringProperty("Task name within a workflow run to filter logs for a specific step"),
			"start_time":        stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":          stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"search_phrase":     stringProperty("Text to search within log messages"),
			"log_levels":        arrayProperty("Log levels to filter (e.g., ['ERROR', 'WARN', 'INFO', 'DEBUG']). Default: all levels"),
			"limit":             limitLogsProperty(),
			"sort_order":        sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace       string   `json:"namespace"`
		WorkflowRunName string   `json:"workflow_run_name"`
		TaskName        string   `json:"task_name"`
		StartTime       string   `json:"start_time"`
		EndTime         string   `json:"end_time"`
		SearchPhrase    string   `json:"search_phrase"`
		LogLevels       []string `json:"log_levels"`
		Limit           int      `json:"limit"`
		SortOrder       string   `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.QueryWorkflowLogs(ctx,
			args.Namespace, args.WorkflowRunName, args.TaskName,
			args.StartTime, args.EndTime, args.SearchPhrase,
			args.LogLevels, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool: query_component_events
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_component_events",
		Description: "Query Kubernetes events for components (services, APIs, workers, scheduled tasks) deployed in OpenChoreo. Returns events such as scheduling, scaling, image pulls, and job completions. Supports filtering by project, component, environment, and time range.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter events"),
			"component":   stringProperty("Component name to filter events"),
			"environment": stringProperty("Environment name to filter events (e.g., 'development', 'production')"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":       limitProperty(),
			"sort_order":  sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Limit       int    `json:"limit"`
		SortOrder   string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryComponentEvents(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool: query_workflow_events
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_workflow_events",
		Description: "Query Kubernetes events for CI/CD workflow runs in OpenChoreo. Captures events emitted during build, test, and deployment pipeline execution. Supports filtering by workflow run name and time range.",
		InputSchema: createSchema(map[string]any{
			"namespace":         stringProperty("Organization namespace (required)"),
			"workflow_run_name": stringProperty("Workflow run name to filter events for a specific CI/CD run"),
			"start_time":        stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":          stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":             limitProperty(),
			"sort_order":        sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace       string `json:"namespace"`
		WorkflowRunName string `json:"workflow_run_name"`
		StartTime       string `json:"start_time"`
		EndTime         string `json:"end_time"`
		Limit           int    `json:"limit"`
		SortOrder       string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.QueryWorkflowEvents(ctx,
			args.Namespace, args.WorkflowRunName,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 3: query_resource_metrics
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_resource_metrics",
		Description: "Query CPU and memory resource usage metrics for components in OpenChoreo. Returns time-series data for CPU usage/requests/limits and memory usage/requests/limits. Useful for capacity planning, identifying resource constraints, and detecting memory leaks.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter metrics"),
			"component":   stringProperty("Component name to filter metrics"),
			"environment": stringProperty("Environment name to filter metrics"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"step":        stringProperty("Query resolution step (e.g., '1m', '5m', '15m', '30m', '1h'). Controls data point density"),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Step        string `json:"step"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		var step *string
		if args.Step != "" {
			step = &args.Step
		}
		result, err := handler.QueryResourceMetrics(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, step,
		)
		return handleToolResult(result, err)
	})

	// Tool 4: query_http_metrics
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_http_metrics",
		Description: "Query HTTP request and latency metrics for components in OpenChoreo. Returns time-series data for request counts (total, successful, unsuccessful), mean latency, and percentile latencies (p50, p90, p99). Useful for monitoring API performance and debugging HTTP errors.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter metrics"),
			"component":   stringProperty("Component name to filter metrics"),
			"environment": stringProperty("Environment name to filter metrics"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"step":        stringProperty("Query resolution step (e.g., '1m', '5m', '15m', '30m', '1h'). Controls data point density"),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Step        string `json:"step"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		var step *string
		if args.Step != "" {
			step = &args.Step
		}
		result, err := handler.QueryHTTPMetrics(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, step,
		)
		return handleToolResult(result, err)
	})

	// Tool 5: query_traces
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_traces",
		Description: "Query distributed traces for components in OpenChoreo. Returns a list of traces with summary information including trace ID, name, span count, root span details, and duration. Useful for understanding request flows across services and identifying performance bottlenecks.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter traces"),
			"component":   stringProperty("Component name to filter traces"),
			"environment": stringProperty("Environment name to filter traces"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":       limitTraceSpansProperty(),
			"sort_order":  sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Limit       int    `json:"limit"`
		SortOrder   string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryTraces(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 6: query_trace_spans
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_trace_spans",
		Description: "Query all spans within a specific distributed trace in OpenChoreo. Returns span details including span ID, name, parent span, start/end times, and duration. Use the trace ID from query_traces results to drill into individual traces.",
		InputSchema: createSchema(map[string]any{
			"trace_id":    stringProperty("Trace ID to retrieve spans for (required). Obtained from query_traces results"),
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name"),
			"component":   stringProperty("Component name"),
			"environment": stringProperty("Environment name"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":       limitTraceSpansProperty(),
			"sort_order":  sortOrderProperty(),
		}, []string{"trace_id", "namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		TraceID     string `json:"trace_id"`
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Limit       int    `json:"limit"`
		SortOrder   string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryTraceSpans(ctx,
			args.TraceID,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 7: get_span_details
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_span_details",
		Description: "Get full details for a specific span within a trace in OpenChoreo. Returns complete span information including attributes, resource attributes, parent span ID, and timing details. Use trace_id and span_id from query_trace_spans results.",
		InputSchema: createSchema(map[string]any{
			"trace_id":    stringProperty("Trace ID containing the span (required)"),
			"span_id":     stringProperty("Span ID to retrieve details for (required)"),
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name"),
			"component":   stringProperty("Component name"),
			"environment": stringProperty("Environment name"),
		}, []string{"trace_id", "span_id", "namespace"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		TraceID     string `json:"trace_id"`
		SpanID      string `json:"span_id"`
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.GetSpanDetails(ctx,
			args.TraceID, args.SpanID,
			args.Namespace, args.Project, args.Component, args.Environment,
		)
		return handleToolResult(result, err)
	})

	// Tool 8: query_alerts
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_alerts",
		Description: "Query fired alerts in OpenChoreo. Supports filtering by project, component, environment, and time range. Useful for investigating recent alerts and details about them.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter alerts"),
			"component":   stringProperty("Component name to filter alerts"),
			"environment": stringProperty("Environment name to filter alerts (e.g., 'development', 'production')"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":       limitProperty(),
			"sort_order":  sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Limit       int    `json:"limit"`
		SortOrder   string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryAlerts(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 9: query_incidents
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_incidents",
		Description: "Query incidents in OpenChoreo. Supports filtering by project, component, environment, and time range. Useful for tracking incident lifecycle and response status. All incidents have an accompanying alert but not the other way around.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to filter incidents"),
			"component":   stringProperty("Component name to filter incidents"),
			"environment": stringProperty("Environment name to filter incidents (e.g., 'development', 'production')"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":       limitProperty(),
			"sort_order":  sortOrderProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		Environment string `json:"environment"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Limit       int    `json:"limit"`
		SortOrder   string `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryIncidents(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.StartTime, args.EndTime, args.Limit, args.SortOrder,
		)
		return handleToolResult(result, err)
	})

	// Tool 10: query_costs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_costs",
		Description: "Query infrastructure costs in OpenChoreo. Returns a flat list of per-component cost records (CPU cost, memory cost, and resource efficiency) for a namespace within the given environment and time range. By default the query covers every component in the namespace+environment; set 'project' to scope it to one project, or 'project'+'component' to scope it to a single component. Useful for cost attribution and spotting inefficient workloads.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"environment": stringProperty("Environment name (required, e.g., 'development', 'production')"),
			"project":     stringProperty("Project name. When set, narrows the query to components in this project"),
			"component":   stringProperty("Component name. When set, narrows the query to this single component. Requires project"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"granularity": stringProperty("Optional time bucket size in <count><unit> notation where unit is h (hours), d (days), or w (weeks), e.g. '1h', '2d', '3w'. Splits each component into one record per bucket"),
		}, []string{"namespace", "environment", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Environment string `json:"environment"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Granularity string `json:"granularity"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.QueryCosts(ctx,
			args.Namespace, args.Environment, args.Project, args.Component,
			args.StartTime, args.EndTime, args.Granularity,
		)
		return handleToolResult(result, err)
	})

	// Tool 11: query_recommendations
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_recommendations",
		Description: "Query right-sizing recommendations in OpenChoreo. Returns per-component recommendations comparing current CPU/memory requests and limits against recommended values derived from observed usage, with associated costs, for a namespace within the given environment and time range. By default the query covers every component in the namespace+environment; set 'project' to scope it to one project, or 'project'+'component' to scope it to a single component. Useful for reducing over-provisioning.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"environment": stringProperty("Environment name (required, e.g., 'development', 'production')"),
			"project":     stringProperty("Project name. When set, narrows the query to components in this project"),
			"component":   stringProperty("Component name. When set, narrows the query to this single component. Requires project"),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
		}, []string{"namespace", "environment", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string `json:"namespace"`
		Environment string `json:"environment"`
		Project     string `json:"project"`
		Component   string `json:"component"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.QueryRecommendations(ctx,
			args.Namespace, args.Environment, args.Project, args.Component,
			args.StartTime, args.EndTime,
		)
		return handleToolResult(result, err)
	})

	// Tool 12: query_dora_metrics
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name: "query_dora_metrics",
		Description: "Query DORA (DevOps Research and Assessment) delivery performance metrics for " +
			"OpenChoreo components: Deployment Frequency, Lead Time for Changes, Change Failure Rate, and " +
			"Mean Time to Recovery (MTTR). Returns a summary (value, Elite/High/Medium/Low classification, " +
			"trend vs the previous window of equal length) plus a time-bucketed series for each requested " +
			"metric. Use this to answer questions like 'how often do we deploy', 'what's our lead time for " +
			"changes', 'what's our change failure rate', or 'how long does it take to recover from failures'.",
		InputSchema: createSchema(map[string]any{
			"namespace":   stringProperty("Organization namespace (required)"),
			"project":     stringProperty("Project name to scope the metrics to"),
			"component":   stringProperty("Component name to scope the metrics to (requires project)"),
			"environment": stringProperty("Environment name to scope the metrics to (e.g., 'development', 'production')"),
			"granularity": granularityProperty(),
			"start_time":  stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":    stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"metrics":     doraMetricsProperty(),
		}, []string{"namespace", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		Namespace   string   `json:"namespace"`
		Project     string   `json:"project"`
		Component   string   `json:"component"`
		Environment string   `json:"environment"`
		Granularity string   `json:"granularity"`
		StartTime   string   `json:"start_time"`
		EndTime     string   `json:"end_time"`
		Metrics     []string `json:"metrics"`
	}) (*mcpsdk.CallToolResult, any, error) {
		if err := validateComponentScope(args.Namespace, args.Project, args.Component); err != nil {
			return nil, nil, err
		}
		result, err := handler.QueryDoraMetrics(ctx,
			args.Namespace, args.Project, args.Component, args.Environment,
			args.Granularity, args.StartTime, args.EndTime, args.Metrics,
		)
		return handleToolResult(result, err)
	})
}

// Helper functions for schema creation
func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func sortOrderProperty() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Sort order by timestamp: 'asc' (oldest first) or 'desc' (newest first). Default: 'desc'",
		"enum":        []string{"asc", "desc"},
	}
}

func limitLogsProperty() map[string]any {
	return map[string]any{
		"type":        "number",
		"description": "Maximum number of log entries to return. Default: 100",
	}
}

func limitTraceSpansProperty() map[string]any {
	return map[string]any{
		"type":        "number",
		"description": "Maximum number of entries to return. Default: 100",
	}
}

func limitProperty() map[string]any {
	return map[string]any{
		"type":        "number",
		"description": "Maximum number of entries to return. Default: 100",
	}
}

func granularityProperty() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Time bucket size for the series: 'daily', 'weekly', or 'monthly'. Default: 'daily'",
		"enum":        []string{"daily", "weekly", "monthly"},
	}
}

func doraMetricsProperty() map[string]any {
	return map[string]any{
		"type": "array",
		"description": "Which DORA metrics to compute: 'deploymentFrequency', 'leadTime', " +
			"'changeFailureRate', 'mttr'. Default: all four",
		"items": map[string]any{
			"type": "string",
			"enum": []string{"deploymentFrequency", "leadTime", "changeFailureRate", "mttr"},
		},
	}
}

func arrayProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
		},
	}
}

func createSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
