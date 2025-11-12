// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
)

// MetricsService provides metrics query functionality
type MetricsService struct {
	client *Client
	logger *slog.Logger
}

// ResourceMetricsTimeSeries represents resource usage metrics with time series data
type ResourceMetricsTimeSeries struct {
	CPUUsage       []TimeValuePoint `json:"cpuUsage"`
	CPURequests    []TimeValuePoint `json:"cpuRequests"`
	CPULimits      []TimeValuePoint `json:"cpuLimits"`
	Memory         []TimeValuePoint `json:"memory"`
	MemoryRequests []TimeValuePoint `json:"memoryRequests"`
	MemoryLimits   []TimeValuePoint `json:"memoryLimits"`
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

// QueryRangeTimeSeries executes a Prometheus range query and returns time series data
func (s *MetricsService) QueryRangeTimeSeries(ctx context.Context, query string, start, end time.Time, step time.Duration) (*TimeSeriesResponse, error) {
	return s.client.QueryRangeTimeSeries(ctx, query, start, end, step)
}

// Converts Kubernetes label names to Prometheus metric label names
// e.g., "component-name" becomes "label_component_name"
func prometheusLabelName(kubernetesLabel string) string {
	return "label_" + strings.ReplaceAll(kubernetesLabel, "-", "_")
}

// BuildLabelFilter builds a Prometheus label filter string for component identification
func BuildLabelFilter(componentID, projectID, environmentID string) string {
	componentLabel := prometheusLabelName(labels.ComponentID)
	projectLabel := prometheusLabelName(labels.ProjectID)
	environmentLabel := prometheusLabelName(labels.EnvironmentID)

	return fmt.Sprintf(`%s=%q,%s=%q,%s=%q`,
		componentLabel, componentID, projectLabel, projectID, environmentLabel, environmentID)
}

// BuildCPUUsageQuery builds a PromQL query for CPU usage rate
func BuildCPUUsageQuery(labelFilter string) string {
	query := fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name) (
    rate(container_cpu_usage_seconds_total{container="main"}[2m]) * on (pod) group_left (label_component_name, label_environment_name, label_project_name) kube_pod_labels{%s} )`, labelFilter)
	return query
}

// BuildMemoryUsageQuery builds a PromQL query for memory usage
func BuildMemoryUsageQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name) (
              container_memory_working_set_bytes{container="main"}
              * on (pod) group_left (label_component_name, label_environment_name, label_project_name)
                kube_pod_labels{%s}
            )`, labelFilter)
}

// BuildCPURequestsQuery PromQL query for CPU requests
func BuildCPURequestsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name, resource) (
            (
                kube_pod_container_resource_requests{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_component_name, label_environment_name, label_project_name)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildCPULimitsQuery builds a PromQL query for CPU limits
func BuildCPULimitsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name, resource) (
            (
                kube_pod_container_resource_limits{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_component_name, label_environment_name, label_project_name)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildMemoryRequestsQuery builds a PromQL query for memory requests
func BuildMemoryRequestsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name, resource) (
            (
                kube_pod_container_resource_requests{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_component_name, label_environment_name, label_project_name)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildMemoryLimitsQuery builds a PromQL query for memory limits
func BuildMemoryLimitsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_component_name, label_environment_name, label_project_name, resource) (
            (
                kube_pod_container_resource_limits{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_component_name, label_environment_name, label_project_name)
            kube_pod_labels{%s}
        )`, labelFilter)
}
