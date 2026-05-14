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

// RuntimeTopologyRequest is the request body for POST /api/v1alpha1/metrics/runtime-topology.
// Matches the OpenAPI RuntimeTopologyRequest schema.
type RuntimeTopologyRequest struct {
	// SearchScope identifies the project and environment to query. namespace,
	// project, and environment are all required for this endpoint.
	SearchScope ComponentSearchScope `json:"searchScope"`

	// Time range for the query window (RFC3339, required).
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`

	// IncludeGateways toggles inclusion of gateway -> component edges. Defaults
	// to true when omitted.
	IncludeGateways *bool `json:"includeGateways,omitempty"`

	// IncludeExternal toggles inclusion of cross-project / off-platform edges.
	// Defaults to true when omitted.
	IncludeExternal *bool `json:"includeExternal,omitempty"`
}

// RuntimeTopologyNodeKind identifies the kind of node referenced in the topology.
type RuntimeTopologyNodeKind string

const (
	RuntimeTopologyNodeKindComponent RuntimeTopologyNodeKind = "component"
	RuntimeTopologyNodeKindGateway   RuntimeTopologyNodeKind = "gateway"
	RuntimeTopologyNodeKindExternal  RuntimeTopologyNodeKind = "external"
)

// RuntimeTopologyProtocol is the wire protocol of an observed edge.
type RuntimeTopologyProtocol string

const (
	RuntimeTopologyProtocolHTTP RuntimeTopologyProtocol = "http"
)

// RuntimeTopologyMetrics carries aggregate HTTP metrics for a node or edge.
// Latency values are in seconds. Counts are totals over the requested window.
type RuntimeTopologyMetrics struct {
	RequestCount             float64 `json:"requestCount"`
	UnsuccessfulRequestCount float64 `json:"unsuccessfulRequestCount"`
	MeanLatency              float64 `json:"meanLatency"`
	LatencyP50               float64 `json:"latencyP50"`
	LatencyP90               float64 `json:"latencyP90"`
	LatencyP99               float64 `json:"latencyP99"`
}

// RuntimeTopologyNodeRef identifies a node in the topology. The shape depends
// on Kind:
//   - Kind == "component": Component (name) + ComponentUID and Project (name) + ProjectUID
//     are populated from the metrics backend using pod label data.
//   - Kind == "gateway":   Name is required (e.g. "internet", "intranet").
//   - Kind == "external":  at least one of Host or Component should be set.
type RuntimeTopologyNodeRef struct {
	Kind RuntimeTopologyNodeKind `json:"kind"`

	// Component-kind fields — both name and UID from pod labels.
	Component    string `json:"component,omitempty"`
	ComponentUID string `json:"componentUid,omitempty"`
	Service      string `json:"service,omitempty"`

	// Gateway-kind fields.
	Name string `json:"name,omitempty"`

	// External-kind fields.
	Host       string `json:"host,omitempty"`
	Project    string `json:"project,omitempty"`
	ProjectUID string `json:"projectUid,omitempty"`

	// Common identifier — namespace the node lives in.
	Namespace string `json:"namespace,omitempty"`
}

// RuntimeTopologyNode is a topology node with its observed aggregate metrics.
// Embeds RuntimeTopologyNodeRef for the identifying fields.
type RuntimeTopologyNode struct {
	RuntimeTopologyNodeRef
	Metrics *RuntimeTopologyMetrics `json:"metrics,omitempty"`
}

// RuntimeTopologyEdge represents an observed traffic flow between two nodes.
type RuntimeTopologyEdge struct {
	// ID is a stable identifier for the edge. See the OpenAPI schema for the
	// convention used by the server when constructing it.
	ID       string                  `json:"id"`
	Source   RuntimeTopologyNodeRef  `json:"source"`
	Target   RuntimeTopologyNodeRef  `json:"target"`
	Protocol RuntimeTopologyProtocol `json:"protocol,omitempty"`
	Metrics  *RuntimeTopologyMetrics `json:"metrics,omitempty"`
}

// RuntimeTopologySummary describes the query window the response covers.
type RuntimeTopologySummary struct {
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// RuntimeTopologyResponse is the response body for the runtime topology
// endpoint. Nodes and edges only include entities for which traffic was
// observed in the window — static topology must come from a separate source.
type RuntimeTopologyResponse struct {
	Nodes   []RuntimeTopologyNode  `json:"nodes,omitempty"`
	Edges   []RuntimeTopologyEdge  `json:"edges,omitempty"`
	Summary RuntimeTopologySummary `json:"summary"`
}
