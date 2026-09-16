// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/observer/api/handlers"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
)

// NewHTTPServer creates a new MCP HTTP server for the observer API.
//
// auditOpts.Emitter should be the same *audit.Emitter the REST chain uses and
// its Bindings should come from observeraudit.MCPBindings(), so one policy and
// one operation table cover both surfaces.
//
// This is the only constructor that installs audit; NewServer registers tools
// and nothing else. Serving a *mcpsdk.Server built any other way leaves
// query_audit_logs reading the trail without appending to it.
func NewHTTPServer(handler *MCPHandler, auditOpts mcpaudit.MiddlewareOptions) (http.Handler, error) {
	server := NewServer(handler)

	auditMw, err := mcpaudit.NewMiddleware(auditOpts)
	if err != nil {
		return nil, fmt.Errorf("create MCP audit middleware: %w", err)
	}
	server.AddReceivingMiddleware(auditMw)

	return mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return server
	}, nil), nil
}

// NewServer creates the MCP server with every observer tool registered, and no
// middleware. See NewHTTPServer, which adds audit.
//
// Exported so the audit coverage gate can enumerate the registered tools over
// the protocol rather than trusting a hand-maintained list — the SDK offers no
// way to read tools back off a *Server directly.
func NewServer(handler *MCPHandler) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "openchoreo-observer",
		Version: "1.0.0",
	}, nil)

	registerTools(server, handler)

	return server
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

	// Tool: query_platform_logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name: "query_platform_logs",
		Description: "Query logs from OpenChoreo's own platform components - the control plane " +
			"(controller-manager, openchoreo-api, cluster-gateway), the data plane agents and gateways, " +
			"and the workflow and observability plane infrastructure. This is the operator's view of the " +
			"platform itself, not of user workloads: it is addressed by raw Kubernetes coordinates rather " +
			"than by project, component and environment, so use query_component_logs for a deployed " +
			"component's runtime logs. Multi-value filters match any of their values, and different " +
			"filters must all match. Requires the cluster-scoped 'platformlogs:view' permission.",
		InputSchema: createSchema(map[string]any{
			"cluster_instance": arrayProperty(
				"Clusters the logs were collected from, as named on each cluster's logs collector (e.g. ['cluster1'])"),
			"kubernetes_namespace": arrayProperty(
				"Kubernetes namespaces of the pods (e.g. ['openchoreo-control-plane']). " +
					"This is a Kubernetes namespace, not the OpenChoreo organization namespace other tools take"),
			"pod_name":       arrayProperty("Pod names (e.g. ['controller-manager-7f58b689b5-pwsb5'])"),
			"container_name": arrayProperty("Container names within the pods (e.g. ['manager'])"),
			"labels": stringProperty(
				"Kubernetes label selector over the pod labels, as 'key=value' pairs joined by commas, " +
					"where a comma means AND (the syntax 'kubectl -l' accepts). This is how a plane is selected: " +
					"'openchoreo.dev/plane=controlplane' (or dataplane, workflowplane, observabilityplane), " +
					"narrowed further with 'openchoreo.dev/plane-id=<planeID>'. Components OpenChoreo does not " +
					"ship carry no plane label"),
			"start_time":    stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":      stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"search_phrase": stringProperty("Text to search within log messages"),
			"log_levels":    arrayProperty("Log levels to filter (e.g., ['ERROR', 'WARN', 'INFO', 'DEBUG']). Default: all levels"),
			"limit":         limitLogsProperty(),
			"sort_order":    sortOrderProperty(),
			"include_sources": enumArrayProperty(
				"Also return a breakdown of which coordinates produced the matching logs, as the distinct "+
					"values each named coordinate takes with a count for each, ordered by count descending. "+
					"Use it to find where a problem is concentrated - include_sources ['pod_name'] with "+
					"log_levels ['ERROR'] answers which pods are erroring and how much. To get only the "+
					"breakdown, set limit to 1 so the log entries themselves cost nothing",
				platformLogSourceFieldNames),
			"max_sources": maxSourcesProperty(),
		}, []string{"start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ClusterInstance     []string `json:"cluster_instance"`
		KubernetesNamespace []string `json:"kubernetes_namespace"`
		PodName             []string `json:"pod_name"`
		ContainerName       []string `json:"container_name"`
		Labels              string   `json:"labels"`
		StartTime           string   `json:"start_time"`
		EndTime             string   `json:"end_time"`
		SearchPhrase        string   `json:"search_phrase"`
		LogLevels           []string `json:"log_levels"`
		Limit               int      `json:"limit"`
		SortOrder           string   `json:"sort_order"`
		IncludeSources      []string `json:"include_sources"`
		MaxSources          int      `json:"max_sources"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.QueryPlatformLogs(ctx,
			args.ClusterInstance, args.KubernetesNamespace, args.PodName, args.ContainerName,
			args.Labels, args.StartTime, args.EndTime, args.SearchPhrase,
			args.LogLevels, args.Limit, args.SortOrder,
			args.IncludeSources, args.MaxSources,
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
			"trace_id": stringProperty("Trace ID containing the span (required)"),
			"span_id":  stringProperty("Span ID to retrieve details for (required)"),
		}, []string{"trace_id", "span_id"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		TraceID string `json:"trace_id"`
		SpanID  string `json:"span_id"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.GetSpanDetails(ctx, args.TraceID, args.SpanID)
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

	// Tool: query_audit_logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name: "query_audit_logs",
		Description: "Query OpenChoreo's audit trail: who did what, to which resource, and whether it " +
			"succeeded. Use it for questions about people and authority - who deleted a component, " +
			"what one subject did in a single login, what was denied to whom. For application or " +
			"platform output use query_component_logs or query_platform_logs instead. " +
			"Filter semantics: values within one filter are OR-ed, separate filters are AND-ed, and an " +
			"omitted filter constrains nothing - so actor_id ['alice','bob'] with result ['denied'] " +
			"means (alice OR bob) AND denied. " +
			"Requires the cluster-scoped 'auditlogs:view' permission, evaluated before any filter is " +
			"read, so resource_namespace narrows the results but grants no access. Reading the trail " +
			"is itself audited.",
		InputSchema: createSchema(map[string]any{
			"start_time": stringProperty(
				"Inclusive start of the event window, RFC3339 (e.g. 2026-08-14T16:30:00Z). " +
					"At most 366 days wide; page a longer investigation a year at a time"),
			"end_time": stringProperty(
				"Exclusive end of the event window, RFC3339, strictly after start_time"),

			"actor_id": arrayProperty(
				"Subject identifiers (e.g. ['alice@example.com']). Unique only within an issuer, so " +
					"pair with actor_issuer where more than one identity provider is configured"),
			"actor_type":   arrayProperty("Kinds of subject, e.g. ['user', 'service_account', 'anonymous']"),
			"actor_issuer": arrayProperty("Token issuers - the namespace an actor_id is unique within"),
			"actor_session_id": arrayProperty(
				"Session IDs from the token's 'sid' claim, joining every action taken in one login. " +
					"Absent for client-credentials tokens, so this selects human activity only"),
			"actor_entitlements": arrayProperty(
				"Entitlement values such as a group name (e.g. ['platform-engineer']). Matched across " +
					"every claim in the entitlements map, not one named claim"),

			"resource_type":      arrayProperty("Resource kinds the action targeted (e.g. ['project'])"),
			"resource_namespace": arrayProperty("OpenChoreo namespaces (e.g. ['default'])"),
			"resource_environment": arrayProperty(
				"Environments in '{namespace}/{name}' form (e.g. ['default/development']). " +
					"A bare environment name will not match"),
			"resource_project":   arrayProperty("Projects"),
			"resource_component": arrayProperty("Components"),
			"resource_resource":  arrayProperty("Resources, the hierarchy level that is a sibling of component"),
			"resource_name":      arrayProperty("Resource names"),

			"action": arrayProperty("Semantic action names (e.g. ['create_project', 'delete_component'])"),
			"category": enumArrayProperty(
				"Event categories. 'access' is disclosure rather than change; reading the trail is "+
					"recorded under it",
				handlers.AuditLogCategoryValues()),
			"result": enumArrayProperty(
				"Outcomes. 'denied' is a subject the policy refused, 'unauthenticated' a call with no "+
					"usable identity, 'failure' an error",
				handlers.AuditLogResultValues()),
			"producer": arrayProperty("Emitting services (e.g. ['openchoreo-api'])"),
			"surface": enumArrayProperty(
				"Surfaces the call arrived through. MCP wraps the same API, so the REST value is 'rest'",
				handlers.AuditLogSurfaceValues()),
			"operation_id": arrayProperty("Canonical operation identifiers (e.g. ['CreateProject'])"),
			"request_id": arrayProperty(
				"Correlation IDs, matched exactly. The pivot from an access log line to its audit record"),
			"event_id": arrayProperty("Record identifiers, for fetching known records directly"),
			"source_ip": arrayProperty(
				"Client addresses, matched exactly rather than by network range. Behind a proxy the " +
					"recorded value is the proxy"),
			"user_agent": arrayProperty(
				"Client identifications, matched exactly. Agent strings vary by version, so " +
					"search_phrase is usually the better tool for 'anything occ'"),

			"search_phrase": stringProperty("Free text to match within the record"),
			"limit":         auditLimitProperty(),
			"sort_order":    sortOrderProperty(),

			"include_timeline": booleanProperty(
				"Also return per-interval counts across the whole window, broken down by result. " +
					"A page of records cannot be bucketed into a histogram, so ask for this to see when " +
					"activity happened. Set limit to 1 for the shape alone"),
			"timeline_interval": stringProperty(
				"Bucket width, <count><unit> where unit is m, h, d or w (e.g. '15m'). Ignored unless " +
					"include_timeline is true. A width exceeding 500 buckets is coarsened, so read the " +
					"width actually used from timeline.interval"),
		}, []string{"start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args AuditLogsQueryArgs) (
		*mcpsdk.CallToolResult, any, error,
	) {
		result, err := handler.QueryAuditLogs(ctx, args)
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

// auditLimitProperty declares the cap as well as the default, unlike
// limitProperty: exceeding it is an error rather than a clamp, so a caller that
// cannot see the ceiling only learns it from a failed call.
func auditLimitProperty() map[string]any {
	return map[string]any{
		"type": "number",
		"description": fmt.Sprintf(
			"Maximum number of records to return. Default: 100, maximum: %d. Page a larger "+
				"investigation by narrowing the window rather than raising this",
			config.MaxLimit),
		"maximum": config.MaxLimit,
	}
}

func maxSourcesProperty() map[string]any {
	return map[string]any{
		"type": "number",
		"description": fmt.Sprintf(
			"Maximum number of values to return per coordinate in 'sources'. Default: %d", defaultMaxSources),
	}
}

func enumArrayProperty(description string, values []string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
			"enum": values,
		},
	}
}

func booleanProperty(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
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
