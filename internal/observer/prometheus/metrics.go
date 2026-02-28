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

// BuildLabelFilterV1 builds a Prometheus label filter string. If any of the IDs are empty, they are not included in the filter.
func BuildLabelFilterV1(namespaceName, componentID, projectID, environmentID string) string {
	namespaceLabel := prometheusLabelName(labels.NamespaceName)
	componentLabel := prometheusLabelName(labels.ComponentID)
	projectLabel := prometheusLabelName(labels.ProjectID)
	environmentLabel := prometheusLabelName(labels.EnvironmentID)

	labelFilter := fmt.Sprintf("%s=%q", namespaceLabel, namespaceName)
	if componentID != "" {
		labelFilter = fmt.Sprintf("%s,%s=%q", labelFilter, componentLabel, componentID)
	}
	if projectID != "" {
		labelFilter = fmt.Sprintf("%s,%s=%q", labelFilter, projectLabel, projectID)
	}
	if environmentID != "" {
		labelFilter = fmt.Sprintf("%s,%s=%q", labelFilter, environmentLabel, environmentID)
	}
	return labelFilter
}

// BuildScopeLabelNamesV1 returns the Prometheus label names for whichever of
// componentID, projectID, and environmentID are non-empty. These are used as
// the "by" dimensions in sum/group_left clauses for V1 queries.
func BuildScopeLabelNamesV1(componentID, projectID, environmentID string) []string {
	scopeLabels := make([]string, 0, 3)
	if componentID != "" {
		scopeLabels = append(scopeLabels, prometheusLabelName(labels.ComponentID))
	}
	if projectID != "" {
		scopeLabels = append(scopeLabels, prometheusLabelName(labels.ProjectID))
	}
	if environmentID != "" {
		scopeLabels = append(scopeLabels, prometheusLabelName(labels.EnvironmentID))
	}
	return scopeLabels
}

// BuildSumByClauseV1 builds the label list for a PromQL "sum by (...)" clause.
// scopeLabels are prepended; metricLabel (e.g. "container", "resource") is appended
// when non-empty. Pass an empty string for metricLabel when no extra dimension is needed.
func BuildSumByClauseV1(metricLabel string, scopeLabels []string) string {
	sumByLabels := make([]string, 0, len(scopeLabels)+1)
	sumByLabels = append(sumByLabels, scopeLabels...)
	if metricLabel != "" {
		sumByLabels = append(sumByLabels, metricLabel)
	}
	return strings.Join(sumByLabels, ", ")
}

// BuildHistogramSumByClauseV1 builds the label list for histogram "sum by (..., le)" clauses.
// If sumByClause is empty, this returns "le" to avoid invalid PromQL like "sum by (, le)".
func BuildHistogramSumByClauseV1(sumByClause string) string {
	if strings.TrimSpace(sumByClause) == "" {
		return "le"
	}
	return fmt.Sprintf("%s, le", sumByClause)
}

// BuildGroupLeftClauseV1 builds a PromQL group_left clause that propagates the
// given scope labels from the right-hand side of a join.
func BuildGroupLeftClauseV1(scopeLabels []string) string {
	if len(scopeLabels) == 0 {
		return "group_left"
	}
	return fmt.Sprintf("group_left (%s)", strings.Join(scopeLabels, ", "))
}

// BuildCPUUsageQuery builds a PromQL query for CPU usage rate
func BuildCPUUsageQuery(labelFilter string) string {
	query := fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container) (
    rate(container_cpu_usage_seconds_total{container!=""}[2m]) * on (pod) group_left (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid) kube_pod_labels{%s} )`, labelFilter)
	return query
}

// BuildCPUUsageQueryV1 builds a PromQL query for CPU usage rate, scoped by provided IDs only.
func BuildCPUUsageQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
    rate(container_cpu_usage_seconds_total{container!=""}[2m]) * on (pod) %s kube_pod_labels{%s} )`, sumByClause, groupLeftClause, labelFilter)
}

// BuildMemoryUsageQuery builds a PromQL query for memory usage
func BuildMemoryUsageQuery(labelFilter string) string {
	return fmt.Sprintf(`sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container) (
              container_memory_working_set_bytes{container!=""}
              * on (pod) group_left (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
                kube_pod_labels{%s}
            )`, labelFilter)
}

// BuildMemoryUsageQueryV1 builds a PromQL query for memory usage, scoped by provided IDs only.
func BuildMemoryUsageQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
              container_memory_working_set_bytes{container!=""}
              * on (pod) %s
                kube_pod_labels{%s}
            )`, sumByClause, groupLeftClause, labelFilter)
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

// BuildCPURequestsQueryV1 builds a PromQL query for CPU requests, scoped by provided IDs only.
func BuildCPURequestsQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
            (
                kube_pod_container_resource_requests{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) %s
            kube_pod_labels{%s}
        )`, sumByClause, groupLeftClause, labelFilter)
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

// BuildCPULimitsQueryV1 builds a PromQL query for CPU limits, scoped by provided IDs only.
func BuildCPULimitsQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
            (
                kube_pod_container_resource_limits{resource="cpu"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) %s
            kube_pod_labels{%s}
        )`, sumByClause, groupLeftClause, labelFilter)
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

// BuildMemoryRequestsQueryV1 builds a PromQL query for memory requests, scoped by provided IDs only.
func BuildMemoryRequestsQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
            (
                kube_pod_container_resource_requests{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) %s
            kube_pod_labels{%s}
        )`, sumByClause, groupLeftClause, labelFilter)
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

