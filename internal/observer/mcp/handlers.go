// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

type MCPHandler struct {
	healthService        *service.HealthService
	logsService          service.LogsQuerier
	metricsService       service.MetricsQuerier
	alertIncidentService service.AlertIncidentService
	tracesService        service.TracesQuerier
	logger               *slog.Logger
}

func NewMCPHandler(
	healthService *service.HealthService,
	logsService service.LogsQuerier,
	metricsService service.MetricsQuerier,
	alertIncidentService service.AlertIncidentService,
	tracesService service.TracesQuerier,
	logger *slog.Logger,
) (*MCPHandler, error) {
	if healthService == nil {
		return nil, fmt.Errorf("missing healthService")
	}
	if logsService == nil {
		return nil, fmt.Errorf("missing logsService")
	}
	if metricsService == nil {
		return nil, fmt.Errorf("missing metricsService")
	}
	if alertIncidentService == nil {
		return nil, fmt.Errorf("missing alertIncidentService")
	}
	if tracesService == nil {
		return nil, fmt.Errorf("missing tracesService")
	}
	if logger == nil {
		return nil, fmt.Errorf("missing logger")
	}
	return &MCPHandler{
		healthService:        healthService,
		logsService:          logsService,
		metricsService:       metricsService,
		alertIncidentService: alertIncidentService,
		tracesService:        tracesService,
		logger:               logger,
	}, nil
}

func (h *MCPHandler) QueryComponentLogs(ctx context.Context, namespace, project, component, environment,
	startTime, endTime, searchPhrase string, logLevels []string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, logLevels = setDefaults(limit, sortOrder, logLevels)
	req := &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace:   namespace,
				Project:     project,
				Component:   component,
				Environment: environment,
			},
		},
		StartTime:    startTime,
		EndTime:      endTime,
		SearchPhrase: searchPhrase,
		LogLevels:    logLevels,
		Limit:        limit,
		SortOrder:    sortOrder,
	}
	return h.logsService.QueryLogs(ctx, req)
}

func (h *MCPHandler) QueryWorkflowLogs(ctx context.Context, namespace, workflowRunName, taskName,
	startTime, endTime, searchPhrase string, logLevels []string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, logLevels = setDefaults(limit, sortOrder, logLevels)
	req := &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       namespace,
				WorkflowRunName: workflowRunName,
				TaskName:        taskName,
			},
		},
		StartTime:    startTime,
		EndTime:      endTime,
		SearchPhrase: searchPhrase,
		LogLevels:    logLevels,
		Limit:        limit,
		SortOrder:    sortOrder,
	}
	return h.logsService.QueryLogs(ctx, req)
}

func (h *MCPHandler) QueryResourceMetrics(ctx context.Context, namespace, project, component, environment,
	startTime, endTime string, step *string) (any, error) {
	req := &types.MetricsQueryRequest{
		Metric:    types.MetricTypeResource,
		StartTime: startTime,
		EndTime:   endTime,
		Step:      step,
		SearchScope: types.ComponentSearchScope{
			Namespace:   namespace,
			Project:     project,
			Component:   component,
			Environment: environment,
		},
	}
	return h.metricsService.QueryMetrics(ctx, req)
}

func (h *MCPHandler) QueryHTTPMetrics(ctx context.Context, namespace, project, component, environment,
	startTime, endTime string, step *string) (any, error) {
	req := &types.MetricsQueryRequest{
		Metric:    types.MetricTypeHTTP,
		StartTime: startTime,
		EndTime:   endTime,
		Step:      step,
		SearchScope: types.ComponentSearchScope{
			Namespace:   namespace,
			Project:     project,
			Component:   component,
			Environment: environment,
		},
	}
	return h.metricsService.QueryMetrics(ctx, req)
}

func (h *MCPHandler) QueryTraces(ctx context.Context, namespace, project, component, environment,
	startTime, endTime string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, _ = setDefaults(limit, sortOrder, nil)
	start, err := parseRFC3339Time(startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %w", err)
	}
	end, err := parseRFC3339Time(endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time: %w", err)
	}
	req := &types.TracesQueryRequest{
		StartTime: start,
		EndTime:   end,
		Limit:     limit,
		SortOrder: sortOrder,
		SearchScope: types.ComponentSearchScope{
			Namespace:   namespace,
			Project:     project,
			Component:   component,
			Environment: environment,
		},
	}
	return h.tracesService.QueryTraces(ctx, req)
}

func (h *MCPHandler) QueryTraceSpans(ctx context.Context, traceID, namespace, project, component, environment,
	startTime, endTime string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, _ = setDefaults(limit, sortOrder, nil)
	start, err := parseRFC3339Time(startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %w", err)
	}
	end, err := parseRFC3339Time(endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time: %w", err)
	}
	req := &types.TracesQueryRequest{
		StartTime: start,
		EndTime:   end,
		Limit:     limit,
		SortOrder: sortOrder,
		SearchScope: types.ComponentSearchScope{
			Namespace:   namespace,
			Project:     project,
			Component:   component,
			Environment: environment,
		},
	}
	return h.tracesService.QuerySpans(ctx, traceID, req)
}

func (h *MCPHandler) GetSpanDetails(ctx context.Context, traceID, spanID string) (any, error) {
	return h.tracesService.GetSpanDetails(ctx, traceID, spanID)
}

func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return t, nil
}
