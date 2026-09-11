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

// PlatformLogsResponse is the response for GET /api/v1alpha1/platform-logs.
// Matches OpenAPI PlatformLogsResponse schema.
type PlatformLogsResponse struct {
	Logs   []PlatformLog `json:"logs"`
	Total  int           `json:"total"`
	TookMs int           `json:"tookMs"`
}

// PlatformLogFilterValuesRequest is the parsed form of the query string on
// GET /api/v1alpha1/platform-logs/filter-values.
type PlatformLogFilterValuesRequest struct {
	// Query is the record query the values are drawn from. Its own selections for
	// Filter are ignored.
	Query PlatformLogsQueryRequest `json:"query"`

	// Filter names the coordinate to list (required)
	Filter string `json:"filter"`

	// ValueSearch narrows the values returned rather than the records (optional)
	ValueSearch string `json:"valueSearch,omitempty"`

	// MaxValues caps how many values come back (optional)
	MaxValues int `json:"maxValues,omitempty"`
}

// PlatformLogFilterValue is one value a filter takes, with how many records carry it.
// Matches the OpenAPI schema of the same name.
type PlatformLogFilterValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// PlatformLogFilterValuesResponse is the response for
// GET /api/v1alpha1/platform-logs/filter-values.
type PlatformLogFilterValuesResponse struct {
	Filter        string                   `json:"filter"`
	Values        []PlatformLogFilterValue `json:"values"`
	TotalValues   int64                    `json:"totalValues"`
	TotalRelation string                   `json:"totalRelation"`
	TookMs        int                      `json:"tookMs"`
}