// BuildMemoryLimitsQueryV1 builds a PromQL query for memory limits, scoped by provided IDs only.
func BuildMemoryLimitsQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`sum by (%s) (
            (
                kube_pod_container_resource_limits{resource="memory"}
                AND ON (pod, namespace)
                (kube_pod_status_phase{phase="Running"} == 1)
            )
          * ON (pod, namespace) %s
            kube_pod_labels{%s}
        )`, sumByClause, groupLeftClause, labelFilter)
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

// ----------------------------
// HTTP REQUEST METRICS QUERIES (V1 — dynamic scope)
// ----------------------------
// V1 variants accept sumByClause and groupLeftClause built from the request's
// SearchScope so that queries aggregate correctly at namespace, project, or
// component granularity. Legacy (non-V1) functions above are kept for the
// legacy API path and are not modified.

// BuildHTTPRequestCountQueryV1 builds a PromQL query for HTTP request count,
// dynamically scoped by the provided label clauses.
func BuildHTTPRequestCountQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`
	    sum by (%s) (
	        rate(hubble_http_requests_total{reporter="client"}[2m])
	            * on(destination_pod) %s
	            label_replace(
	                kube_pod_labels{%s},
	                "destination_pod",
	                "$1",
	                "pod",
	                "(.*)"
	            )
	    )
	    >= 0
	`, sumByClause, groupLeftClause, labelFilter)
}

// BuildSuccessfulHTTPRequestCountQueryV1 builds a PromQL query for successful HTTP request count
// (1xx, 2xx, 3xx), dynamically scoped by the provided label clauses.
func BuildSuccessfulHTTPRequestCountQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`
	    sum by (%s) (
	        rate(hubble_http_requests_total{reporter="client", status=~"^[123]..?$"}[2m])
	            * on(destination_pod) %s
	            label_replace(
	                kube_pod_labels{%s},
	                "destination_pod",
	                "$1",
	                "pod",
	                "(.*)"
	            )
	    )
	    >= 0
	`, sumByClause, groupLeftClause, labelFilter)
}

// BuildUnsuccessfulHTTPRequestCountQueryV1 builds a PromQL query for unsuccessful HTTP request
// count (4xx, 5xx), dynamically scoped by the provided label clauses.
func BuildUnsuccessfulHTTPRequestCountQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`
	    sum by (%s) (
	        rate(hubble_http_requests_total{reporter="client", status=~"^[45]..?$"}[2m])
	            * on(destination_pod) %s
	            label_replace(
	                kube_pod_labels{%s},
	                "destination_pod",
	                "$1",
	                "pod",
	                "(.*)"
	            )
	    )
	    >= 0
	`, sumByClause, groupLeftClause, labelFilter)
}

// BuildMeanHTTPRequestLatencyQueryV1 builds a PromQL query for mean HTTP request latency,
// dynamically scoped by the provided label clauses.
func BuildMeanHTTPRequestLatencyQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	return fmt.Sprintf(`
	    sum by (%s) (
	        (
	            sum by(destination_pod) (rate(hubble_http_request_duration_seconds_sum{reporter="client"}[2m]))
	            /
	            sum by(destination_pod) (rate(hubble_http_requests_total{reporter="client"}[2m]))
	        )
	        * on(destination_pod) %s
	        label_replace(
	            kube_pod_labels{%s},
	            "destination_pod",
	            "$1",
	            "pod",
	            "(.*)"
	        )
	    )
	    >= 0
	`, sumByClause, groupLeftClause, labelFilter)
}

// Build50thPercentileHTTPRequestLatencyQueryV1 builds a PromQL query for 50th percentile HTTP
// request latency, dynamically scoped by the provided label clauses. Unlike the legacy variant,
// this correctly includes all scope labels (including environment) in the sum-by clause.
func Build50thPercentileHTTPRequestLatencyQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	histogramSumByClause := BuildHistogramSumByClauseV1(sumByClause)
	return fmt.Sprintf(`
	    histogram_quantile(
	        0.5,
	        sum by (%s) (
	            rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
	                * on(destination_pod) %s
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
	`, histogramSumByClause, groupLeftClause, labelFilter)
}

// Build90thPercentileHTTPRequestLatencyQueryV1 builds a PromQL query for 90th percentile HTTP
// request latency, dynamically scoped by the provided label clauses.
func Build90thPercentileHTTPRequestLatencyQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	histogramSumByClause := BuildHistogramSumByClauseV1(sumByClause)
	return fmt.Sprintf(`
	    histogram_quantile(
	        0.9,
	        sum by (%s) (
	            rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
	                * on(destination_pod) %s
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
	`, histogramSumByClause, groupLeftClause, labelFilter)
}

// Build99thPercentileHTTPRequestLatencyQueryV1 builds a PromQL query for 99th percentile HTTP
// request latency, dynamically scoped by the provided label clauses.
func Build99thPercentileHTTPRequestLatencyQueryV1(labelFilter, sumByClause, groupLeftClause string) string {
	histogramSumByClause := BuildHistogramSumByClauseV1(sumByClause)
	return fmt.Sprintf(`
	    histogram_quantile(
	        0.99,
	        sum by (%s) (
	            rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
	                * on(destination_pod) %s
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
	`, histogramSumByClause, groupLeftClause, labelFilter)
}
