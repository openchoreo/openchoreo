// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

import "time"

// MetricsQueryRequest represents the request body for POST /api/v1/metrics/query
// Matches OpenAPI MetricsQueryRequest schema
type MetricsQueryRequest struct {
	// Metric defines the type of metrics to query
	Metric string `json:"metric" validate:"required"` // "resource" | "http"

	// Time range for the query (required)
	StartTime string `json:"startTime" validate:"required"`
	EndTime   string `json:"endTime" validate:"required"`

	// Step is the query resolution step (e.g. "1m", "5m", "15m", "30m", "1h")
	Step *string `json:"step,omitempty"`

	// SearchScope restricts the query to a specific component, project, or namespace.
	// Uses ComponentSearchScope directly (not the SearchScope union) because metrics
	// queries do not support workflow scope. Required — matches the OpenAPI spec.
	SearchScope ComponentSearchScope `json:"searchScope"`
}

// Metric type constants
const (
	MetricTypeResource = "resource"
	MetricTypeHTTP     = "http"
)

// MetricsTimeSeriesItem represents a single point in a metrics time series
type MetricsTimeSeriesItem struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ResourceMetricsQueryResponse is the response for metric="resource" queries
type ResourceMetricsQueryResponse struct {
	CPUUsage       []MetricsTimeSeriesItem `json:"cpuUsage,omitempty"`
	CPURequests    []MetricsTimeSeriesItem `json:"cpuRequests,omitempty"`
	CPULimits      []MetricsTimeSeriesItem `json:"cpuLimits,omitempty"`
	MemoryUsage    []MetricsTimeSeriesItem `json:"memoryUsage,omitempty"`
	MemoryRequests []MetricsTimeSeriesItem `json:"memoryRequests,omitempty"`
	MemoryLimits   []MetricsTimeSeriesItem `json:"memoryLimits,omitempty"`
}

// HTTPMetricsQueryResponse is the response for metric="http" queries
type HTTPMetricsQueryResponse struct {
	RequestCount             []MetricsTimeSeriesItem `json:"requestCount,omitempty"`
	SuccessfulRequestCount   []MetricsTimeSeriesItem `json:"successfulRequestCount,omitempty"`
	UnsuccessfulRequestCount []MetricsTimeSeriesItem `json:"unsuccessfulRequestCount,omitempty"`
	MeanLatency              []MetricsTimeSeriesItem `json:"meanLatency,omitempty"`
	LatencyP50               []MetricsTimeSeriesItem `json:"latencyP50,omitempty"`
	LatencyP90               []MetricsTimeSeriesItem `json:"latencyP90,omitempty"`
	LatencyP99               []MetricsTimeSeriesItem `json:"latencyP99,omitempty"`
}
