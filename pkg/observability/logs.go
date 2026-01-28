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

// LogEntry represents a parsed log entry
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
}

// ComponentApplicationLogsResult represents the result of a component log query
type ComponentApplicationLogsResult struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int        `json:"totalCount"`
	Took       int        `json:"took"`
}

type LogsBackend interface {
	GetComponentApplicationLogs(ctx context.Context,
		params ComponentApplicationLogsParams) (*ComponentApplicationLogsResult, error)
}
