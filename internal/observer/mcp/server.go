// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

type Handler interface {
	GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (any, error)
	GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (any, error)
	GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (any, error)
	GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (any, error)
	GetTraces(ctx context.Context, params opensearch.TracesRequestParams) (any, error)
	GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error)
	GetComponentHTTPMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error)
}

// NewHTTPServer creates a new MCP HTTP server for the observer API
func NewHTTPServer(handler Handler) http.Handler {
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

// setDefaults applies default values for common query parameters
func setDefaults(limit int, sortOrder string, logLevels []string) (int, string, []string) {
	if limit == 0 {
		limit = 100
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if logLevels == nil {
		logLevels = []string{}
	}
	return limit, sortOrder, logLevels
}

func registerTools(s *mcpsdk.Server, handler Handler) {
	// Get Component Logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_component_logs",
		Description: "Retrieve logs from a specific component (deployable unit like a web service, API, worker, or scheduled task) in an OpenChoreo environment. Supports filtering by time range, log levels, search phrases, versions, and log type (application or build logs).",
		InputSchema: createSchema(map[string]any{
			"component_id":   defaultStringProperty(),
			"environment_id": defaultStringProperty(),
			"start_time":     stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":       stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"namespace":      stringProperty("Optional: Kubernetes namespace where the component is deployed"),
			"search_phrase":  stringProperty("Optional: Text to search within log messages"),
			"log_levels":     arrayProperty("Optional: Array of log levels to filter (e.g., ['ERROR', 'WARN']). Common values: ERROR, WARN, INFO, DEBUG, TRACE. Default: []"),
			"versions":       arrayProperty("Optional: Array of component version strings to filter (e.g., ['1.0.0', '1.0.1'])"),
			"version_ids":    arrayProperty("Optional: Array of internal version identifiers to filter"),
			"limit":          limitLogsProperty(),
			"sort_order":     sortOrderProperty(),
			"log_type":       stringProperty("Optional: Type of logs - 'application' for runtime logs or 'build' for build process logs. Default: 'application'"),
			"build_id":       stringProperty("Optional: Build identifier for retrieving build logs"),
			"build_uuid":     stringProperty("Optional: Build UUID for retrieving build logs"),
		}, []string{"component_id", "environment_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ComponentID   string   `json:"component_id"`
		EnvironmentID string   `json:"environment_id"`
		Namespace     string   `json:"namespace"`
		StartTime     string   `json:"start_time"`
		EndTime       string   `json:"end_time"`
		SearchPhrase  string   `json:"search_phrase"`
		LogLevels     []string `json:"log_levels"`
		Versions      []string `json:"versions"`
		VersionIDs    []string `json:"version_ids"`
		Limit         int      `json:"limit"`
		SortOrder     string   `json:"sort_order"`
		LogType       string   `json:"log_type"`
		BuildID       string   `json:"build_id"`
		BuildUUID     string   `json:"build_uuid"`
	}) (*mcpsdk.CallToolResult, any, error) {
		limit, sortOrder, logLevels := setDefaults(args.Limit, args.SortOrder, args.LogLevels)

		params := opensearch.ComponentQueryParams{
			QueryParams: opensearch.QueryParams{
				StartTime:     args.StartTime,
				EndTime:       args.EndTime,
				EnvironmentID: args.EnvironmentID,
				ComponentID:   args.ComponentID,
				Namespace:     args.Namespace,
				SearchPhrase:  args.SearchPhrase,
				LogLevels:     logLevels,
				Versions:      args.Versions,
				VersionIDs:    args.VersionIDs,
				Limit:         limit,
				SortOrder:     sortOrder,
				LogType:       args.LogType,
			},
			BuildID:   args.BuildID,
			BuildUUID: args.BuildUUID,
		}

		result, err := handler.GetComponentLogs(ctx, params)
		return handleToolResult(result, err)
	})

	// Get Project Logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_project_logs",
		Description: "Retrieve aggregated logs across all components within an OpenChoreo project. A project is a cloud-native application composed of multiple components (microservices). Useful for investigating issues that span multiple services or getting a holistic view of application behavior.",
		InputSchema: createSchema(map[string]any{
			"project_id":     defaultStringProperty(),
			"environment_id": defaultStringProperty(),
			"start_time":     stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":       stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"namespace":      stringProperty("Optional: Kubernetes namespace where the project is deployed"),
			"component_ids":  arrayProperty("Optional: Array of specific component IDs to filter. If omitted, retrieves logs from all components in the project"),
			"search_phrase":  stringProperty("Optional: Text to search within log messages across all components"),
			"log_levels":     arrayProperty("Optional: Array of log levels to filter (e.g., ['ERROR', 'WARN']). Common values: ERROR, WARN, INFO, DEBUG, TRACE. Default: []"),
			"versions":       arrayProperty("Optional: Array of component version strings to filter"),
			"version_ids":    arrayProperty("Optional: Array of internal version identifiers to filter"),
			"limit":          limitLogsProperty(),
			"sort_order":     sortOrderProperty(),
			"log_type":       stringProperty("Optional: Type of logs - 'application' for runtime logs or 'build' for build process logs. Default: 'application'"),
			"build_id":       stringProperty("Optional: Build identifier for retrieving build logs"),
			"build_uuid":     stringProperty("Optional: Build UUID for retrieving build logs"),
		}, []string{"project_id", "environment_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ProjectID     string   `json:"project_id"`
		EnvironmentID string   `json:"environment_id"`
		StartTime     string   `json:"start_time"`
		EndTime       string   `json:"end_time"`
		Namespace     string   `json:"namespace"`
		ComponentIDs  []string `json:"component_ids"`
		SearchPhrase  string   `json:"search_phrase"`
		LogLevels     []string `json:"log_levels"`
		Versions      []string `json:"versions"`
		VersionIDs    []string `json:"version_ids"`
		Limit         int      `json:"limit"`
		SortOrder     string   `json:"sort_order"`
		LogType       string   `json:"log_type"`
		BuildID       string   `json:"build_id"`
		BuildUUID     string   `json:"build_uuid"`
	}) (*mcpsdk.CallToolResult, any, error) {
		limit, sortOrder, logLevels := setDefaults(args.Limit, args.SortOrder, args.LogLevels)

		params := opensearch.QueryParams{
			StartTime:     args.StartTime,
			EndTime:       args.EndTime,
			EnvironmentID: args.EnvironmentID,
			ProjectID:     args.ProjectID,
			Namespace:     args.Namespace,
			SearchPhrase:  args.SearchPhrase,
			LogLevels:     logLevels,
			Versions:      args.Versions,
			VersionIDs:    args.VersionIDs,
			Limit:         limit,
			SortOrder:     sortOrder,
			LogType:       args.LogType,
		}

		result, err := handler.GetProjectLogs(ctx, params, args.ComponentIDs)
		return handleToolResult(result, err)
	})

	// Get Gateway Logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_gateway_logs",
		Description: "Retrieve logs from OpenChoreo's API gateways (Envoy-based ingress/egress) for an organization. Gateway logs capture HTTP traffic, routing decisions, rate limiting, and authentication events. Useful for investigating API performance, routing issues, or traffic patterns. These are infrastructure-layer logs, distinct from application logs.",
		InputSchema: createSchema(map[string]any{
			"organization_id":       defaultStringProperty(),
			"start_time":            stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":              stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"search_phrase":         stringProperty("Optional: Text to search within gateway logs (URLs, IPs, status codes, etc.)"),
			"api_id_to_version_map": objectProperty("Optional: Map of API IDs to version strings (e.g., {'api-123': 'v1.0'}). Filter logs for specific API versions"),
			"gateway_vhosts":        arrayProperty("Optional: Array of virtual host names (e.g., ['api.example.com']). Filter logs by hostname"),
			"limit":                 limitLogsProperty(),
			"sort_order":            sortOrderProperty(),
			"log_type":              stringProperty("Optional: Type of gateway logs"),
		}, []string{"organization_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		OrganizationID    string            `json:"organization_id"`
		StartTime         string            `json:"start_time"`
		EndTime           string            `json:"end_time"`
		SearchPhrase      string            `json:"search_phrase"`
		APIIDToVersionMap map[string]string `json:"api_id_to_version_map"`
		GatewayVHosts     []string          `json:"gateway_vhosts"`
		Limit             int               `json:"limit"`
		SortOrder         string            `json:"sort_order"`
		LogType           string            `json:"log_type"`
	}) (*mcpsdk.CallToolResult, any, error) {
		limit, sortOrder, _ := setDefaults(args.Limit, args.SortOrder, nil)

		params := opensearch.GatewayQueryParams{
			QueryParams: opensearch.QueryParams{
				StartTime:      args.StartTime,
				EndTime:        args.EndTime,
				SearchPhrase:   args.SearchPhrase,
				Limit:          limit,
				SortOrder:      sortOrder,
				LogType:        args.LogType,
				OrganizationID: args.OrganizationID,
			},
			APIIDToVersionMap: args.APIIDToVersionMap,
			GatewayVHosts:     args.GatewayVHosts,
		}

		result, err := handler.GetGatewayLogs(ctx, params)
		return handleToolResult(result, err)
	})

	// Get Organization Logs
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_organization_logs",
		Description: "Retrieve logs across an entire OpenChoreo organization with Kubernetes pod label filtering. An organization is the top-level grouping containing multiple projects. This advanced tool enables cross-project log analysis and infrastructure-level debugging using custom pod label selectors.",
		InputSchema: createSchema(map[string]any{
			"organization_id": defaultStringProperty(),
			"environment_id":  defaultStringProperty(),
			"start_time":      stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":        stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"namespace":       stringProperty("Optional: Kubernetes namespace to filter"),
			"pod_labels":      objectProperty("Optional: Map of Kubernetes pod labels to filter (e.g., {'app': 'backend', 'tier': 'database'}). Enables targeting specific workloads"),
			"search_phrase":   stringProperty("Optional: Text to search within log messages"),
			"log_levels":      arrayProperty("Optional: Array of log levels to filter (e.g., ['ERROR', 'WARN']). Common values: ERROR, WARN, INFO, DEBUG, TRACE. Default: []"),
			"versions":        arrayProperty("Optional: Array of component version strings to filter"),
			"version_ids":     arrayProperty("Optional: Array of internal version identifiers to filter"),
			"limit":           limitLogsProperty(),
			"sort_order":      sortOrderProperty(),
			"log_type":        stringProperty("Optional: Type of logs - 'application' for runtime logs or 'build' for build process logs. Default: 'application'"),
			"build_id":        stringProperty("Optional: Build identifier for retrieving build logs"),
			"build_uuid":      stringProperty("Optional: Build UUID for retrieving build logs"),
		}, []string{"organization_id", "environment_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		OrganizationID string            `json:"organization_id"`
		EnvironmentID  string            `json:"environment_id"`
		StartTime      string            `json:"start_time"`
		EndTime        string            `json:"end_time"`
		Namespace      string            `json:"namespace"`
		PodLabels      map[string]string `json:"pod_labels"`
		SearchPhrase   string            `json:"search_phrase"`
		LogLevels      []string          `json:"log_levels"`
		Versions       []string          `json:"versions"`
		VersionIDs     []string          `json:"version_ids"`
		Limit          int               `json:"limit"`
		SortOrder      string            `json:"sort_order"`
		LogType        string            `json:"log_type"`
		BuildID        string            `json:"build_id"`
		BuildUUID      string            `json:"build_uuid"`
	}) (*mcpsdk.CallToolResult, any, error) {
		limit, sortOrder, logLevels := setDefaults(args.Limit, args.SortOrder, args.LogLevels)

		params := opensearch.QueryParams{
			StartTime:      args.StartTime,
			EndTime:        args.EndTime,
			EnvironmentID:  args.EnvironmentID,
			OrganizationID: args.OrganizationID,
			Namespace:      args.Namespace,
			SearchPhrase:   args.SearchPhrase,
			LogLevels:      logLevels,
			Versions:       args.Versions,
			VersionIDs:     args.VersionIDs,
			Limit:          limit,
			SortOrder:      sortOrder,
			LogType:        args.LogType,
		}

		result, err := handler.GetOrganizationLogs(ctx, params, args.PodLabels)
		return handleToolResult(result, err)
	})

	// Get Traces
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_traces",
		Description: "Retrieve distributed tracing spans for a specific component/service in OpenChoreo. Traces capture the flow of requests across services, providing visibility into service interactions, latencies, and dependencies. Useful for investigating performance bottlenecks, debugging cross-service issues, and understanding request flows. Returns OpenTelemetry span data including trace IDs, span IDs, durations, and timestamps.",
		InputSchema: createSchema(map[string]any{
			"project_uid":     stringProperty("Required: Project UID to retrieve traces for"),
			"component_uids":  arrayProperty("Optional: Array of component UIDs to filter traces (e.g., ['8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b', '3f7b9e1a-4c6d-4e8f-a2b5-7d1c3e8f4a9b'])"),
			"environment_uid": stringProperty("Optional: Environment UID to filter traces (e.g. '2f5a8c1e-7d9b-4e3f-6a4c-8e1f2d7a9b5c', '6c8f1e4a-9d3b-4e7f-2a5c-8e4b1d3f9a7c')"),
			"trace_id":        stringProperty("Optional: Specific trace ID to retrieve (e.g. 'a372188b620ba2d5e159a35fc529ae12')"),
			"start_time":      stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":        stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
			"limit":           limitLogsProperty(),
			"sort_order":      sortOrderProperty(),
		}, []string{"project_uid", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ProjectUID     string   `json:"project_uid"`
		ComponentUIDs  []string `json:"component_uids"`
		EnvironmentUID string   `json:"environment_uid"`
		TraceID        string   `json:"trace_id"`
		StartTime      string   `json:"start_time"`
		EndTime        string   `json:"end_time"`
		Limit          int      `json:"limit"`
		SortOrder      string   `json:"sort_order"`
	}) (*mcpsdk.CallToolResult, any, error) {
		limit, sortOrder, _ := setDefaults(args.Limit, args.SortOrder, nil)

		params := opensearch.TracesRequestParams{
			ProjectUID:     args.ProjectUID,
			ComponentUIDs:  args.ComponentUIDs,
			EnvironmentUID: args.EnvironmentUID,
			TraceID:        args.TraceID,
			StartTime:      args.StartTime,
			EndTime:        args.EndTime,
			Limit:          limit,
			SortOrder:      sortOrder,
		}

		result, err := handler.GetTraces(ctx, params)
		return handleToolResult(result, err)
	})

	// Get Component Resource Metrics
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_component_resource_metrics",
		Description: "Retrieve time-series resource usage metrics (CPU and memory) for a component in OpenChoreo. Returns historical data points showing resource consumption, requests, and limits over time. Useful for capacity planning, identifying resource constraints, detecting memory leaks, and optimizing resource allocations. Metrics are sampled at 5-minute intervals and include both usage and configured limits/requests.",
		InputSchema: createSchema(map[string]any{
			"component_id":   stringProperty("Optional: Specific component ID to filter. If omitted, returns metrics for all components in the project"),
			"project_id":     defaultStringProperty(),
			"environment_id": defaultStringProperty(),
			"start_time":     stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":       stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
		}, []string{"project_id", "environment_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ComponentID   string `json:"component_id"`
		ProjectID     string `json:"project_id"`
		EnvironmentID string `json:"environment_id"`
		StartTime     string `json:"start_time"`
		EndTime       string `json:"end_time"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.GetComponentResourceMetrics(ctx, args.ComponentID, args.EnvironmentID, args.ProjectID, args.StartTime, args.EndTime)
		return handleToolResult(result, err)
	})

	// Get Component HTTP Metrics
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_component_http_metrics",
		Description: "Retrieve time-series HTTP metrics for a component in OpenChoreo. Returns historical data points showing request counts (total, successful, unsuccessful), latency metrics (mean, p50, p95, p99), and throughput over time. Useful for monitoring API performance, identifying bottlenecks, debugging HTTP errors, and understanding traffic patterns. Metrics are sampled at 5-minute intervals.",
		InputSchema: createSchema(map[string]any{
			"component_id":   stringProperty("Optional: Specific component ID to filter. If omitted, returns metrics for all components in the project"),
			"project_id":     defaultStringProperty(),
			"environment_id": defaultStringProperty(),
			"start_time":     stringProperty("Start of time range in RFC3339 format (e.g., 2025-11-04T08:29:02.452Z)"),
			"end_time":       stringProperty("End of time range in RFC3339 format (e.g., 2025-11-04T09:29:02.452Z)"),
		}, []string{"project_id", "environment_id", "start_time", "end_time"}),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args struct {
		ComponentID   string `json:"component_id"`
		ProjectID     string `json:"project_id"`
		EnvironmentID string `json:"environment_id"`
		StartTime     string `json:"start_time"`
		EndTime       string `json:"end_time"`
	}) (*mcpsdk.CallToolResult, any, error) {
		result, err := handler.GetComponentHTTPMetrics(ctx, args.ComponentID, args.EnvironmentID, args.ProjectID, args.StartTime, args.EndTime)
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

func defaultStringProperty() map[string]any {
	return map[string]any{
		"type": "string",
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

func arrayProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
		},
	}
}

func objectProperty(description string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
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
