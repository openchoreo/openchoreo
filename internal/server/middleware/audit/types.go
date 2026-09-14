// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"encoding/json"
	"time"
)

// Actor represents who performed the action. ID is unique only within Issuer:
// the same sub from two IdPs is two different subjects.
type Actor struct {
	Type         string              `json:"type"`                   // e.g., "user", "service_account", "anonymous"
	ID           string              `json:"id"`                     // The token's validated sub claim, or "anonymous"
	Issuer       string              `json:"issuer,omitempty"`       // The token's iss claim; ID's namespace
	SessionID    string              `json:"session_id,omitempty"`   // The token's sid claim, joining this event to an IdP login
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
	// CategoryAccess covers reads that disclose enough to be worth recording in
	// their own right — reading the audit trail itself, today. Most reads are
	// not audited at all (see each service's RESTExemptions); this category is
	// for the ones where knowing who looked is part of the point, so an
	// investigator can filter disclosure apart from change.
	CategoryAccess Category = "access"
)

// Resource identifies the target resource of an action, as reported by a
// handler via SetResource (or, pre-handler, by a surface adapter's seed —
// see NewAuditContext).
//
// UID equals Name for a resource whose only identifier is a generated one (an
// observer incident): omitting it instead would publish that value as "name"
// on a denial, from the path seed, and as "uid" on success.
type Resource struct {
	Namespace string // Namespace the resource belongs to, if namespace-scoped
	UID       string // Server-generated id that is never reused, absent when the operation returned no object
	Name      string // The name the operation addresses the resource by
	Metadata  map[string]any
}

// Hierarchy identifies where in OpenChoreo's resource tree an audited
// operation was authorized, so a record is directly comparable to a policy
// scope. Declared locally rather than importing internal/authz/core, keeping
// this package a leaf so internal/openchoreo-api can depend on it without a
// cycle.
//
// Namespace/Project/Component/Resource mirror authz.ResourceHierarchy's
// fields; Environment has no level there and comes from authz.Context.Resource,
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
	Namespace   string
	Environment string
	Project     string
	Component   string
	Resource    string
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

// Surface identifies which surface of the API a call arrived through. MCP
// wraps the same API, so the REST value is "rest" rather than "api".
type Surface string

const (
	SurfaceREST Surface = "rest"
	SurfaceMCP  Surface = "mcp"
)

// SchemaVersion is stamped on every published event as "schema_version".
// major.minor: major on a field removal or a changed value representation,
// minor on an addition.
const SchemaVersion = "1.0"

// HTTPInfo records the request line of an event that arrived over HTTP.
// Absent for an MCP tools/call, which has no request line of its own.
type HTTPInfo struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// RequestInfo is what a surface adapter captures when it receives a request.
// None of it survives to emit time: emission runs in a deferred call after the
// handler returns, so a timestamp taken there is the response's completion —
// minutes late for a handler that hijacks the connection.
type RequestInfo struct {
	// EventTime is when the audit adapter received the request — after token
	// validation on an authenticated request, since Middleware sits inside
	// auth. The gap from socket-accept belongs to the access log.
	EventTime time.Time
	// HTTP is the request line, nil on a surface that has none.
	HTTP *HTTPInfo
}

// Event represents a complete audit log event
type Event struct {
	EventID      string // Unique identifier for this record (UUID v7)
	EventTime    time.Time
	Actor        Actor
	Action       string // Semantic action name (e.g., "create_project")
	Category     Category
	Surface      Surface // Which surface of the API the call arrived through: rest | mcp
	OperationID  string  // Canonical operation identifier, e.g. "CreateProject"
	HTTP         *HTTPInfo
	ResourceType string
	Resource     *Resource // Target resource (can be nil for non-resource actions)
	Hierarchy    Hierarchy // Project/component/resource the decision was made on; folded into "resource" at render time
	Result       Result
	RequestID    string // Correlation ID linking to the access log line
	SourceIP     string // Client IP address
	// UserAgent is client-supplied and unverifiable, like SourceIP. It is the
	// only field that separates a portal session from occ, CI or an agent.
	UserAgent string
	Producer  string // Emitting service (e.g., "openchoreo-api")
	Metadata  map[string]any
}

// eventJSON is the single definition of the published audit record. Field
// order here is published order, for json.Marshal and for Logger.LogEvent
// alike — reordering these fields changes the record consumers receive.
type eventJSON struct {
	SchemaVersion string         `json:"schema_version"`
	EventID       string         `json:"event_id"`
	EventTime     time.Time      `json:"event_time"`
	Actor         Actor          `json:"actor"`
	Action        string         `json:"action"`
	Category      Category       `json:"category"`
	Result        Result         `json:"result"`
	RequestID     string         `json:"request_id"`
	SourceIP      string         `json:"source_ip"`
	UserAgent     string         `json:"user_agent"`
	Producer      string         `json:"producer"`
	Surface       Surface        `json:"surface,omitempty"`
	OperationID   string         `json:"operation_id,omitempty"`
	HTTP          *HTTPInfo      `json:"http,omitempty"`
	Resource      *resourceJSON  `json:"resource,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
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
	UID         string         `json:"uid,omitempty"`
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
		SchemaVersion: SchemaVersion,
		EventID:       e.EventID,
		EventTime:     e.EventTime,
		Actor:         e.Actor,
		Action:        e.Action,
		Category:      e.Category,
		Result:        e.Result,
		RequestID:     e.RequestID,
		SourceIP:      e.SourceIP,
		UserAgent:     e.UserAgent,
		Producer:      e.Producer,
		Surface:       e.Surface,
		OperationID:   e.OperationID,
		HTTP:          e.HTTP,
		Resource:      e.resolvedResource(),
		Metadata:      e.Metadata,
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
		out.UID = e.Resource.UID
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
//
// Request is the exception to "mutable": the adapter fills it once at entry
// and nothing else writes it.
type AuditData struct {
	Resource  *Resource
	Metadata  map[string]any
	Hierarchy Hierarchy
	Result    *Result
	Request   RequestInfo
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// auditDataKey is the context key for storing mutable audit data
	auditDataKey contextKey = "audit_data"
)
