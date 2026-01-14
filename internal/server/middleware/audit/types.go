// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"time"
)

// Actor represents who performed the action
type Actor struct {
	Type         string                 `json:"type"`                   // e.g., "user", "service_account", "anonymous"
	ID           string                 `json:"id"`                     // User ID, service account ID, or "anonymous"
	Entitlements map[string]interface{} `json:"entitlements,omitempty"` // Optional entitlements associated with the actor
}

// ActionCategory represents the category of audit action
type ActionCategory string

const (
	CategoryResource      ActionCategory = "resource"
	CategoryAuth          ActionCategory = "auth"
	CategoryObservability ActionCategory = "observability"
)

// Resource represents the target resource of an action
type Resource struct {
	Type string `json:"type"`           // e.g., "project", "component", "environment"
	ID   string `json:"id,omitempty"`   // Resource identifier
	Name string `json:"name,omitempty"` // Resource name (if different from ID)
}

// Result represents the outcome of an action
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultDenied  Result = "denied"
)

// Event represents a complete audit log event
type Event struct {
	EventID   string         `json:"event_id"`           // Unique identifier (UUID v7)
	Timestamp time.Time      `json:"timestamp"`          // When the action occurred
	Actor     Actor          `json:"actor"`              // Who performed the action
	Action    string         `json:"action"`             // Semantic action name (e.g., "create_project")
	Category  ActionCategory `json:"category"`           // Action category
	Resource  *Resource      `json:"resource"`           // Target resource (can be nil for non-resource actions)
	Result    Result         `json:"result"`             // Outcome
	RequestID string         `json:"request_id"`         // Correlation ID linking to access log
	SourceIP  string         `json:"source_ip"`          // Client IP address
	Service   string         `json:"service"`            // Emitting service (e.g., "openchoreo-api")
	Metadata  map[string]any `json:"metadata,omitempty"` // Additional context (optional)
}

// ActionDefinition defines how to map an HTTP route to an audit action
type ActionDefinition struct {
	// Method is the HTTP method (GET, POST, PUT, DELETE, etc.)
	Method string
	// Pattern is the route pattern to match (using Go 1.22+ ServeMux patterns)
	// Examples: "/api/v1/orgs/{org}/projects", "POST /api/v1/components/{id}"
	Pattern string
	// Action is the semantic action name for audit logging
	Action string
	// Category is the action category
	Category ActionCategory
}

// AuditData is a mutable container for audit information set by handlers
type AuditData struct {
	Resource *Resource
	Metadata map[string]any
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// auditDataKey is the context key for storing mutable audit data
	auditDataKey contextKey = "audit_data"
)
