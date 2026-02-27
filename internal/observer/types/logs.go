// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/json"
	"fmt"
)

// ComponentSearchScope defines the search scope for component logs
// Matches OpenAPI ComponentSearchScope schema
type ComponentSearchScope struct {
	Namespace   string `json:"namespace" validate:"required"`
	Project     string `json:"project,omitempty"`
	Component   string `json:"component,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// WorkflowSearchScope defines the search scope for workflow run logs
// Matches OpenAPI WorkflowSearchScope schema
type WorkflowSearchScope struct {
	Namespace       string `json:"namespace" validate:"required"`
	WorkflowRunName string `json:"workflowRunName,omitempty"`
}

// LogsQueryRequest represents the request body for POST /api/v1/logs/query
// Matches OpenAPI LogsQueryRequest schema
type LogsQueryRequest struct {
	// SearchScope defines where to search for logs (component or workflow)
	SearchScope *SearchScope `json:"searchScope" validate:"required"`

	// Time range for the query (required)
	StartTime string `json:"startTime" validate:"required"`
	EndTime   string `json:"endTime" validate:"required"`

	// Optional filters
	SearchPhrase string   `json:"searchPhrase,omitempty"`
	LogLevels    []string `json:"logLevels,omitempty"`

	// Pagination and sorting
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"` // asc or desc, default: desc
}

// SearchScope is a union type for component or workflow search scope
// Implements oneOf from OpenAPI spec - can be either ComponentSearchScope or WorkflowSearchScope
type SearchScope struct {
	Component *ComponentSearchScope `json:"-"`
	Workflow  *WorkflowSearchScope  `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle oneOf
// The JSON can be either a ComponentSearchScope or WorkflowSearchScope directly
func (s *SearchScope) UnmarshalJSON(data []byte) error {
	// First, unmarshal into a map to check for distinguishing fields
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to parse searchScope: %w", err)
	}

	// Check for distinguishing fields:
	// - workflowRunName indicates WorkflowSearchScope
	// - project, component, or environment indicates ComponentSearchScope
	if _, hasWorkflowRunName := raw["workflowRunName"]; hasWorkflowRunName {
		var workflowScope WorkflowSearchScope
		if err := json.Unmarshal(data, &workflowScope); err != nil {
			return fmt.Errorf("failed to unmarshal as WorkflowSearchScope: %w", err)
		}
		s.Workflow = &workflowScope
		return nil
	}

	// Check for component-specific fields
	if _, hasProject := raw["project"]; hasProject {
		var componentScope ComponentSearchScope
		if err := json.Unmarshal(data, &componentScope); err != nil {
			return fmt.Errorf("failed to unmarshal as ComponentSearchScope: %w", err)
		}
		s.Component = &componentScope
		return nil
	}
	if _, hasComponent := raw["component"]; hasComponent {
		var componentScope ComponentSearchScope
		if err := json.Unmarshal(data, &componentScope); err != nil {
			return fmt.Errorf("failed to unmarshal as ComponentSearchScope: %w", err)
		}
		s.Component = &componentScope
		return nil
	}
	if _, hasEnvironment := raw["environment"]; hasEnvironment {
		var componentScope ComponentSearchScope
		if err := json.Unmarshal(data, &componentScope); err != nil {
			return fmt.Errorf("failed to unmarshal as ComponentSearchScope: %w", err)
		}
		s.Component = &componentScope
		return nil
	}

	// If only namespace is present, default to ComponentSearchScope
	// (both types require namespace, but component scope is more common for namespace-only queries)
	var componentScope ComponentSearchScope
	if err := json.Unmarshal(data, &componentScope); err != nil {
		return fmt.Errorf("failed to unmarshal searchScope: %w", err)
	}
	s.Component = &componentScope
	return nil
}

// MarshalJSON implements custom JSON marshaling
func (s *SearchScope) MarshalJSON() ([]byte, error) {
	if s.Component != nil {
		return json.Marshal(s.Component)
	}
	if s.Workflow != nil {
		return json.Marshal(s.Workflow)
	}
	return json.Marshal(nil)
}

// LogMetadata contains metadata for a log entry
// Used for both component and workflow logs
// Matches OpenAPI ComponentLogEntry.metadata schema (workflow logs use a subset)
type LogMetadata struct {
	// Component-specific fields (empty for workflow logs)
	ComponentName   string `json:"componentName,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	ComponentUID    string `json:"componentUid,omitempty"`
	ProjectUID      string `json:"projectUid,omitempty"`
	EnvironmentUID  string `json:"environmentUid,omitempty"`
	ContainerName   string `json:"containerName,omitempty"`
	PodName         string `json:"podName,omitempty"`
	PodNamespace    string `json:"podNamespace,omitempty"`
}

// LogEntry represents a single log entry in the response
// Used for both component and workflow logs
// Matches OpenAPI ComponentLogEntry/WorkflowLogEntry schemas
type LogEntry struct {
	Timestamp string       `json:"timestamp"`
	Log       string       `json:"log"`
	Level     string       `json:"level,omitempty"`
	Metadata  *LogMetadata `json:"metadata,omitempty"`
}

// LogsQueryResponse represents the response for POST /api/v1/logs/query
// Matches OpenAPI LogsQueryResponse schema
type LogsQueryResponse struct {
	Logs   []LogEntry `json:"logs"`
	Total  int        `json:"total"`
	TookMs int        `json:"tookMs"`
}

// ErrorResponse represents a standardized error response for the new API
type ErrorResponse struct {
	Title     string `json:"title"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

// Error codes for the new API
const (
	// Error types
	ErrorTypeValidation = "validation_error"
	ErrorTypeInternal   = "internal_error"
	ErrorTypeForbidden  = "forbidden"
	ErrorTypeNotFound   = "not_found"

	// Error codes
	ErrorCodeInvalidRequest   = "OBS-V1-001"
	ErrorCodeMissingField     = "OBS-V1-002"
	ErrorCodeInvalidTimeRange = "OBS-V1-003"
	ErrorCodeInternalError    = "OBS-V1-100"
	ErrorCodeForbidden        = "OBS-V1-403"
)
