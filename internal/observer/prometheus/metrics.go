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
	label := strings.ReplaceAll(kubernetesLabel, "-", "_")
	label = strings.ReplaceAll(label, ".", "_")
	label = strings.ReplaceAll(label, "/", "_")
	return "label_" + label
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
	query := fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container) (
    rate(container_cpu_usage_seconds_total{container!=""}[2m]) * on (pod) group_left (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) kube_pod_labels{%s} )`, labelFilter)
	return query
}

// BuildMemoryUsageQuery builds a PromQL query for memory usage
func BuildMemoryUsageQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container) (
              container_memory_working_set_bytes{container!=""}
              * on (pod) group_left (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
                kube_pod_labels{%s}
            )`, labelFilter)
}

// BuildCPURequestsQuery PromQL query for CPU requests
func BuildCPURequestsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource) (
            (
                kube_pod_container_resource_requests{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildCPULimitsQuery builds a PromQL query for CPU limits
func BuildCPULimitsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource) (
            (
                kube_pod_container_resource_limits{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildMemoryRequestsQuery builds a PromQL query for memory requests
func BuildMemoryRequestsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource) (
            (
                kube_pod_container_resource_requests{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// BuildMemoryLimitsQuery builds a PromQL query for memory limits
func BuildMemoryLimitsQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource) (
            (
                kube_pod_container_resource_limits{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) GROUP_LEFT (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
            kube_pod_labels{%s}
        )`, labelFilter)
}

// ----------------------------
// HTTP REQUEST METRICS QUERIES
// ----------------------------

// BuildHTTPRequestCountQuery builds a PromQL query for HTTP request count
func BuildHTTPRequestCountQuery(labelFilter string) string {
	return fmt.Sprintf(`
	    rate(hubble_http_requests_total{reporter="client"}[2m])
            * on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
            label_replace(
	            kube_pod_labels{%s},
    			"destination_pod",
	    		"$1",
		    	"pod",
			    "(.*)"
		    ) 
			>= 0
	`, labelFilter)
}

// BuildSuccessfulHTTPRequestCountQuery builds a PromQL query for successful HTTP request count. Requests are
// considered successful if they have a HTTP 1xx, 2xx or 3xx status code.
func BuildSuccessfulHTTPRequestCountQuery(labelFilter string) string {
	return fmt.Sprintf(`
	    rate(hubble_http_requests_total{reporter="client", status=~"^[123]..?$"}[2m])
            * on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
            label_replace(
    	        kube_pod_labels{%s},
	    		"destination_pod",
		    	"$1",
			    "pod",
			    "(.*)"
		    )
		    >= 0
	`, labelFilter)
}

// BuildUnsuccessfulHTTPRequestCountQuery builds a PromQL query for unsuccessful HTTP request count. Requests are
// considered unsuccessful if they have a 4xx or 5xx status code.
func BuildUnsuccessfulHTTPRequestCountQuery(labelFilter string) string {
	return fmt.Sprintf(`
	    rate(hubble_http_requests_total{reporter="client", status=~"^[45]..?$"}[2m])
            * on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
            label_replace(
	            kube_pod_labels{%s},
    			"destination_pod",
	    		"$1",
		    	"pod",
			    "(.*)"
		    )
			>= 0
	`, labelFilter)
}

// BuildMeanHTTPRequestLatencyQuery builds a PromQL query for mean HTTP request latency
func BuildMeanHTTPRequestLatencyQuery(labelFilter string) string {
	return fmt.Sprintf(`
		(
		    sum by(destination_pod) (rate(hubble_http_request_duration_seconds_sum{reporter="client"}[2m]))
		    /
		    sum by(destination_pod) (rate(hubble_http_requests_total{reporter="client"}[2m]))
		)
		* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
	    	label_replace(
		    	kube_pod_labels{%s},
			    "destination_pod",
	    		"$1",
		    	"pod",
			    "(.*)"
		    )
			>= 0
	`, labelFilter)
}

// Build50thPercentileHTTPRequestLatencyQuery builds a PromQL query for 50th percentile HTTP request latency
func Build50thPercentileHTTPRequestLatencyQuery(labelFilter string) string {
	return fmt.Sprintf(`
		histogram_quantile(
			0.5,
			sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
				rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
				    * on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
				    label_replace(
					    kube_pod_labels{%s},
					    "destination_pod",
						"$1",
						"pod",
						"(.*)"
				    )
			)
		)
		>= 0
	`, labelFilter)
}

// Build90thPercentileHTTPRequestLatencyQuery builds a PromQL query for 90th percentile HTTP request latency
func Build90thPercentileHTTPRequestLatencyQuery(labelFilter string) string {
	return fmt.Sprintf(`
	    histogram_quantile(
            0.9,
			sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
			    rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
    				* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
	    			label_replace(
		    			kube_pod_labels{%s},
			    		"destination_pod",
				    	"$1",
					    "pod",
					    "(.*)"
				    )
            )
        )
		>= 0
	`, labelFilter)
}

// Build99thPercentileHTTPRequestLatencyQuery builds a PromQL query for 99th percentile HTTP request latency
func Build99thPercentileHTTPRequestLatencyQuery(labelFilter string) string {
	return fmt.Sprintf(`
	    histogram_quantile(
            0.99,
			sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
			    rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
				    * on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) 
    				label_replace(
	    				kube_pod_labels{%s},
		    			"destination_pod",
			    		"$1",
				    	"pod",
					    "(.*)"
				    )
            )
        )
		>= 0
	`, labelFilter)
}
