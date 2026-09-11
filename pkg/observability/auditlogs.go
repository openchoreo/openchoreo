// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"time"
)

// AuditLogsActorFilter filters the record's actor group.
type AuditLogsActorFilter struct {
	IDs   []string `json:"ids"`
	Types []string `json:"types"`
	// Issuers is the namespace an ID is unique within: filtering on IDs alone
	// conflates two subjects that share a sub across identity providers.
	Issuers    []string `json:"issuers"`
	SessionIDs []string `json:"sessionIds"`
	// Entitlements matches values across every claim in the record's
	// actor.entitlements map: the key varies by subject kind.
	Entitlements []string `json:"entitlements"`
}

// AuditLogsResourceFilter filters the record's resource group.
type AuditLogsResourceFilter struct {
	Types      []string `json:"types"`
	Namespaces []string `json:"namespaces"`
	// Environments are dual-scoped "{namespace}/{name}", the form the record
	// stores, since the value is recorded exactly as authorization evaluated it.
	Environments []string `json:"environments"`
	Projects     []string `json:"projects"`
	Components   []string `json:"components"`
	Names        []string `json:"names"`
}

// AuditLogsParams holds parameters for audit trail queries. Multi-value fields
// OR within a field and AND with each other; an empty field is not a filter.
//
// The fields under Resource are filters, not scopes: the observer authorizes at
// cluster scope before reaching an adapter.
type AuditLogsParams struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`

	Actor    AuditLogsActorFilter    `json:"actor"`
	Resource AuditLogsResourceFilter `json:"resource"`

	Actions      []string `json:"actions"`
	Categories   []string `json:"categories"`
	Results      []string `json:"results"`
	Producers    []string `json:"producers"`
	Surfaces     []string `json:"surfaces"`
	OperationIDs []string `json:"operationIds"`
	RequestIDs   []string `json:"requestIds"`
	EventIDs     []string `json:"eventIds"`
	SourceIPs    []string `json:"sourceIps"`
	UserAgents   []string `json:"userAgents"`

	SearchPhrase string `json:"searchPhrase"`

	Limit     int    `json:"limit"`
	SortOrder string `json:"sortOrder"`
	// Cursor is opaque; only the adapter that minted it interprets it.
	Cursor string `json:"cursor"`

	// IncludeTimeline asks for per-interval counts across the window. Opt-in:
	// it costs an aggregation pass and describes the query rather than the
	// page, so a paginating caller asks only on the first.
	IncludeTimeline bool `json:"includeTimeline"`
	// TimelineInterval is the requested bucket width in "<count><unit>"
	// notation (m, h, d, w). Empty leaves the width to the adapter, which also
	// coarsens anything exceeding 500 buckets.
	TimelineInterval string `json:"timelineInterval"`
}

// AuditLogActor identifies who performed an audited action. ID is unique only
// within Issuer.
type AuditLogActor struct {
	Type         string              `json:"type"`
	ID           string              `json:"id"`
	Issuer       string              `json:"issuer,omitempty"`
	SessionID    string              `json:"sessionId,omitempty"`
	Entitlements map[string][]string `json:"entitlements,omitempty"`
}

// AuditLogHTTPInfo is the request line of an event that arrived over HTTP. Nil
// for an MCP tools/call, which has none.
type AuditLogHTTPInfo struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// AuditLogResource is the target resource of an audited action together with
// the point in OpenChoreo's tree the decision was authorized at. Nil on a
// rejection that resolved no operation.
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
// collector rather than by the emitting service. Carried so a caller can
// compare it against Producer; nothing here performs that comparison.
type AuditLogCollectorInfo struct {
	NamespaceName string `json:"namespaceName,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

// AuditLogRecord is one stored audit event, mirroring the published event so a
// queried record and an exported log line carry the same fields. Category and
// Result stay plain strings: their vocabulary grows with SchemaVersion.
type AuditLogRecord struct {
	SchemaVersion string                 `json:"schemaVersion"`
	EventID       string                 `json:"eventId"`
	EventTime     time.Time              `json:"eventTime"`
	Actor         AuditLogActor          `json:"actor"`
	Action        string                 `json:"action"`
	Category      string                 `json:"category"`
	Result        string                 `json:"result"`
	RequestID     string                 `json:"requestId,omitempty"`
	SourceIP      string                 `json:"sourceIp,omitempty"`
	UserAgent     string                 `json:"userAgent,omitempty"`
	Producer      string                 `json:"producer,omitempty"`
	Surface       string                 `json:"surface,omitempty"`
	OperationID   string                 `json:"operationId,omitempty"`
	HTTP          *AuditLogHTTPInfo      `json:"http,omitempty"`
	Resource      *AuditLogResource      `json:"resource,omitempty"`
	Metadata      map[string]any         `json:"metadata,omitempty"`
	Collector     *AuditLogCollectorInfo `json:"collector,omitempty"`
	// Log is the raw ingested line, the ground truth for a parsing discrepancy.
	Log string `json:"log,omitempty"`
}

