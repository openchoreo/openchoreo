// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package observability provides public interfaces for component observability.
// External Go modules can implement these interfaces to provide custom
// logging backends while the Observer handles authentication, authorization,
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

// LogsBackend defines the interface for logs backend implementations
type LogsBackend interface {
	// GetComponentApplicationLogs retrieves component application logs
	GetComponentApplicationLogs(ctx context.Context,
		params ComponentApplicationLogsParams) (*ComponentApplicationLogsResult, error)

	// GetWorkflowLogs retrieves workflow run logs
	GetWorkflowLogs(ctx context.Context,
		params WorkflowLogsParams) (*WorkflowLogsResult, error)
}
