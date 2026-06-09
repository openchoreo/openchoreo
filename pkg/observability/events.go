// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"time"
)

// ComponentEventsParams holds parameters for component Kubernetes event queries
type ComponentEventsParams struct {
	ComponentID   string    `json:"componentId"`
	EnvironmentID string    `json:"environmentId"`
	ProjectID     string    `json:"projectId"`
	Namespace     string    `json:"namespace"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	Limit         int       `json:"limit"`
	SortOrder     string    `json:"sortOrder"`
}

// WorkflowEventsParams holds parameters for workflow run Kubernetes event queries
type WorkflowEventsParams struct {
	Namespace       string    `json:"namespace"`
	WorkflowRunName string    `json:"workflowRunName"`
	TaskName        string    `json:"taskName"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	Limit           int       `json:"limit"`
	SortOrder       string    `json:"sortOrder"`
}

// EventEntry represents a parsed Kubernetes event entry.
// The same shape is used for both component and workflow scopes; fields that do
// not apply to a given scope are left empty.
type EventEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	// OpenChoreo resource metadata
	ComponentID     string `json:"componentId,omitempty"`
	ComponentName   string `json:"componentName,omitempty"`
	EnvironmentID   string `json:"environmentId,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	ProjectID       string `json:"projectId,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	// Kubernetes object the event involves
	ObjectKind      string `json:"objectKind,omitempty"`
	ObjectName      string `json:"objectName,omitempty"`
	ObjectNamespace string `json:"objectNamespace,omitempty"`
}

// ComponentEventsResult represents the result of a component event query
type ComponentEventsResult struct {
	Events     []EventEntry `json:"events"`
	TotalCount int          `json:"totalCount"`
	Took       int          `json:"took"`
}

// WorkflowEventsResult represents the result of a workflow run event query
type WorkflowEventsResult struct {
	Events     []EventEntry `json:"events"`
	TotalCount int          `json:"totalCount"`
	Took       int          `json:"took"`
}

// EventsAdapter defines the interface for events adapter implementations
type EventsAdapter interface {
	// GetComponentEvents retrieves Kubernetes events for a component
	GetComponentEvents(ctx context.Context,
		params ComponentEventsParams) (*ComponentEventsResult, error)

	// GetWorkflowEvents retrieves Kubernetes events for a workflow run
	GetWorkflowEvents(ctx context.Context,
		params WorkflowEventsParams) (*WorkflowEventsResult, error)
}
