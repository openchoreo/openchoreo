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
	// resources.
	CategoryManagement Category = "management"
	// CategoryAuthorization covers authorization-change operations (authzroles,
	// authzrolebindings).
	CategoryAuthorization Category = "authorization"
)

// Resource identifies the target resource of an action, as reported by a
// handler via SetResource (or, pre-handler, by a surface adapter's seed —
// see NewAuditContext).
type Resource struct {
	Namespace string         `json:"namespace,omitempty"` // Namespace the resource belongs to, if namespace-scoped
	ID        string         `json:"id,omitempty"`        // Resource identifier
	Name      string         `json:"name,omitempty"`      // Resource name (if different from ID)
	Metadata  map[string]any `json:"metadata,omitempty"`  // Additional resource-scoped context (optional)
}

// Hierarchy identifies where in OpenChoreo's resource tree an audited
// operation was authorized, so a record is directly comparable to a policy
// scope. Declared locally rather than importing internal/authz/core, keeping
// this package a leaf so internal/openchoreo-api can depend on it without a
// cycle.
//
// Namespace/Project/Component/Resource mirror authz.ResourceHierarchy's json
// tags; Environment has no level there and comes from authz.Context.Resource,
// the ABAC attributes CEL conditions evaluate against.
//
// Recorded here rather than merged into Resource: SetResource replaces the
// whole *Resource rather than merging fields (see SetResource's doc comment),
// so a hierarchy stored inside Resource would be silently wiped by any of the
// handler-side SetResource calls that run after the authz check that
// populated it. Kept as a sibling and folded into the "resource" group only
// at render time (see Event.MarshalJSON and Logger.LogEvent), that ordering
// can't erase it.
type Hierarchy struct {
	Namespace   string `json:"namespace,omitempty"`
	Environment string `json:"environment,omitempty"`
	Project     string `json:"project,omitempty"`
	Component   string `json:"component,omitempty"`
	Resource    string `json:"resource,omitempty"`
}

// Result represents the outcome of an action
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	// ResultDenied means an authenticated subject was refused by policy
	// (e.g. a PDP denial). Distinguished from ResultUnauthenticated so a
	// misconfigured client's expired-token retries don't read the same as a
	// real authorization refusal.
	ResultDenied Result = "denied"
	// ResultUnauthenticated means the request carried no authenticated
	// subject at all — REST's 401, or MCP's tools.ErrNoSubject.
	ResultUnauthenticated Result = "unauthenticated"
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
	OperationID  string         `json:"operation_id,omitempty"` // OpenAPI operationId, e.g. "CreateProject"
	ResourceType string         `json:"-"`
	Resource     *Resource      `json:"resource"`           // Target resource (can be nil for non-resource actions)
	Hierarchy    Hierarchy      `json:"-"`                  // Project/component/resource the decision was made on; folded into "resource" at render time
	Result       Result         `json:"result"`             // Outcome
	RequestID    string         `json:"request_id"`         // Correlation ID linking to access log
	SourceIP     string         `json:"source_ip"`          // Client IP address
	Service      string         `json:"service"`            // Emitting service (e.g., "openchoreo-api")
	Metadata     map[string]any `json:"metadata,omitempty"` // Additional context (optional)
}

// eventJSON is the single definition of the published audit record. Field
// order here is published order, for json.Marshal and for Logger.LogEvent
// alike — reordering these fields changes the record consumers receive.
type eventJSON struct {
	EventID     string         `json:"event_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Actor       Actor          `json:"actor"`
	Action      string         `json:"action"`
	Category    Category       `json:"category"`
	Result      Result         `json:"result"`
	RequestID   string         `json:"request_id"`
	SourceIP    string         `json:"source_ip"`
	Service     string         `json:"service"`
	Origin      Origin         `json:"origin,omitempty"`
	OperationID string         `json:"operation_id,omitempty"`
	Resource    *resourceJSON  `json:"resource,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// resourceJSON is Resource with Type merged back in, plus Hierarchy's
// environment/project/component/resource folded in as flat siblings after
// namespace. Field order here is published order — see eventJSON.
type resourceJSON struct {
	Type        string         `json:"type,omitempty"`
	Namespace   string         `json:"namespace,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Project     string         `json:"project,omitempty"`
	Component   string         `json:"component,omitempty"`
	Resource    string         `json:"resource,omitempty"`
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// MarshalJSON defines the published audit record. Logger.LogEvent renders its
// output rather than building attrs of its own, so a sink that marshals an
// *Event and the log stream cannot disagree.
//
// It nests ResourceType inside "resource" (as "type"); without that, a
// marshaled Event would publish "resource_type" as a sibling field instead.
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventJSON{
		EventID:     e.EventID,
		Timestamp:   e.Timestamp,
		Actor:       e.Actor,
		Action:      e.Action,
		Category:    e.Category,
		Result:      e.Result,
		RequestID:   e.RequestID,
		SourceIP:    e.SourceIP,
		Service:     e.Service,
		Origin:      e.Origin,
		OperationID: e.OperationID,
		Resource:    e.resolvedResource(),
		Metadata:    e.Metadata,
	})
}

// resolvedResource folds ResourceType and Hierarchy into the published
// "resource" group, returning nil when the event has nothing resource-shaped
// to report (a rejection that resolved no operation).
//
// Resource.Namespace wins over the hierarchy's when set. buildEvent already
// applies that precedence via withHierarchyNamespaceFallback, so this repeats
// it only for the hand-built Events that reach exported MarshalJSON without
// passing through the emitter.
func (e Event) resolvedResource() *resourceJSON {
	if e.ResourceType == "" && e.Resource == nil && e.Hierarchy == (Hierarchy{}) {
		return nil
	}
	out := &resourceJSON{
		Type:        e.ResourceType,
		Namespace:   e.Hierarchy.Namespace,
		Environment: e.Hierarchy.Environment,
		Project:     e.Hierarchy.Project,
		Component:   e.Hierarchy.Component,
		Resource:    e.Hierarchy.Resource,
	}
	if e.Resource != nil {
		if e.Resource.Namespace != "" {
			out.Namespace = e.Resource.Namespace
		}
		out.ID = e.Resource.ID
		out.Name = e.Resource.Name
		out.Metadata = e.Resource.Metadata
	}
	return out
}

// AuditData is a mutable container for audit information set by handlers.
//
// Metadata has no writer today, so it is always nil and never appears in a
// published event. It is still plumbed through end-to-end (EmitFromContext →
// Envelope → Event → Logger renders it as a "metadata" group), so a future
// setter can be wired in without touching the pipeline.
//
// Result overrides the status-code-derived Result the REST middleware would
// otherwise compute (see determineResult in middleware.go). nil everywhere
// except a handler that hijacks the connection: once hijacked, the response
// status code can no longer change, so WriteHeader-based classification goes
// silently wrong for anything that fails after the hijack (see exec.go's
// SetResult calls). MCP never sets it — mcpaudit.classifyResult uses the
// tool's returned error instead, which stays available after any point in
// the call.
type AuditData struct {
	Resource  *Resource
	Metadata  map[string]any
	Hierarchy Hierarchy
	Result    *Result
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// auditDataKey is the context key for storing mutable audit data
	auditDataKey contextKey = "audit_data"
)
