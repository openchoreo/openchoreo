// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"time"
)

// AuditLogsActorFilter filters the record's actor group. Grouped rather than
// flattened so the mapping to the wire is a field-for-field copy, which is
// where a rename bug would otherwise live.
type AuditLogsActorFilter struct {
	IDs   []string `json:"ids"`
	Types []string `json:"types"`
	// Issuers is the namespace an ID is unique within. On a deployment with
	// more than one identity provider, filtering on IDs alone conflates two
	// subjects that share a sub.
	Issuers    []string `json:"issuers"`
	SessionIDs []string `json:"sessionIds"`
	// Entitlements matches the values of every claim in the record's
	// actor.entitlements map rather than one named claim, since the claim key
	// varies by subject kind — "groups" for a user, "sub" for a service
	// account.
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
// Filters mirror the record they match, so a stored field maps to a filter
// without a lookup table.
//
// The fields under Resource are filters over the stored record, not scopes: the
// observer authorizes an audit query at cluster scope before it reaches an
// adapter, so an adapter must never read them as a permission.
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
	// Cursor is an opaque continuation token the adapter minted and is the only
	// component that interprets it. Passed through unparsed.
	Cursor string `json:"cursor"`

	// IncludeTimeline asks for per-interval counts across the window. No page
	// of records can be bucketed into a timeline, so a caller that wants a
	// histogram has to ask for one. Opt-in: it costs an aggregation pass, and
	// the answer describes the query rather than the page, so a caller
	// paginating asks only for the first one.
	//
	// The only aggregation on this query. Per-filter distinct values are not
	// requested here — one aggregation per filter rather than one in total is
	// enough load to matter on a busy trail — and are planned as their own
	// operation.
	IncludeTimeline bool `json:"includeTimeline"`
	// TimelineInterval is the requested bucket width in "<count><unit>"
	// notation (m, h, d, w). Empty leaves the width to the adapter. A width
	// that would exceed 500 buckets is coarsened rather than rejected, and the
	// width used is reported back on AuditLogTimeline.
	TimelineInterval string `json:"timelineInterval"`
}

