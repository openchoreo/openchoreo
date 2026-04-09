// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/agent"
	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

type baseParams struct {
	Namespace   string `json:"namespace"`
	Project     string `json:"project"`
	Component   string `json:"component"`
	Environment string `json:"environment"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
}

func (p *baseParams) parseTimeRange() (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, p.StartTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time: %w", err)
	}
	end, err := time.Parse(time.RFC3339, p.EndTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time: %w", err)
	}
	return start, end, nil
}

func (p *baseParams) buildScope() obsgen.ComponentSearchScope {
	scope := obsgen.ComponentSearchScope{Namespace: p.Namespace}
	if p.Project != "" {
		scope.Project = &p.Project
	}
	if p.Component != "" {
		scope.Component = &p.Component
	}
	if p.Environment != "" {
		scope.Environment = &p.Environment
	}
	return scope
}

// NewOPTools creates the observability-plane tools that call the Observer API
// using the generated client.
func NewOPTools(baseURL string, httpClient *http.Client) ([]agent.Tool, error) {
	client, err := obsgen.NewClient(baseURL, obsgen.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating observer client: %w", err)
	}

	return []agent.Tool{
		{
			Name:        "query_component_logs",
			Description: "Query runtime application logs for components deployed in OpenChoreo. Supports filtering by project, component, environment, time range, log levels, and search phrases.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "start_time", "end_time"},
				"properties": map[string]any{
					"namespace":     map[string]any{"type": "string", "description": "Namespace name"},
					"project":       map[string]any{"type": "string", "description": "Project name"},
					"component":     map[string]any{"type": "string", "description": "Component name"},
					"environment":   map[string]any{"type": "string", "description": "Environment name"},
					"start_time":    map[string]any{"type": "string", "description": "Start time in RFC3339 format"},
					"end_time":      map[string]any{"type": "string", "description": "End time in RFC3339 format"},
					"search_phrase": map[string]any{"type": "string", "description": "Text to search for in log messages"},
					"log_levels": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": []string{"ERROR", "WARN", "INFO", "DEBUG"}},
						"description": "Log levels to filter by",
					},
					"limit":      map[string]any{"type": "integer", "description": "Maximum number of results (default 100)"},
					"sort_order": map[string]any{"type": "string", "enum": []string{"asc", "desc"}, "description": "Sort order (default desc)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					baseParams
					SearchPhrase string   `json:"search_phrase"`
					LogLevels    []string `json:"log_levels"`
					Limit        int      `json:"limit"`
					SortOrder    string   `json:"sort_order"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}

				startTime, endTime, err := p.parseTimeRange()
				if err != nil {
					return "", err
				}

				var searchScope obsgen.LogsQueryRequest_SearchScope
				if err := searchScope.FromComponentSearchScope(p.buildScope()); err != nil {
					return "", fmt.Errorf("building search scope: %w", err)
				}

				req := obsgen.LogsQueryRequest{
					StartTime:   startTime,
					EndTime:     endTime,
					SearchScope: searchScope,
				}
				if p.SearchPhrase != "" {
					req.SearchPhrase = &p.SearchPhrase
				}
				if len(p.LogLevels) > 0 {
					levels := make([]obsgen.LogsQueryRequestLogLevels, len(p.LogLevels))
					for i, l := range p.LogLevels {
						levels[i] = obsgen.LogsQueryRequestLogLevels(l)
					}
					req.LogLevels = &levels
				}
				if p.Limit > 0 {
					req.Limit = &p.Limit
				}
				if p.SortOrder != "" {
					so := obsgen.LogsQueryRequestSortOrder(p.SortOrder)
					req.SortOrder = &so
				}

				resp, err := client.QueryLogs(ctx, req)
				if err != nil {
					return "", err
				}
				raw, err := readResponse(resp)
				if err != nil {
					return "", err
				}
				return formatLogs(raw), nil
			},
		},
		{
			Name:        "query_resource_metrics",
			Description: "Query CPU and memory resource usage metrics for components in OpenChoreo. Returns time-series data for CPU usage/requests/limits and memory usage/requests/limits. Useful for capacity planning, identifying resource constraints, and detecting memory leaks.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "start_time", "end_time"},
				"properties": map[string]any{
					"namespace":   map[string]any{"type": "string", "description": "Namespace name"},
					"project":     map[string]any{"type": "string", "description": "Project name"},
					"component":   map[string]any{"type": "string", "description": "Component name"},
					"environment": map[string]any{"type": "string", "description": "Environment name"},
					"start_time":  map[string]any{"type": "string", "description": "Start time in RFC3339 format"},
					"end_time":    map[string]any{"type": "string", "description": "End time in RFC3339 format"},
					"step":        map[string]any{"type": "string", "description": "Query step for point density (e.g. 1m, 5m, 15m, 1h)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					baseParams
					Step string `json:"step"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}

				startTime, endTime, err := p.parseTimeRange()
				if err != nil {
					return "", err
				}

				req := obsgen.MetricsQueryRequest{
					StartTime:   startTime,
					EndTime:     endTime,
					SearchScope: p.buildScope(),
					Metric:      obsgen.Resource,
				}
				if p.Step != "" {
					req.Step = &p.Step
				}

				resp, err := client.QueryMetrics(ctx, req)
				if err != nil {
					return "", err
				}
				raw, err := readResponse(resp)
				if err != nil {
					return "", err
				}
				return formatMetrics(raw), nil
			},
		},
		{
			Name:        "query_traces",
			Description: "Query distributed traces for components in OpenChoreo. Returns traces with summary information including trace ID, name, span count, root span details, and duration. Useful for understanding request flows across services and identifying performance bottlenecks.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "start_time", "end_time"},
				"properties": map[string]any{
					"namespace":   map[string]any{"type": "string", "description": "Namespace name"},
					"project":     map[string]any{"type": "string", "description": "Project name"},
					"component":   map[string]any{"type": "string", "description": "Component name"},
					"environment": map[string]any{"type": "string", "description": "Environment name"},
					"start_time":  map[string]any{"type": "string", "description": "Start time in RFC3339 format"},
					"end_time":    map[string]any{"type": "string", "description": "End time in RFC3339 format"},
					"limit":       map[string]any{"type": "integer", "description": "Maximum number of results (default 100)"},
					"sort_order":  map[string]any{"type": "string", "enum": []string{"asc", "desc"}, "description": "Sort order (default desc)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					baseParams
					Limit     int    `json:"limit"`
					SortOrder string `json:"sort_order"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}

				startTime, endTime, err := p.parseTimeRange()
				if err != nil {
					return "", err
				}

				req := obsgen.TracesQueryRequest{
					StartTime:   startTime,
					EndTime:     endTime,
					SearchScope: p.buildScope(),
				}
				if p.Limit > 0 {
					req.Limit = &p.Limit
				}
				if p.SortOrder != "" {
					so := obsgen.TracesQueryRequestSortOrder(p.SortOrder)
					req.SortOrder = &so
				}

				resp, err := client.QueryTraces(ctx, req)
				if err != nil {
					return "", err
				}
				raw, err := readResponse(resp)
				if err != nil {
					return "", err
				}
				return formatTraces(raw), nil
			},
		},
		{
			Name:        "query_trace_spans",
			Description: "Query all spans within a specific distributed trace. Returns span details including span ID, name, parent span, start/end times, and duration. Use the trace ID from query_traces results to drill into individual traces.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"trace_id", "namespace", "start_time", "end_time"},
				"properties": map[string]any{
					"trace_id":    map[string]any{"type": "string", "description": "Trace ID to query spans for"},
					"namespace":   map[string]any{"type": "string", "description": "Namespace name"},
					"project":     map[string]any{"type": "string", "description": "Project name"},
					"component":   map[string]any{"type": "string", "description": "Component name"},
					"environment": map[string]any{"type": "string", "description": "Environment name"},
					"start_time":  map[string]any{"type": "string", "description": "Start time in RFC3339 format"},
					"end_time":    map[string]any{"type": "string", "description": "End time in RFC3339 format"},
					"limit":       map[string]any{"type": "integer", "description": "Maximum number of results (default 100)"},
					"sort_order":  map[string]any{"type": "string", "enum": []string{"asc", "desc"}, "description": "Sort order (default desc)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					baseParams
					TraceID   string `json:"trace_id"`
					Limit     int    `json:"limit"`
					SortOrder string `json:"sort_order"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}

				startTime, endTime, err := p.parseTimeRange()
				if err != nil {
					return "", err
				}

				req := obsgen.TracesQueryRequest{
					StartTime:   startTime,
					EndTime:     endTime,
					SearchScope: p.buildScope(),
				}
				if p.Limit > 0 {
					req.Limit = &p.Limit
				}
				if p.SortOrder != "" {
					so := obsgen.TracesQueryRequestSortOrder(p.SortOrder)
					req.SortOrder = &so
				}

				resp, err := client.QuerySpansForTrace(ctx, p.TraceID, req)
				if err != nil {
					return "", err
				}
				raw, err := readResponse(resp)
				if err != nil {
					return "", err
				}
				return formatTraceSpans(raw), nil
			},
		},
	}, nil
}
