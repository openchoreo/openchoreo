// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package observability provides public interfaces for component observability.
// External Go modules can implement these interfaces to provide custom
// logging adapters while the Observer handles authentication, authorization,
// and HTTP request handling.
package observability

import (
	"context"
	"time"
)

// ComponentApplicationLogsParams holds parameters for component application log queries
type ComponentApplicationLogsParams struct {
	ComponentID   string    `json:"componentId"`
	EnvironmentID string    `json:"environmentId"`
	ProjectID     string    `json:"projectId"`
	Namespace     string    `json:"namespace"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	SearchPhrase  string    `json:"searchPhrase"`
	LogLevels     []string  `json:"logLevels"`
	Versions      []string  `json:"versions"`
	VersionIDs    []string  `json:"versionIds"`
	Limit         int       `json:"limit"`
	SortOrder     string    `json:"sortOrder"`
}

// WorkflowLogsParams holds parameters for workflow log queries
type WorkflowLogsParams struct {
	Namespace       string    `json:"namespace"`
	WorkflowRunName string    `json:"workflowRunName"`
	TaskName        string    `json:"taskName"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	SearchPhrase    string    `json:"searchPhrase"`
	LogLevels       []string  `json:"logLevels"`
	Limit           int       `json:"limit"`
	SortOrder       string    `json:"sortOrder"`
}

// LogEntry represents a parsed log entry for component logs
type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Log           string            `json:"log"`
	LogLevel      string            `json:"logLevel"`
	ComponentID   string            `json:"componentId"`
	EnvironmentID string            `json:"environmentId"`
	ProjectID     string            `json:"projectId"`
	Version       string            `json:"version"`
	VersionID     string            `json:"versionId"`
	Namespace     string            `json:"namespace"`
	PodID         string            `json:"podId"`
	ContainerName string            `json:"containerName"`
	Labels        map[string]string `json:"labels"`
	// Additional fields for logs API v1
	ComponentName   string `json:"componentName,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	PodNamespace    string `json:"podNamespace,omitempty"`
	PodName         string `json:"podName,omitempty"`
}

// WorkflowLogEntry represents a parsed log entry for workflow logs
type WorkflowLogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Log           string            `json:"log"`
	LogLevel      string            `json:"logLevel"`
	PodNamespace  string            `json:"podNamespace,omitempty"`
	PodID         string            `json:"podId,omitempty"`
	PodName       string            `json:"podName,omitempty"`
	ContainerName string            `json:"containerName,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// ComponentApplicationLogsResult represents the result of a component log query
type ComponentApplicationLogsResult struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int        `json:"totalCount"`
	Took       int        `json:"took"`
}

// WorkflowLogsResult represents the result of a workflow log query
type WorkflowLogsResult struct {
	Logs       []WorkflowLogEntry `json:"logs"`
	TotalCount int                `json:"totalCount"`
	Took       int                `json:"took"`
}

// PlatformLogsParams holds parameters for platform log queries.
type PlatformLogsParams struct {
	ClusterInstances []string          `json:"clusterInstances"`
	Namespaces       []string          `json:"namespaces"`
	PodNames         []string          `json:"podNames"`
	ContainerNames   []string          `json:"containerNames"`
	Labels           map[string]string `json:"labels"`
	StartTime        time.Time         `json:"startTime"`
	EndTime          time.Time         `json:"endTime"`
	SearchPhrase     string            `json:"searchPhrase"`
	LogLevels        []string          `json:"logLevels"`
	Limit            int               `json:"limit"`
	SortOrder        string            `json:"sortOrder"`

	// IncludeFacets asks the adapter to aggregate the coordinate values reachable
	// under this query alongside the page of logs. Opt-in: it costs an aggregation
	// pass, and the answer describes the query rather than the page, so a caller
	// paginating asks only for the first one.
	IncludeFacets bool `json:"includeFacets"`
}

// PlatformLogEntry represents a parsed platform log record.
type PlatformLogEntry struct {
	Timestamp       time.Time         `json:"timestamp"`
	Log             string            `json:"log"`
	LogLevel        string            `json:"logLevel"`
	ClusterInstance string            `json:"clusterInstance"`
	NamespaceName   string            `json:"namespaceName"`
	PodName         string            `json:"podName"`
	ContainerName   string            `json:"containerName"`
	PodIP           string            `json:"podIp"`
	NodeName        string            `json:"nodeName"`
	ContainerImage  string            `json:"containerImage"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// PlatformLogFacetValue is one value a coordinate takes under a query, with the
// number of matching entries carrying it. The count may be approximate on a
// high-cardinality coordinate, so it orders a list rather than totalling it.
type PlatformLogFacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// PlatformLogFacets are the coordinate values reachable under a query, one list per
// filterable coordinate.
//
// Each list is aggregated with every filter applied except the one it describes.
// Counting namespaces under a selected namespace would report that namespace alone
// and leave a caller's picker with no way back to the others; excluding it means a
// selected namespace still narrows PodNames and ContainerNames while Namespaces
// keeps offering the alternatives.
type PlatformLogFacets struct {
	ClusterInstances []PlatformLogFacetValue `json:"clusterInstances"`
	Namespaces       []PlatformLogFacetValue `json:"namespaces"`
	PodNames         []PlatformLogFacetValue `json:"podNames"`
	ContainerNames   []PlatformLogFacetValue `json:"containerNames"`
}

// PlatformLogsResult represents the result of a platform log query
type PlatformLogsResult struct {
	Logs       []PlatformLogEntry `json:"logs"`
	TotalCount int                `json:"totalCount"`
	Took       int                `json:"took"`

	// Facets is nil unless the query asked for them and the adapter can compute
	// them. Nil means "unknown", never "nothing matches".
	Facets *PlatformLogFacets `json:"facets,omitempty"`
}

// LogsAdapter defines the interface for logs adapter implementations
type LogsAdapter interface {
	// GetComponentApplicationLogs retrieves component application logs
	GetComponentApplicationLogs(ctx context.Context,
		params ComponentApplicationLogsParams) (*ComponentApplicationLogsResult, error)

	// GetWorkflowLogs retrieves workflow run logs
	GetWorkflowLogs(ctx context.Context,
		params WorkflowLogsParams) (*WorkflowLogsResult, error)
}

// PlatformLogsAdapter defines the interface for fetching platform logs
type PlatformLogsAdapter interface {
	// GetPlatformLogs retrieves logs by raw Kubernetes coordinates, with no
	// project/component/environment correlation.
	GetPlatformLogs(ctx context.Context, params PlatformLogsParams) (*PlatformLogsResult, error)
}
