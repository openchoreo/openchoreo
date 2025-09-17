// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
)

// MetricsService provides metrics query functionality
type MetricsService struct {
	client *Client
	logger *slog.Logger
}

// HTTPMetrics represents HTTP performance metrics for a component
type HTTPMetrics struct {
	RequestCount        int            `json:"requestCount"`
	ResponseCount       int            `json:"responseCount"`
	AverageLatency      float64        `json:"averageLatencyMs"`
	ErrorRate           float64        `json:"errorRate"`
	StatusCodeBreakdown map[string]int `json:"statusCodeBreakdown"`
}

// ResourceMetrics represents resource usage metrics for a component
type ResourceMetrics struct {
	CPUUsage      float64 `json:"cpuUsage"`
	CPURequest    float64 `json:"cpuRequest"`
	CPULimit      float64 `json:"cpuLimit"`
	MemoryUsage   int64   `json:"memoryUsage"`
	MemoryRequest int64   `json:"memoryRequest"`
	MemoryLimit   int64   `json:"memoryLimit"`
}

// MetricsRequest represents a metrics query request
type MetricsRequest struct {
	ComponentID   string    `json:"componentId"`
	EnvironmentID string    `json:"environmentId"`
	ProjectID     string    `json:"projectId"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
}

// NewMetricsService creates a new metrics service
func NewMetricsService(client *Client, logger *slog.Logger) *MetricsService {
	return &MetricsService{
		client: client,
		logger: logger,
	}
}

// prometheusLabelName converts Kubernetes label names to Prometheus metric label names
// e.g., "component-name" becomes "label_component_name" 
func prometheusLabelName(kubernetesLabel string) string {
	return "label_" + strings.ReplaceAll(kubernetesLabel, "-", "_")
}

// GetHTTPMetrics retrieves HTTP performance metrics for a component
func (s *MetricsService) GetHTTPMetrics(ctx context.Context, req MetricsRequest) (*HTTPMetrics, error) {
	s.logger.Info("Getting HTTP metrics",
		"component_id", req.ComponentID,
		"project_id", req.ProjectID,
		"environment_id", req.EnvironmentID,
		"start_time", req.StartTime,
		"end_time", req.EndTime)

	metrics := &HTTPMetrics{
		StatusCodeBreakdown: make(map[string]int),
	}

	// Build the component-filtered HTTP query using label constants
	// Convert Kubernetes label names to Prometheus metric label names
	componentLabel := prometheusLabelName(labels.ComponentID)
	projectLabel := prometheusLabelName(labels.ProjectID) 
	environmentLabel := prometheusLabelName(labels.EnvironmentID)
	
	baseQuery := fmt.Sprintf(`hubble_http_requests_total * on(destination_workload) group_left(%s,%s,%s) (
		label_replace(
			kube_pod_info * on(namespace,pod) group_left(%s,%s,%s) kube_pod_labels{%s="%s",%s="%s",%s="%s"},
			"destination_workload", "$1", "created_by_name", "(.+)-[a-f0-9]{10}"
		)
	)`, componentLabel, projectLabel, environmentLabel,
		componentLabel, projectLabel, environmentLabel,
		componentLabel, req.ComponentID, projectLabel, req.ProjectID, environmentLabel, req.EnvironmentID)

	// Query for total request count
	requestQuery := fmt.Sprintf(`sum(%s)`, baseQuery)
	requestResp, err := s.client.Query(ctx, requestQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query request count: %w", err)
	}

	if len(requestResp.Data.Result) > 0 {
		if value, ok := requestResp.Data.Result[0].Value[1].(string); ok {
			if count, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.RequestCount = int(count)
			}
		}
	}

	// Query for status code breakdown
	statusQuery := fmt.Sprintf(`sum by (status) (%s)`, baseQuery)
	statusResp, err := s.client.Query(ctx, statusQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query status breakdown: %w", err)
	}

	totalResponses := 0
	errorResponses := 0
	for _, result := range statusResp.Data.Result {
		if value, ok := result.Value[1].(string); ok {
			if count, err := strconv.ParseFloat(value, 64); err == nil {
				status := result.Metric["status"]
				responseCount := int(count)
				metrics.StatusCodeBreakdown[status] = responseCount
				totalResponses += responseCount

				// Count 4xx and 5xx as errors
				if len(status) > 0 && (status[0] == '4' || status[0] == '5') {
					errorResponses += responseCount
				}
			}
		}
	}

	metrics.ResponseCount = totalResponses

	// Calculate error rate
	if totalResponses > 0 {
		metrics.ErrorRate = float64(errorResponses) / float64(totalResponses) * 100
	}

	// Query for request rate (more useful than raw count)
	rateQuery := fmt.Sprintf(`sum(rate((%s)[5m]))`, baseQuery)
	rateResp, err := s.client.Query(ctx, rateQuery)
	if err != nil {
		s.logger.Warn("Failed to query request rate", "error", err)
	} else if len(rateResp.Data.Result) > 0 {
		if value, ok := rateResp.Data.Result[0].Value[1].(string); ok {
			if rate, err := strconv.ParseFloat(value, 64); err == nil {
				// Convert rate per second to total for time window
				windowSeconds := req.EndTime.Sub(req.StartTime).Seconds()
				metrics.RequestCount = int(rate * windowSeconds)
			}
		}
	}

	s.logger.Info("HTTP metrics retrieved",
		"request_count", metrics.RequestCount,
		"response_count", metrics.ResponseCount,
		"error_rate", metrics.ErrorRate,
		"status_breakdown", metrics.StatusCodeBreakdown)

	return metrics, nil
}

// GetResourceMetrics retrieves resource usage metrics for a component
func (s *MetricsService) GetResourceMetrics(ctx context.Context, req MetricsRequest) (*ResourceMetrics, error) {
	s.logger.Info("Getting resource metrics",
		"component_id", req.ComponentID,
		"project_id", req.ProjectID,
		"environment_id", req.EnvironmentID)

	metrics := &ResourceMetrics{}

	// Build component label filter for resource queries using label constants
	componentLabel := prometheusLabelName(labels.ComponentID)
	projectLabel := prometheusLabelName(labels.ProjectID)
	environmentLabel := prometheusLabelName(labels.EnvironmentID)
	
	labelFilter := fmt.Sprintf(`kube_pod_labels{%s="%s",%s="%s",%s="%s"}`,
		componentLabel, req.ComponentID, projectLabel, req.ProjectID, environmentLabel, req.EnvironmentID)

	// Query CPU usage using label constants
	cpuUsageQuery := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total * on(namespace,pod) group_left(%s,%s,%s) %s[5m]))`, componentLabel, projectLabel, environmentLabel, labelFilter)
	cpuResp, err := s.client.Query(ctx, cpuUsageQuery)
	if err != nil {
		s.logger.Warn("Failed to query CPU usage", "error", err)
	} else if len(cpuResp.Data.Result) > 0 {
		if value, ok := cpuResp.Data.Result[0].Value[1].(string); ok {
			if cpu, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.CPUUsage = cpu
			}
		}
	}

	// Query Memory usage using label constants
	memUsageQuery := fmt.Sprintf(`sum(container_memory_usage_bytes * on(namespace,pod) group_left(%s,%s,%s) %s)`, componentLabel, projectLabel, environmentLabel, labelFilter)
	memResp, err := s.client.Query(ctx, memUsageQuery)
	if err != nil {
		s.logger.Warn("Failed to query memory usage", "error", err)
	} else if len(memResp.Data.Result) > 0 {
		if value, ok := memResp.Data.Result[0].Value[1].(string); ok {
			if mem, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.MemoryUsage = int64(mem)
			}
		}
	}

	// Query CPU requests using label constants
	cpuRequestQuery := fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="cpu"} * on(namespace,pod) group_left(%s,%s,%s) %s)`, componentLabel, projectLabel, environmentLabel, labelFilter)
	cpuRequestResp, err := s.client.Query(ctx, cpuRequestQuery)
	if err != nil {
		s.logger.Warn("Failed to query CPU requests", "error", err)
	} else if len(cpuRequestResp.Data.Result) > 0 {
		if value, ok := cpuRequestResp.Data.Result[0].Value[1].(string); ok {
			if cpu, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.CPURequest = cpu
			}
		}
	}

	// Query CPU limits using label constants
	cpuLimitQuery := fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu"} * on(namespace,pod) group_left(%s,%s,%s) %s)`, componentLabel, projectLabel, environmentLabel, labelFilter)
	cpuLimitResp, err := s.client.Query(ctx, cpuLimitQuery)
	if err != nil {
		s.logger.Warn("Failed to query CPU limits", "error", err)
	} else if len(cpuLimitResp.Data.Result) > 0 {
		if value, ok := cpuLimitResp.Data.Result[0].Value[1].(string); ok {
			if cpu, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.CPULimit = cpu
			}
		}
	}

	// Query Memory requests using label constants
	memRequestQuery := fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory"} * on(namespace,pod) group_left(%s,%s,%s) %s)`, componentLabel, projectLabel, environmentLabel, labelFilter)
	memRequestResp, err := s.client.Query(ctx, memRequestQuery)
	if err != nil {
		s.logger.Warn("Failed to query memory requests", "error", err)
	} else if len(memRequestResp.Data.Result) > 0 {
		if value, ok := memRequestResp.Data.Result[0].Value[1].(string); ok {
			if mem, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.MemoryRequest = int64(mem)
			}
		}
	}

	// Query Memory limits using label constants
	memLimitQuery := fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory"} * on(namespace,pod) group_left(%s,%s,%s) %s)`, componentLabel, projectLabel, environmentLabel, labelFilter)
	memLimitResp, err := s.client.Query(ctx, memLimitQuery)
	if err != nil {
		s.logger.Warn("Failed to query memory limits", "error", err)
	} else if len(memLimitResp.Data.Result) > 0 {
		if value, ok := memLimitResp.Data.Result[0].Value[1].(string); ok {
			if mem, err := strconv.ParseFloat(value, 64); err == nil {
				metrics.MemoryLimit = int64(mem)
			}
		}
	}

	s.logger.Info("Resource metrics retrieved",
		"cpu_usage", metrics.CPUUsage,
		"cpu_request", metrics.CPURequest,
		"cpu_limit", metrics.CPULimit,
		"memory_usage", metrics.MemoryUsage,
		"memory_request", metrics.MemoryRequest,
		"memory_limit", metrics.MemoryLimit)

	return metrics, nil
}
