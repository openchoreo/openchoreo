// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// AuditLogsActorFilter filters the record's actor group. Matches the OpenAPI
// AuditLogsActorFilter schema.
type AuditLogsActorFilter struct {
	IDs   []string `json:"id,omitempty"`
	Types []string `json:"type,omitempty"`
	// Issuers is the namespace an ID is unique within.
	Issuers    []string `json:"issuer,omitempty"`
	SessionIDs []string `json:"session_id,omitempty"`
	// Entitlements matches values across every claim in the record's
	// actor.entitlements map: the key varies by subject kind.
	Entitlements []string `json:"entitlements,omitempty"`
}

// AuditLogsResourceFilter filters the record's resource group. Matches the
// OpenAPI AuditLogsResourceFilter schema.
//
// No uid: absent on deletes and non-CRUD mutations. No resource: set only where
// it duplicates the name.
type AuditLogsResourceFilter struct {
	Types      []string `json:"type,omitempty"`
	Namespaces []string `json:"namespace,omitempty"`
	// Environments are dual-scoped "{namespace}/{name}", the form the record
	// stores, since the value is recorded exactly as authorization evaluated it.
	Environments []string `json:"environment,omitempty"`
	Projects     []string `json:"project,omitempty"`
	Components   []string `json:"component,omitempty"`
	Names        []string `json:"name,omitempty"`
}

// AuditLogsQueryRequest is the decoded body of
// POST /api/v1alpha1/audit-logs/query. Matches the OpenAPI
// AuditLogsQueryRequest schema.
//
// Filters are named and nested as the record field they match, so the
// record-derived ones keep the record's snake_case while the query's own
// controls stay camelCase.
//
// The filters under Resource narrow the result set and nothing else — see
// service/audit_logs_authz.go.
type AuditLogsQueryRequest struct {
	// Time range for the query (required)
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`

	// Record-group filters (optional)
	Actor    AuditLogsActorFilter    `json:"actor"`
	Resource AuditLogsResourceFilter `json:"resource"`

	// Event filters (optional)
	Actions      []string `json:"action,omitempty"`
	Categories   []string `json:"category,omitempty"`
	Results      []string `json:"result,omitempty"`
	Producers    []string `json:"producer,omitempty"`
	Surfaces     []string `json:"surface,omitempty"`
	OperationIDs []string `json:"operation_id,omitempty"`
	RequestIDs   []string `json:"request_id,omitempty"`
	EventIDs     []string `json:"event_id,omitempty"`
	SourceIPs    []string `json:"source_ip,omitempty"`
	UserAgents   []string `json:"user_agent,omitempty"`

	// Search options (optional)
	SearchPhrase string `json:"searchPhrase,omitempty"`

	// Pagination and sorting (optional)
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"` // asc or desc, default: desc
	// Cursor is opaque and minted by the adapter. Passed through unparsed in
	// both directions; observer never interprets it.
	Cursor string `json:"cursor,omitempty"`

	// IncludeTimeline asks for per-interval counts across the window, which no
	// page of records can be bucketed into.
	IncludeTimeline bool `json:"includeTimeline,omitempty"`
	// TimelineInterval is a "<count><unit>" width (m, h, d, w). Empty leaves
	// the width to the adapter, which also coarsens past 500 buckets.
	TimelineInterval string `json:"timelineInterval,omitempty"`
}

// AuditLogActor identifies who performed an audited action. Matches the OpenAPI
// AuditLogActor schema.
type AuditLogActor struct {
	Type         string              `json:"type"`
	ID           string              `json:"id"`
	Issuer       string              `json:"issuer,omitempty"`
	SessionID    string              `json:"session_id,omitempty"`
	Entitlements map[string][]string `json:"entitlements,omitempty"`
}

