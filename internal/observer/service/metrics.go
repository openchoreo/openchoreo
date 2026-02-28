// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

var (
	// ErrMetricsInvalidRequest indicates that the request contains invalid parameters
	// (e.g., unparseable time values). Maps to HTTP 400 Bad Request.
	ErrMetricsInvalidRequest = errors.New("invalid metrics request")
	// ErrMetricsResolveSearchScope indicates a failure while resolving scope/resource identifiers.
	ErrMetricsResolveSearchScope = errors.New("metrics search scope resolution failed")
	// ErrMetricsRetrieval indicates a failure while retrieving metrics from the backend.
	ErrMetricsRetrieval = errors.New("metrics retrieval failed")
)

const (
	defaultMetricsStep = 5 * time.Minute
)

// MetricsService provides metrics querying functionality for the new API
type MetricsService struct {
	prometheusMetrics *prometheus.MetricsService
	resolver          *ResourceUIDResolver
	logger            *slog.Logger
}

// NewMetricsService creates a new MetricsService instance
func NewMetricsService(prometheusMetrics *prometheus.MetricsService, resolver *ResourceUIDResolver, logger *slog.Logger) (*MetricsService, error) {
	if prometheusMetrics == nil {
		return nil, fmt.Errorf("prometheus metrics service is required")
	}
	if resolver == nil {
		return nil, fmt.Errorf("resource UID resolver is required")
	}
	return &MetricsService{
		prometheusMetrics: prometheusMetrics,
		resolver:          resolver,
		logger:            logger,
	}, nil
}

// QueryMetrics queries metrics based on the provided request.
// The concrete return type is one of:
//   - *types.ResourceMetricsQueryResponse  (when req.Metric == "resource")
//   - *types.HTTPMetricsQueryResponse      (when req.Metric == "http")
func (s *MetricsService) QueryMetrics(ctx context.Context, req *types.MetricsQueryRequest) (interface{}, error) {
	if req == nil {
		return nil, fmt.Errorf("request must not be nil")
	}

	s.logger.Debug("QueryMetrics called",
		"metric", req.Metric,
		"startTime", req.StartTime,
		"endTime", req.EndTime)

	// Parse time parameters
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse startTime: %w", ErrMetricsInvalidRequest, err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse endTime: %w", ErrMetricsInvalidRequest, err)
	}

	// Determine step: use caller-supplied value, otherwise use the default.
	step := defaultMetricsStep
	if req.Step != nil && *req.Step != "" {
		parsed, err := time.ParseDuration(*req.Step)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid step format %q: %w", ErrMetricsInvalidRequest, *req.Step, err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("%w: step must be greater than 0", ErrMetricsInvalidRequest)
		}
		step = parsed
	}

	scope := &req.SearchScope

	// Resolve search scope names to UIDs
	var projectUID, componentUID, environmentUID string
	if scope.Project != "" {
		projectUID, err = s.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get project UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}
	if scope.Component != "" {
		componentUID, err = s.resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get component UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}
	if scope.Environment != "" {
		environmentUID, err = s.resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get environment UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}

	labelFilter := prometheus.BuildLabelFilterV1(scope.Namespace, componentUID, projectUID, environmentUID)
	scopeLabels := prometheus.BuildScopeLabelNamesV1(componentUID, projectUID, environmentUID)
	groupLeftClause := prometheus.BuildGroupLeftClauseV1(scopeLabels)

	switch req.Metric {
	case types.MetricTypeResource:
		// metricLabel is intentionally "" — results are aggregated across all containers
		// in the matched pods. Per-container breakdown is not exposed by this API.
		sumByClause := prometheus.BuildSumByClauseV1("", scopeLabels)
		return s.queryResourceMetrics(ctx, labelFilter, sumByClause, groupLeftClause, startTime, endTime, step)
	case types.MetricTypeHTTP:
		// metricLabel is intentionally "" — HTTP metrics are aggregated across all pods
		// in the matched scope. Per-pod breakdown is not exposed by this API.
		sumByClause := prometheus.BuildSumByClauseV1("", scopeLabels)
		return s.queryHTTPMetrics(ctx, labelFilter, sumByClause, groupLeftClause, startTime, endTime, step)
	default:
		return nil, fmt.Errorf("%w: unknown metric type %q", ErrMetricsRetrieval, req.Metric)
	}
}

