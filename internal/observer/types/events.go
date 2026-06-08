// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// EventsQueryRequest represents the request body for POST /api/v1/events/query
// Matches OpenAPI EventsQueryRequest schema.
type EventsQueryRequest struct {
	// SearchScope defines where to search for events (component or workflow)
	SearchScope *SearchScope `json:"searchScope" validate:"required"`

	// Time range for the query (required)
	StartTime string `json:"startTime" validate:"required"`
	EndTime   string `json:"endTime" validate:"required"`

	// Pagination and sorting
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"` // asc or desc, default: desc
}

// EventMetadata contains metadata for a single event entry.
// Matches OpenAPI EventEntry.metadata schema.
type EventMetadata struct {
	ComponentName   string `json:"componentName,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	ComponentUID    string `json:"componentUid,omitempty"`
	ProjectUID      string `json:"projectUid,omitempty"`
	EnvironmentUID  string `json:"environmentUid,omitempty"`
	ObjectKind      string `json:"objectKind,omitempty"`
	ObjectName      string `json:"objectName,omitempty"`
	ObjectNamespace string `json:"objectNamespace,omitempty"`
}

// EventEntry represents a single Kubernetes event entry in the response.
// Matches OpenAPI EventEntry schema.
type EventEntry struct {
	Timestamp string         `json:"timestamp"`
	Message   string         `json:"message"`
	Type      string         `json:"type,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  *EventMetadata `json:"metadata,omitempty"`
}

// EventsQueryResponse represents the response for POST /api/v1/events/query.
// Matches OpenAPI EventsQueryResponse schema.
type EventsQueryResponse struct {
	Events []EventEntry `json:"events"`
	Total  int          `json:"total"`
	TookMs int          `json:"tookMs"`
}
