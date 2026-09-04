// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// ClusterLogsQueryRequest is the parsed form of the query string on
// GET /api/v1alpha1/cluster-logs.
// Matches the OpenAPI ClusterLogs* parameter set.
type ClusterLogsQueryRequest struct {
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

// ClusterLog is a single cluster log record matching the OpenAPI ClusterLog schema.
type ClusterLog struct {
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

// ClusterLogsResponse is the response for GET /api/v1alpha1/cluster-logs.
// Matches OpenAPI ClusterLogsResponse schema.
type ClusterLogsResponse struct {
	Logs   []ClusterLog `json:"logs"`
	Total  int          `json:"total"`
	TookMs int          `json:"tookMs"`
}
