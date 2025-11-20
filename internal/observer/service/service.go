// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
)

const (
	logLevelDebug = "debug"
)

// OpenSearchClient interface for testing
type OpenSearchClient interface {
	Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error)
	GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error)
	HealthCheck(ctx context.Context) error
}

// LoggingService provides logging and metrics functionality
type LoggingService struct {
	osClient       OpenSearchClient
	queryBuilder   *opensearch.QueryBuilder
	metricsService *prometheus.MetricsService
	config         *config.Config
	logger         *slog.Logger
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"totalCount"`
	Took       int                   `json:"tookMs"`
}

// HTTPMetricsTimeSeries represents HTTP metrics as time series data. This is what will be returned by the
// POST /api/metrics/component/http API
type HTTPMetricsTimeSeries struct {
	LatencyPercentile50th    []prometheus.TimeValuePoint `json:"latencyPercentile50th"`
	LatencyPercentile90th    []prometheus.TimeValuePoint `json:"latencyPercentile90th"`
	LatencyPercentile99th    []prometheus.TimeValuePoint `json:"latencyPercentile99th"`
	MeanLatency              []prometheus.TimeValuePoint `json:"meanLatency"`
	RequestCount             []prometheus.TimeValuePoint `json:"requestCount"`
	SuccessfulRequestCount   []prometheus.TimeValuePoint `json:"successfulRequestCount"`
	UnsuccessfulRequestCount []prometheus.TimeValuePoint `json:"unsuccessfulRequestCount"`
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(osClient OpenSearchClient, metricsService *prometheus.MetricsService, cfg *config.Config, logger *slog.Logger) *LoggingService {
	return &LoggingService{
		osClient:       osClient,
		queryBuilder:   opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		metricsService: metricsService,
		config:         cfg,
		logger:         logger,
	}
}

// GetComponentLogs retrieves logs for a specific component using V2 wildcard search
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting component logs",
		"component_id", params.ComponentID,
		"environment_id", params.EnvironmentID,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildComponentLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute component logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Component logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetProjectLogs retrieves logs for a specific project using V2 wildcard search
func (s *LoggingService) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	s.logger.Info("Getting project logs",
		"project_id", params.ProjectID,
		"environment_id", params.EnvironmentID,
		"component_ids", componentIDs,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildProjectLogsQuery(params, componentIDs)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute project logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Project logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetGatewayLogs retrieves gateway logs using V2 wildcard search
func (s *LoggingService) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting gateway logs",
		"organization_id", params.OrganizationID,
		"gateway_vhosts", params.GatewayVHosts,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildGatewayLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute gateway logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Gateway logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetOrganizationLogs retrieves logs for an organization with custom filters
func (s *LoggingService) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting organization logs",
		"organization_id", params.OrganizationID,
		"environment_id", params.EnvironmentID,
		"pod_labels", podLabels,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build organization-specific query
	query := s.queryBuilder.BuildOrganizationLogsQuery(params, podLabels)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute organization logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Organization logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

func (s *LoggingService) GetComponentTraces(ctx context.Context, params opensearch.ComponentTracesRequestParams) (*opensearch.TraceResponse, error) {
	s.logger.Info("Getting component traces",
		"serviceName", params.ServiceName)

	// Build component traces query
	query := s.queryBuilder.BuildComponentTracesQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, []string{"otel-v1-apm-span"}, query)
	if err != nil {
		s.logger.Error("Failed to execute component traces search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	traces := make([]opensearch.Span, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		span := opensearch.ParseSpanEntry(hit)
		traces = append(traces, span)
	}

	s.logger.Info("Component traces retrieved",
		"count", len(traces),
		"total", response.Hits.Total.Value)

	return &opensearch.TraceResponse{
		Spans:      traces,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// HealthCheck performs a health check on the service
func (s *LoggingService) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.osClient.HealthCheck(ctx); err != nil {
		s.logger.Error("Health check failed", "error", err)
		return fmt.Errorf("opensearch health check failed: %w", err)
	}

	s.logger.Debug("Health check passed")
	return nil
}

// GetComponentHTTPMetrics retrieves HTTP metrics for a component
func (s *LoggingService) GetComponentHTTPMetrics(ctx context.Context, componentID, environmentID, projectID string, startTime, endTime time.Time) (*HTTPMetricsTimeSeries, error) {
	s.logger.Debug("Getting resource metrics",
		"project", projectID,
		"component", componentID,
		"environment", environmentID,
		"start", startTime,
		"end", endTime)

	step := 5 * time.Minute
	metrics := &HTTPMetricsTimeSeries{}

	// Build component label filter using query builder
	labelFilter := prometheus.BuildLabelFilter(componentID, projectID, environmentID)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var queryErrors []error

	// Request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.RequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Successful request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildSuccessfulHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Successful request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query successful request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("successful request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.SuccessfulRequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Unsuccessful request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildUnsuccessfulHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Unsuccessful request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query unsuccessful request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("unsuccessful request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.UnsuccessfulRequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Mean latency
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildMeanHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Mean latency query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query mean latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("mean latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.MeanLatency = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 50th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build50thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 50th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 50th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("50th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile50th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 90th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build90thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 90th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 90th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("90th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile90th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 99th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build99thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 99th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 99th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("99th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile99th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Check if any errors occurred during metric queries
	if len(queryErrors) > 0 {
		s.logger.Error("Failed to fetch one or more HTTP metrics", "errors", queryErrors)
		return nil, fmt.Errorf("internal error occurred when fetching one or more HTTP metrics")
	}

	s.logger.Debug("HTTP metrics time series retrieved",
		"request_count", len(metrics.RequestCount),
		"successful_request_count", len(metrics.SuccessfulRequestCount),
		"unsuccessful_request_count", len(metrics.UnsuccessfulRequestCount),
		"mean_latency", len(metrics.MeanLatency),
		"latency_50th_points", len(metrics.LatencyPercentile50th),
		"latency_90th_points", len(metrics.LatencyPercentile90th),
		"latency_99th_points", len(metrics.LatencyPercentile99th),
	)

	return metrics, nil
}

// GetComponentResourceMetrics retrieves resource usage metrics for a component as time series
func (s *LoggingService) GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID string, startTime, endTime time.Time) (*prometheus.ResourceMetricsTimeSeries, error) {
	s.logger.Debug("Getting resource metrics",
		"project", projectID,
		"component", componentID,
		"environment", environmentID,
		"start", startTime,
		"end", endTime)

	// Define step interval for time series queries (5 minute intervals)
	step := 5 * time.Minute

	metrics := &prometheus.ResourceMetricsTimeSeries{}

	// Build component label filter using query builder
	labelFilter := prometheus.BuildLabelFilter(componentID, projectID, environmentID)

	// CPU usage
	cpuUsageQuery := prometheus.BuildCPUUsageQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU usage query:", cpuUsageQuery)
	}
	cpuResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuUsageQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU usage", "error", err)
	} else if len(cpuResp.Data.Result) > 0 {
		metrics.CPUUsage = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuResp.Data.Result[0])
	}

	// Memory usage
	memUsageQuery := prometheus.BuildMemoryUsageQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory usage query:", memUsageQuery)
	}
	memResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memUsageQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory usage", "error", err)
	} else if len(memResp.Data.Result) > 0 {
		metrics.Memory = prometheus.ConvertTimeSeriesToTimeValuePoints(memResp.Data.Result[0])
	}

	// CPU requests
	cpuRequestQuery := prometheus.BuildCPURequestsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU requests query:", cpuRequestQuery)
	}
	cpuRequestResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuRequestQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU requests", "error", err)
	} else if len(cpuRequestResp.Data.Result) > 0 {
		metrics.CPURequests = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuRequestResp.Data.Result[0])
	}

	// CPU limits
	cpuLimitQuery := prometheus.BuildCPULimitsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU limit query:", cpuLimitQuery)
	}
	cpuLimitResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuLimitQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU limits", "error", err)
	} else if len(cpuLimitResp.Data.Result) > 0 {
		metrics.CPULimits = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuLimitResp.Data.Result[0])
	}

	// Memory requests
	memRequestQuery := prometheus.BuildMemoryRequestsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory requests query:", memRequestQuery)
	}
	memRequestResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memRequestQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory requests", "error", err)
	} else if len(memRequestResp.Data.Result) > 0 {
		metrics.MemoryRequests = prometheus.ConvertTimeSeriesToTimeValuePoints(memRequestResp.Data.Result[0])
	}

	// Memory limits
	memLimitQuery := prometheus.BuildMemoryLimitsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory limit query:", memLimitQuery)
	}
	memLimitResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memLimitQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory limits", "error", err)
	} else if len(memLimitResp.Data.Result) > 0 {
		metrics.MemoryLimits = prometheus.ConvertTimeSeriesToTimeValuePoints(memLimitResp.Data.Result[0])
	}

	s.logger.Debug("Resource metrics time series retrieved",
		"cpu_usage_points", len(metrics.CPUUsage),
		"cpu_requests_points", len(metrics.CPURequests),
		"cpu_limits_points", len(metrics.CPULimits),
		"memory_points", len(metrics.Memory),
		"memory_requests_points", len(metrics.MemoryRequests),
		"memory_limits_points", len(metrics.MemoryLimits))

	return metrics, nil
}