// queryResourceMetrics runs all resource (CPU/memory) PromQL queries concurrently.
// If any sub-query fails, the error is returned and the partial result is discarded.
func (s *MetricsService) queryResourceMetrics(
	ctx context.Context,
	labelFilter string,
	sumByClause string,
	groupLeftClause string,
	startTime, endTime time.Time,
	step time.Duration,
) (*types.ResourceMetricsQueryResponse, error) {
	result := &types.ResourceMetricsQueryResponse{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	runQuery := func(queryFn func() string, assignFn func([]types.MetricsTimeSeriesItem), name string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := queryFn()
			s.logger.Debug("Resource metric query", "name", name, "query", query)
			resp, err := s.prometheusMetrics.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
			if err != nil {
				s.logger.Error("Failed to query resource metric", "metric", name, "error", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%w: sub-query %q failed: %w", ErrMetricsRetrieval, name, err)
				}
				mu.Unlock()
				return
			}
			if len(resp.Data.Result) > 1 {
				s.logger.Warn("Resource metric query returned multiple series, using aggregated result; queries should produce a single series",
					"metric", name, "seriesCount", len(resp.Data.Result))
			}
			if len(resp.Data.Result) > 0 {
				points := s.convertTimeValuePoints(prometheus.ConvertTimeSeriesToTimeValuePoints(resp.Data.Result[0]))
				mu.Lock()
				assignFn(points)
				mu.Unlock()
			}
		}()
	}

	runQuery(func() string {
		return prometheus.BuildCPUUsageQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.CPUUsage = p }, "cpuUsage")
	runQuery(func() string {
		return prometheus.BuildCPURequestsQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.CPURequests = p }, "cpuRequests")
	runQuery(func() string {
		return prometheus.BuildCPULimitsQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.CPULimits = p }, "cpuLimits")
	runQuery(func() string {
		return prometheus.BuildMemoryUsageQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.MemoryUsage = p }, "memoryUsage")
	runQuery(func() string {
		return prometheus.BuildMemoryRequestsQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.MemoryRequests = p }, "memoryRequests")
	runQuery(func() string {
		return prometheus.BuildMemoryLimitsQueryV1(labelFilter, sumByClause, groupLeftClause)
	}, func(p []types.MetricsTimeSeriesItem) { result.MemoryLimits = p }, "memoryLimits")

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

// queryHTTPMetrics runs all HTTP metrics PromQL queries concurrently.
// If any sub-query fails, the error is returned and the partial result is discarded.
func (s *MetricsService) queryHTTPMetrics(
	ctx context.Context,
	labelFilter string,
	sumByClause string,
	groupLeftClause string,
	startTime, endTime time.Time,
	step time.Duration,
) (*types.HTTPMetricsQueryResponse, error) {
	result := &types.HTTPMetricsQueryResponse{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	runQuery := func(queryFn func(string, string, string) string, assignFn func([]types.MetricsTimeSeriesItem), name string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := queryFn(labelFilter, sumByClause, groupLeftClause)
			s.logger.Debug("HTTP metric query", "name", name, "query", query)
			resp, err := s.prometheusMetrics.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
			if err != nil {
				s.logger.Error("Failed to query HTTP metric", "metric", name, "error", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%w: sub-query %q failed: %w", ErrMetricsRetrieval, name, err)
				}
				mu.Unlock()
				return
			}
			if len(resp.Data.Result) > 1 {
				s.logger.Warn("HTTP metric query returned multiple series, using aggregated result; queries should produce a single series",
					"metric", name, "seriesCount", len(resp.Data.Result))
			}
			if len(resp.Data.Result) > 0 {
				points := s.convertTimeValuePoints(prometheus.ConvertTimeSeriesToTimeValuePoints(resp.Data.Result[0]))
				mu.Lock()
				assignFn(points)
				mu.Unlock()
			}
		}()
	}

	runQuery(prometheus.BuildHTTPRequestCountQueryV1, func(p []types.MetricsTimeSeriesItem) { result.RequestCount = p }, "requestCount")
	runQuery(prometheus.BuildSuccessfulHTTPRequestCountQueryV1, func(p []types.MetricsTimeSeriesItem) { result.SuccessfulRequestCount = p }, "successfulRequestCount")
	runQuery(prometheus.BuildUnsuccessfulHTTPRequestCountQueryV1, func(p []types.MetricsTimeSeriesItem) { result.UnsuccessfulRequestCount = p }, "unsuccessfulRequestCount")
	runQuery(prometheus.BuildMeanHTTPRequestLatencyQueryV1, func(p []types.MetricsTimeSeriesItem) { result.MeanLatency = p }, "meanLatency")
	runQuery(prometheus.Build50thPercentileHTTPRequestLatencyQueryV1, func(p []types.MetricsTimeSeriesItem) { result.LatencyP50 = p }, "latencyP50")
	runQuery(prometheus.Build90thPercentileHTTPRequestLatencyQueryV1, func(p []types.MetricsTimeSeriesItem) { result.LatencyP90 = p }, "latencyP90")
	runQuery(prometheus.Build99thPercentileHTTPRequestLatencyQueryV1, func(p []types.MetricsTimeSeriesItem) { result.LatencyP99 = p }, "latencyP99")

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

// convertTimeValuePoints converts prometheus TimeValuePoints to internal MetricsTimeSeriesItems.
// Malformed timestamps are skipped with a warning rather than failing the entire response.
func (s *MetricsService) convertTimeValuePoints(points []prometheus.TimeValuePoint) []types.MetricsTimeSeriesItem {
	items := make([]types.MetricsTimeSeriesItem, 0, len(points))
	for _, p := range points {
		t, err := time.Parse(time.RFC3339, p.Time)
		if err != nil {
			s.logger.Warn("Skipping malformed timestamp in Prometheus response", "time", p.Time, "error", err)
			continue
		}
		items = append(items, types.MetricsTimeSeriesItem{
			Timestamp: t,
			Value:     p.Value,
		})
	}
	return items
}