// AuditLogsTotalRelation says how to read AuditLogsResult.TotalCount.
type AuditLogsTotalRelation string

const (
	// AuditLogsTotalEq means TotalCount is exact.
	AuditLogsTotalEq AuditLogsTotalRelation = "eq"
	// AuditLogsTotalGTE means TotalCount is a lower bound the backend stopped
	// counting at, so a capped count is not read as exact.
	AuditLogsTotalGTE AuditLogsTotalRelation = "gte"
)

// AuditLogTimelineBucket is one interval of a timeline.
type AuditLogTimelineBucket struct {
	StartTime time.Time `json:"startTime"`
	// Total equals the sum of Counts, carried separately so a bucket whose
	// breakdown could not be produced still reports a height.
	Total int64 `json:"total"`
	// Counts is records by result, keyed by the result value. A missing key
	// means zero.
	Counts map[string]int64 `json:"counts,omitempty"`
}

// AuditLogTimeline holds per-interval counts across the queried window, broken
// down by result.
type AuditLogTimeline struct {
	// Interval is the width actually used, not necessarily the one requested.
	Interval string `json:"interval"`
	// Buckets covers the window contiguously in ascending StartTime order,
	// empty buckets included — a sparse series would let a caller draw a
	// continuous chart across a gap in activity.
	Buckets []AuditLogTimelineBucket `json:"buckets"`
}

// AuditLogsResult is the result of an audit trail query.
type AuditLogsResult struct {
	Records       []AuditLogRecord       `json:"records"`
	TotalCount    int64                  `json:"totalCount"`
	TotalRelation AuditLogsTotalRelation `json:"totalRelation"`
	Took          int64                  `json:"took"`
	// NextCursor is empty on the last page, and its absence is the only
	// end-of-results signal.
	NextCursor string `json:"nextCursor,omitempty"`

	// Timeline is nil unless asked for and computable. Nil means "unknown",
	// never "no activity".
	Timeline *AuditLogTimeline `json:"timeline,omitempty"`
}

// AuditLogFilterValuesParams asks for the distinct values one filter takes
// under a query. One filter per call, to keep the aggregation cost
// proportional to what a caller needs.
type AuditLogFilterValuesParams struct {
	// Query carries the window and the filters the values are reached under.
	// Limit, SortOrder, Cursor, IncludeTimeline and TimelineInterval are
	// ignored.
	Query AuditLogsParams `json:"query"`
	// Filter names the filter to list values for, by its path in the query
	// vocabulary. Its own selections in Query are ignored, so a picker keeps
	// offering the alternatives to what is selected.
	Filter string `json:"filter"`
	// ValueSearch narrows the values returned; Query.SearchPhrase narrows the
	// records they are drawn from.
	ValueSearch string `json:"valueSearch"`
	// MaxValues caps the list, ordered by count descending then value ascending.
	MaxValues int `json:"maxValues"`
}

// AuditLogFilterValue is one value a filter takes. Count may be approximate on
// a high-cardinality filter, so it orders a list rather than totalling it.
type AuditLogFilterValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// AuditLogFilterValuesResult is the distinct values one filter takes under a
// query.
type AuditLogFilterValuesResult struct {
	// Filter echoes the filter these values belong to.
	Filter string                `json:"filter"`
	Values []AuditLogFilterValue `json:"values"`
	// TotalValues is how many distinct values match, of which at most
	// MaxValues were returned. Read with TotalRelation.
	TotalValues   int64                  `json:"totalValues"`
	TotalRelation AuditLogsTotalRelation `json:"totalRelation"`
	Took          int64                  `json:"took"`
}

// AuditLogsAdapter fetches audit trail records and the filter values a picker
// is populated from. Separate from LogsAdapter because the audit trail is its
// own signal, though the same process serves both.
type AuditLogsAdapter interface {
	// GetAuditLogs retrieves audit records matching params. An adapter that
	// does not serve the trail must report that distinctly: an empty result is
	// indistinguishable from "nothing happened".
	GetAuditLogs(ctx context.Context, params AuditLogsParams) (*AuditLogsResult, error)

	// GetAuditLogFilterValues retrieves the distinct values one filter takes.
	// Separately declinable from GetAuditLogs: an adapter may serve records
	// without being able to aggregate.
	GetAuditLogFilterValues(
		ctx context.Context, params AuditLogFilterValuesParams,
	) (*AuditLogFilterValuesResult, error)
}
