// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// PlatformLogsQueryRequest is the parsed form of the query string on
// GET /api/v1alpha1/platform-logs.
// Matches the OpenAPI PlatformLogs* parameter set.
type PlatformLogsQueryRequest struct {
	// Kubernetes coordinates to filter logs by (optional)
	ClusterInstances []string `json:"clusterInstance,omitempty"`
	Namespaces       []string `json:"namespace,omitempty"`
	PodNames         []string `json:"podName,omitempty"`
	ContainerNames   []string `json:"containerName,omitempty"`

	// Parsed form of the `labels` selector (optional)
	Labels map[string]string `json:"labels,omitempty"`

	// Time range for the query (required)
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`

	// Search and filter options (optional)
	SearchPhrase string   `json:"searchPhrase,omitempty"`
	LogLevels    []string `json:"logLevels,omitempty"`

	// Pagination and sorting (optional)
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"` // asc or desc, default: desc

	// Also return the coordinate values reachable under this query (optional)
	IncludeFacets bool `json:"includeFacets,omitempty"`
}

// PlatformLog is a single platform log record matching the OpenAPI PlatformLog schema.
type PlatformLog struct {
	Timestamp       string            `json:"timestamp"`
	Log             string            `json:"log"`
	Level           string            `json:"level,omitempty"`
	ClusterInstance string            `json:"clusterInstance,omitempty"`
	NamespaceName   string            `json:"namespaceName,omitempty"`
	PodName         string            `json:"podName,omitempty"`
	ContainerName   string            `json:"containerName,omitempty"`
	PodIP           string            `json:"podIp,omitempty"`
	NodeName        string            `json:"nodeName,omitempty"`
	ContainerImage  string            `json:"containerImage,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// PlatformLogFacetValue is one value a coordinate takes under the query, with how
// many matching entries carry it. Matches the OpenAPI PlatformLogFacetValue schema.
type PlatformLogFacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// PlatformLogFacets are the coordinate values reachable under the query, one list
// per filterable coordinate. Keyed as the query parameter that consumes them, so a
// client maps a facet onto its filter without a lookup table.
// Matches the OpenAPI PlatformLogFacets schema.
type PlatformLogFacets struct {
	ClusterInstance []PlatformLogFacetValue `json:"clusterInstance,omitempty"`
	Namespace       []PlatformLogFacetValue `json:"namespace,omitempty"`
	PodName         []PlatformLogFacetValue `json:"podName,omitempty"`
	ContainerName   []PlatformLogFacetValue `json:"containerName,omitempty"`
}

// PlatformLogsResponse is the response for GET /api/v1alpha1/platform-logs.
// Matches OpenAPI PlatformLogsResponse schema.
type PlatformLogsResponse struct {
	Logs   []PlatformLog `json:"logs"`
	Total  int           `json:"total"`
	TookMs int           `json:"tookMs"`

	// Facets is omitted unless the caller asked for it and the adapter could
	// compute it. Absent means "unknown", never "nothing matches".
	Facets *PlatformLogFacets `json:"facets,omitempty"`
}
