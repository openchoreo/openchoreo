// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// PostureFindingsRequest represents query parameters for listing posture findings
type PostureFindingsRequest struct {
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt *TimeRange        `json:"created_at,omitempty"`
	UpdatedAt *TimeRange        `json:"updated_at,omitempty"`
	SortBy    string            `json:"sort_by,omitempty"`    // "created_at" or "updated_at", default: "updated_at"
	SortOrder string            `json:"sort_order,omitempty"` // "asc" or "desc", default: "desc"
	Page      int               `json:"page,omitempty"`       // default: 1
	PageSize  int               `json:"page_size,omitempty"`  // default: 50, max: 1000
}

// TimeRange represents a time range filter
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// PostureFindingsResponse represents the response for posture findings
type PostureFindingsResponse struct {
	Resources  []ResourceWithFindings `json:"resources"`
	Pagination PaginationInfo         `json:"pagination"`
}

// ResourceWithFindings represents a Kubernetes resource with its posture findings
type ResourceWithFindings struct {
	Type            string            `json:"type"`
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	UID             string            `json:"uid"`
	ResourceVersion string            `json:"resource_version"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Labels          map[string]string `json:"labels"`
	Findings        []PostureFinding  `json:"findings"`
}

// PostureFinding represents a single posture finding
type PostureFinding struct {
	ID          int64     `json:"id"`
	CheckID     string    `json:"check_id"`
	CheckName   string    `json:"check_name"`
	Severity    string    `json:"severity"`
	Category    *string   `json:"category,omitempty"`
	Description *string   `json:"description,omitempty"`
	Remediation *string   `json:"remediation,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	TotalItems int  `json:"total_items"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}