// AuditLogActor identifies who performed an audited action. ID is unique only
// within Issuer: the same sub from two identity providers is two subjects.
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
// collector rather than by the emitting service. It cross-checks Producer: the
// two disagreeing means a record's claimed origin and its actual one differ.
type AuditLogCollectorInfo struct {
	NamespaceName string `json:"namespaceName,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

// AuditLogRecord is one stored audit event. It mirrors the published event
// rather than introducing a shape of its own, so a queried record and an
// exported log line carry the same fields — Category and Result stay plain
// strings for that reason, since their vocabulary grows with SchemaVersion.
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
	// Log is the raw line the collector ingested, kept beside the parsed
	// fields; it is the ground truth a parsing discrepancy is settled against.
	Log string `json:"log,omitempty"`
}

// AuditLogsTotalRelation says how to read AuditLogsResult.TotalCount.
type AuditLogsTotalRelation string

const (
	// AuditLogsTotalEq means TotalCount is exact.
	AuditLogsTotalEq AuditLogsTotalRelation = "eq"
	// AuditLogsTotalGTE means TotalCount is a lower bound the backend stopped
	// counting at. Distinguished from eq so a capped count cannot be read as
	// exact, which would understate how much activity the trail holds.
	AuditLogsTotalGTE AuditLogsTotalRelation = "gte"
)

// AuditLogTimelineBucket is one interval of a timeline.
type AuditLogTimelineBucket struct {
	StartTime time.Time `json:"startTime"`
	// Total is the records in this bucket, equal to the sum of Counts. Carried
	// separately so a bucket whose breakdown could not be produced still
	// reports a height.
	Total int64 `json:"total"`
	// Counts is records by result, keyed by the result value itself. A map
	// rather than four fields so a result value added in a later schema needs
	// no change here. A missing key means zero.
	Counts map[string]int64 `json:"counts,omitempty"`
}

// AuditLogTimeline holds per-interval counts across the queried window, broken
// down by result.
type AuditLogTimeline struct {
	// Interval is the bucket width actually used, which is not necessarily the
	// one requested: a width exceeding 500 buckets is coarsened.
	Interval string `json:"interval"`
	// Buckets covers the window contiguously in ascending StartTime order,
	// whatever the query's sort order. Buckets with no records are present
	// with zero counts — a sparse series would let a caller draw a continuous
	// chart across a gap in activity.
	Buckets []AuditLogTimelineBucket `json:"buckets"`
}

// AuditLogsResult is the result of an audit trail query.
type AuditLogsResult struct {
	Records       []AuditLogRecord       `json:"records"`
	TotalCount    int64                  `json:"totalCount"`
	TotalRelation AuditLogsTotalRelation `json:"totalRelation"`
	Took          int64                  `json:"took"`
	// NextCursor is empty on the last page. Its absence is the only
	// end-of-results signal; an adapter never reports the end as an empty page
	// of an ongoing scroll.
	NextCursor string `json:"nextCursor,omitempty"`

	// Timeline is nil unless the query asked for it and the adapter can
	// compute it. Nil means "unknown", never "no activity in this window".
	Timeline *AuditLogTimeline `json:"timeline,omitempty"`
}

// AuditLogFilterValuesParams asks for the distinct values one filter takes
// under a query.
//
// One filter per call: answering every filter at once costs one aggregation per
// filter rather than one in total, which on a busy trail is enough load to
// matter on every keystroke that changes a query.
type AuditLogFilterValuesParams struct {
	// Query carries the window and the filters the values are reached under.
	// Its Limit, SortOrder, Cursor, IncludeTimeline and TimelineInterval carry
	// no meaning here and are ignored; StartTime, EndTime and SearchPhrase are
	// honored.
	Query AuditLogsParams `json:"query"`
	// Filter names the filter to list values for, by its path in the query
	// vocabulary — "actor.id", "resource.namespace", "action". The named
	// filter's own selections in Query are ignored, so a picker keeps offering
	// the alternatives to what is already selected.
	Filter string `json:"filter"`
	// ValueSearch narrows the values returned to those containing it,
	// case-insensitively. Distinct from Query.SearchPhrase, which narrows the
	// records considered.
	ValueSearch string `json:"valueSearch"`
	// MaxValues caps the list, which is ordered by count descending then value
	// ascending so a truncated list holds the busiest values.
	MaxValues int `json:"maxValues"`
}

// AuditLogFilterValue is one value a filter takes, with the number of matching
// records carrying it. The count may be approximate on a high-cardinality
// filter, so it orders a list rather than totalling it.
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
	// MaxValues were returned. Read with TotalRelation: counting distinct
	// values exactly is itself expensive on a high-cardinality field, so an
	// estimate is labeled gte rather than passed off as exact.
	TotalValues   int64                  `json:"totalValues"`
	TotalRelation AuditLogsTotalRelation `json:"totalRelation"`
	Took          int64                  `json:"took"`
}

// AuditLogsAdapter defines the interface for fetching audit trail records and
// the filter values a picker is populated from.
//
// Separate from LogsAdapter because the audit trail is its own signal with its
// own destination and retention, even though the concrete adapter serving it is
// the same process reached over the same logs adapter URL.
type AuditLogsAdapter interface {
	// GetAuditLogs retrieves audit records matching params. An adapter that
	// does not serve the audit trail must report that distinctly rather than
	// returning an empty result — an empty success is indistinguishable from
	// "nothing happened", which is the one answer an audit query must not
	// fabricate.
	GetAuditLogs(ctx context.Context, params AuditLogsParams) (*AuditLogsResult, error)

	// GetAuditLogFilterValues retrieves the distinct values one filter takes.
	// Separately declinable from GetAuditLogs — an adapter may serve the
	// records without being able to aggregate, so a module can ship the
	// records first. Reporting that distinctly matters for the same reason: an
	// empty value list reads as "this filter has no values", which is a
	// different and wrong answer.
	GetAuditLogFilterValues(
		ctx context.Context, params AuditLogFilterValuesParams,
	) (*AuditLogFilterValuesResult, error)
}
