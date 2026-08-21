// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"encoding/json"
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

// Resource identifies the target resource of an action, as reported by a
// handler via SetResource (or, pre-handler, by a surface adapter's seed —
// see NewAuditContext).
type Resource struct {
	ID       string         `json:"id,omitempty"`       // Resource identifier
	Name     string         `json:"name,omitempty"`     // Resource name (if different from ID)
	Metadata map[string]any `json:"metadata,omitempty"` // Additional resource-scoped context (optional)
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
	EventID      string         `json:"event_id"`               // Unique identifier (UUID v7)
	Timestamp    time.Time      `json:"timestamp"`              // When the action occurred
	Actor        Actor          `json:"actor"`                  // Who performed the action
	Action       string         `json:"action"`                 // Semantic action name (e.g., "create_project")
	Category     Category       `json:"category"`               // Action category
	Origin       Origin         `json:"origin,omitempty"`       // Surface that produced the event: api | mcp
	OperationID  string         `json:"operation_id,omitempty"` // OpenAPI operationId, e.g. "createProject"
	ResourceType string         `json:"-"`
	Resource     *Resource      `json:"resource"`           // Target resource (can be nil for non-resource actions)
	Result       Result         `json:"result"`             // Outcome
	RequestID    string         `json:"request_id"`         // Correlation ID linking to access log
	SourceIP     string         `json:"source_ip"`          // Client IP address
	Service      string         `json:"service"`            // Emitting service (e.g., "openchoreo-api")
	Metadata     map[string]any `json:"metadata,omitempty"` // Additional context (optional)
}

// eventJSON mirrors Event's field order but replaces Resource with
// resourceJSON — see MarshalJSON.
type eventJSON struct {
	EventID     string         `json:"event_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Actor       Actor          `json:"actor"`
	Action      string         `json:"action"`
	Category    Category       `json:"category"`
	Origin      Origin         `json:"origin,omitempty"`
	OperationID string         `json:"operation_id,omitempty"`
	Resource    *resourceJSON  `json:"resource"`
	Result      Result         `json:"result"`
	RequestID   string         `json:"request_id"`
	SourceIP    string         `json:"source_ip"`
	Service     string         `json:"service"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// resourceJSON is Resource with Type merged back in — the shape
// Logger.LogEvent renders, and the shape MarshalJSON reproduces.
type resourceJSON struct {
	Type     string         `json:"type,omitempty"`
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// MarshalJSON nests ResourceType inside "resource", matching what
// Logger.LogEvent renders (event.ResourceType != "" || event.Resource != nil
// → a "resource" group with "type" first). Without this, a future sink that
// calls json.Marshal(event) directly — rather than rendering attrs by hand
// like Logger does — would publish "resource_type" as a sibling field
// instead, a different wire shape for the same event.
func (e Event) MarshalJSON() ([]byte, error) {
	out := eventJSON{
		EventID:     e.EventID,
		Timestamp:   e.Timestamp,
		Actor:       e.Actor,
		Action:      e.Action,
		Category:    e.Category,
		Origin:      e.Origin,
		OperationID: e.OperationID,
		Result:      e.Result,
		RequestID:   e.RequestID,
		SourceIP:    e.SourceIP,
		Service:     e.Service,
		Metadata:    e.Metadata,
	}
	if e.ResourceType != "" || e.Resource != nil {
		out.Resource = &resourceJSON{Type: e.ResourceType}
		if e.Resource != nil {
			out.Resource.ID = e.Resource.ID
			out.Resource.Name = e.Resource.Name
			out.Resource.Metadata = e.Resource.Metadata
		}
	}
	return json.Marshal(out)
}

// AuditData is a mutable container for audit information set by handlers.
//
// Metadata has no writer anywhere in the codebase today — AddMetadata was
// removed and never replaced. It is still plumbed through end-to-end
// (EmitFromContext → Envelope → Event → Logger renders it as a "metadata"
// group), so in future can wire a setter without touching the pipeline;
// until then it is always nil and never appears in a published event.
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
