// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"time"
)

// Actor represents who performed the action
type Actor struct {
	Type         string              `json:"type"`                   // e.g., "user", "service_account", "anonymous"
	ID           string              `json:"id"`                     // User ID, service account ID, or "anonymous"
	Entitlements map[string][]string `json:"entitlements,omitempty"` // Optional entitlements associated with the actor
}

// Category represents the category of audit action
type Category string

const (
	// CategoryManagement covers create/update/delete on managed platform
	// resources (projects, dataplanes, environments, secrets today; more as
	// P1 coverage expands).
	CategoryManagement Category = "management"
	// CategoryAuthorization covers authorization-change operations (authzroles,
	// authzrolebindings). No Operation uses it yet (P1 coverage work), but
	// policy.go's selector grammar already validates against it.
	CategoryAuthorization Category = "authorization"
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

// Origin identifies which surface produced an audit event.
type Origin string

const (
	OriginAPI Origin = "api"
	OriginMCP Origin = "mcp"
)

// Event represents a complete audit log event
type Event struct {
	EventID     string         `json:"event_id"`               // Unique identifier (UUID v7)
	Timestamp   time.Time      `json:"timestamp"`              // When the action occurred
	Actor       Actor          `json:"actor"`                  // Who performed the action
	Action      string         `json:"action"`                 // Semantic action name (e.g., "create_project")
	Category    Category       `json:"category"`               // Action category
	Origin      Origin         `json:"origin,omitempty"`       // Surface that produced the event: api | mcp
	OperationID string         `json:"operation_id,omitempty"` // OpenAPI operationId, e.g. "createProject"
	Resource    *Resource      `json:"resource"`               // Target resource (can be nil for non-resource actions)
	Result      Result         `json:"result"`                 // Outcome
	RequestID   string         `json:"request_id"`             // Correlation ID linking to access log
	SourceIP    string         `json:"source_ip"`              // Client IP address
	Service     string         `json:"service"`                // Emitting service (e.g., "openchoreo-api")
	Metadata    map[string]any `json:"metadata,omitempty"`     // Additional context (optional)
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
