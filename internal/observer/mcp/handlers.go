// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

type MCPHandler struct {
	healthService        *service.HealthService
	logsService          service.LogsQuerier
	eventsService        service.EventsQuerier
	metricsService       service.MetricsQuerier
	alertIncidentService service.AlertIncidentService
	tracesService        service.TracesQuerier
	finopsService        service.FinOpsQuerier
	insightsService      service.InsightsService
	logger               *slog.Logger
}

func NewMCPHandler(
	healthService *service.HealthService,
	logsService service.LogsQuerier,
	eventsService service.EventsQuerier,
	metricsService service.MetricsQuerier,
	alertIncidentService service.AlertIncidentService,
	tracesService service.TracesQuerier,
	finopsService service.FinOpsQuerier,
	insightsService service.InsightsService,
	logger *slog.Logger,
) (*MCPHandler, error) {
	if healthService == nil {
		return nil, fmt.Errorf("missing healthService")
	}
	if logsService == nil {
		return nil, fmt.Errorf("missing logsService")
	}
	if eventsService == nil {
		return nil, fmt.Errorf("missing eventsService")
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
	if finopsService == nil {
		return nil, fmt.Errorf("missing finopsService")
	}
	if insightsService == nil {
		return nil, fmt.Errorf("missing insightsService")
	}
	if logger == nil {
		return nil, fmt.Errorf("missing logger")
	}
	return &MCPHandler{
		healthService:        healthService,
		logsService:          logsService,
		eventsService:        eventsService,
		metricsService:       metricsService,
		alertIncidentService: alertIncidentService,
		tracesService:        tracesService,
		finopsService:        finopsService,
		insightsService:      insightsService,
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

func (h *MCPHandler) QueryComponentEvents(ctx context.Context, namespace, project, component, environment,
	startTime, endTime string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, _ = setDefaults(limit, sortOrder, nil)
	req := &types.EventsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace:   namespace,
				Project:     project,
				Component:   component,
				Environment: environment,
			},
		},
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
		SortOrder: sortOrder,
	}
	return h.eventsService.QueryEvents(ctx, req)
}

func (h *MCPHandler) QueryWorkflowEvents(ctx context.Context, namespace, workflowRunName,
	startTime, endTime string, limit int, sortOrder string) (any, error) {
	limit, sortOrder, _ = setDefaults(limit, sortOrder, nil)
	req := &types.EventsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       namespace,
				WorkflowRunName: workflowRunName,
			},
		},
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
		SortOrder: sortOrder,
	}
	return h.eventsService.QueryEvents(ctx, req)
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

func (h *MCPHandler) GetSpanDetails(ctx context.Context, traceID, spanID,
	namespace, project, component, environment string) (any, error) {
	scope := types.ComponentSearchScope{
		Namespace:   namespace,
		Project:     project,
		Component:   component,
		Environment: environment,
	}
	return h.tracesService.QuerySpanDetails(ctx, traceID, spanID, scope)
}

func (h *MCPHandler) QueryAlerts(ctx context.Context, namespace, project, component, environment,
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
	sortOrderTyped := gen.AlertsQueryRequestSortOrder(sortOrder)
	req := gen.AlertsQueryRequest{
		StartTime: start,
		EndTime:   end,
		Limit:     &limit,
		SortOrder: &sortOrderTyped,
		SearchScope: gen.ComponentSearchScope{
			Namespace:   namespace,
			Project:     strPtr(project),
			Component:   strPtr(component),
			Environment: strPtr(environment),
		},
	}
	return h.alertIncidentService.QueryAlerts(ctx, req)
}

func (h *MCPHandler) QueryIncidents(ctx context.Context, namespace, project, component, environment,
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
	sortOrderTyped := gen.IncidentsQueryRequestSortOrder(sortOrder)
	req := gen.IncidentsQueryRequest{
		StartTime: start,
		EndTime:   end,
		Limit:     &limit,
		SortOrder: &sortOrderTyped,
		SearchScope: gen.ComponentSearchScope{
			Namespace:   namespace,
			Project:     strPtr(project),
			Component:   strPtr(component),
			Environment: strPtr(environment),
		},
	}
	return h.alertIncidentService.QueryIncidents(ctx, req)
}

func (h *MCPHandler) QueryCosts(ctx context.Context, namespace, environment, project, component,
	startTime, endTime, granularity string) (any, error) {
	if err := validateFinOpsScope(namespace, environment, project, component); err != nil {
		return nil, err
	}
	if err := validateGranularity(granularity); err != nil {
		return nil, err
	}
	req := &types.CostQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     project,
		Component:   component,
		StartTime:   startTime,
		EndTime:     endTime,
		Granularity: granularity,
	}
	return h.finopsService.GetComponentCosts(ctx, req)
}

func (h *MCPHandler) QueryRecommendations(ctx context.Context, namespace, environment, project, component,
	startTime, endTime string) (any, error) {
	if err := validateFinOpsScope(namespace, environment, project, component); err != nil {
		return nil, err
	}
	req := &types.RecommendationQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     project,
		Component:   component,
		StartTime:   startTime,
		EndTime:     endTime,
	}
	return h.finopsService.GetRecommendations(ctx, req)
}

func (h *MCPHandler) QueryDoraMetrics(ctx context.Context, namespace, project, component, environment,
	granularity, startTime, endTime string, metrics []string) (any, error) {
	start, err := parseRFC3339Time(startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %w", err)
	}
	end, err := parseRFC3339Time(endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time: %w", err)
	}

	req := gen.DoraMetricsQueryRequest{
		StartTime: start,
		EndTime:   end,
		SearchScope: gen.ComponentSearchScope{
			Namespace:   namespace,
			Project:     strPtr(project),
			Component:   strPtr(component),
			Environment: strPtr(environment),
		},
	}
	if granularity != "" {
		g := gen.DoraMetricsQueryRequestGranularity(granularity)
		req.Granularity = &g
	}
	if len(metrics) > 0 {
		typed := make([]gen.DoraMetricsQueryRequestMetrics, len(metrics))
		for i, m := range metrics {
			typed[i] = gen.DoraMetricsQueryRequestMetrics(m)
		}
		req.Metrics = &typed
	}

	return h.insightsService.QueryDoraMetrics(ctx, req)
}