// AuditLogHTTPInfo is the request line of an event that arrived over HTTP.
// Matches the OpenAPI AuditLogHTTPInfo schema.
type AuditLogHTTPInfo struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// AuditLogResource is the target resource of an audited action together with the
// point in OpenChoreo's tree the decision was authorized at. Matches the OpenAPI
// AuditLogResource schema.
type AuditLogResource struct {
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

// AuditLogCollectorInfo is where a record was collected from, as stamped by the
// collector rather than by the emitting service. Matches the OpenAPI
// AuditLogCollectorInfo schema.
type AuditLogCollectorInfo struct {
	NamespaceName string `json:"namespaceName,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

// AuditLogRecord is one audit event as served by
// POST /api/v1alpha1/audit-logs/query. Matches the OpenAPI AuditLogRecord
// schema.
//
// Field names are snake_case while the surrounding response is camelCase: this
// is the published record, so a response can be compared against an exported
// log line key for key. Category and Result stay plain strings because their
// vocabulary grows with SchemaVersion.
type AuditLogRecord struct {
	SchemaVersion string        `json:"schema_version"`
	EventID       string        `json:"event_id"`
	EventTime     string        `json:"event_time"`
	Actor         AuditLogActor `json:"actor"`
	Action        string        `json:"action"`
	Category      string        `json:"category"`
	Result        string        `json:"result"`
	// No omitempty: the emitter writes all four unconditionally, so omitting an
	// empty one would drop a key from a record served verbatim.
	RequestID   string                 `json:"request_id"`
	SourceIP    string                 `json:"source_ip"`
	UserAgent   string                 `json:"user_agent"`
	Producer    string                 `json:"producer"`
	Surface     string                 `json:"surface,omitempty"`
	OperationID string                 `json:"operation_id,omitempty"`
	HTTP        *AuditLogHTTPInfo      `json:"http,omitempty"`
	Resource    *AuditLogResource      `json:"resource,omitempty"`
	Metadata    map[string]any         `json:"metadata,omitempty"`
	Collector   *AuditLogCollectorInfo `json:"collector,omitempty"`
	Log         string                 `json:"log,omitempty"`
}

// AuditLogsResponse is the response for POST /api/v1alpha1/audit-logs/query.
// Matches the OpenAPI AuditLogsResponse schema.
type AuditLogsResponse struct {
	Records []AuditLogRecord `json:"records"`
	Total   int64            `json:"total"`
	// TotalRelation is "eq" when Total is exact, "gte" when it is a lower
	// bound. Always populated.
	TotalRelation string `json:"totalRelation"`
	TookMs        int64  `json:"tookMs"`
	// NextCursor is empty on the last page; its absence is the only
	// end-of-results signal.
	NextCursor string `json:"nextCursor,omitempty"`

	// Timeline is omitted unless asked for and computable. Absent means
	// "unknown", never "no activity".
	Timeline *AuditLogTimeline `json:"timeline,omitempty"`
}

// AuditLogFilterValuesRequest is the decoded body of
// POST /api/v1alpha1/audit-logs/filter-values. Matches the OpenAPI
// AuditLogFilterValuesRequest schema.
type AuditLogFilterValuesRequest struct {
	// Query carries the window and the filters the values are reached under.
	// Limit, SortOrder, Cursor, IncludeTimeline and TimelineInterval are
	// ignored.
	Query AuditLogsQueryRequest `json:"query"`
	// Filter names the filter to list values for. Its own selections in Query
	// are ignored.
	Filter string `json:"filter"`
	// ValueSearch narrows the values; Query.SearchPhrase narrows the records.
	ValueSearch string `json:"valueSearch,omitempty"`
	// MaxValues caps the list, distinct from Query.Limit.
	MaxValues int `json:"maxValues,omitempty"`
}

// AuditLogFilterValue is one value a filter takes, with how many matching
// records carry it. Matches the OpenAPI AuditLogFilterValue schema.
type AuditLogFilterValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// AuditLogFilterValuesResponse is the response for
// POST /api/v1alpha1/audit-logs/filter-values. Matches the OpenAPI
// AuditLogFilterValuesResponse schema.
type AuditLogFilterValuesResponse struct {
	// Filter echoes the request, for a client driving several pickers.
	Filter string                `json:"filter"`
	Values []AuditLogFilterValue `json:"values"`
	// TotalValues is how many distinct values match, of which at most
	// MaxValues were returned. TotalRelation says whether it is exact.
	TotalValues   int64  `json:"totalValues"`
	TotalRelation string `json:"totalRelation"`
	TookMs        int64  `json:"tookMs"`
}

// AuditLogTimelineBucket is one interval of the timeline. Matches the OpenAPI
// AuditLogTimelineBucket schema.
type AuditLogTimelineBucket struct {
	StartTime string `json:"startTime"`
	// Total equals the sum of Counts, carried separately so a bucket with no
	// breakdown still reports a height.
	Total int64 `json:"total"`
	// Counts is records by result, keyed by the result value itself. A missing
	// key means zero.
	Counts map[string]int64 `json:"counts,omitempty"`
}

// AuditLogTimeline holds per-interval counts across the queried window, broken
// down by result. Matches the OpenAPI AuditLogTimeline schema.
type AuditLogTimeline struct {
	// Interval is the width actually used, not necessarily the one requested.
	Interval string `json:"interval"`
	// Buckets covers the window contiguously in ascending StartTime order,
	// whatever the query's sort order, with empty buckets present at zero.
	Buckets []AuditLogTimelineBucket `json:"buckets"`
}
