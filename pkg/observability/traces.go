// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"time"
)

// TracingAdapter defines the interface for retrieving traces from an external adapter
type TracingAdapter interface {
	// GetTraces retrieves traces based on query parameters
	GetTraces(ctx context.Context, params TracesQueryParams) (*TracesQueryResult, error)
	// GetSpans retrieves spans for a specific trace
	GetSpans(ctx context.Context, traceID string, params TracesQueryParams) (*SpansResult, error)
	// GetSpanDetails retrieves detailed information about a specific span
	GetSpanDetails(ctx context.Context, traceID string, spanID string) (*SpanDetail, error)
}

// SpanDetail represents detailed information about a single span
type SpanDetail struct {
	SpanID             string                 `json:"spanId"`
	SpanName           string                 `json:"spanName"`
	SpanKind           string                 `json:"spanKind,omitempty"`
	ParentSpanID       string                 `json:"parentSpanId,omitempty"`
	StartTime          time.Time              `json:"startTime"`
	EndTime            time.Time              `json:"endTime"`
	DurationNs         int64                  `json:"durationNs"`
	Attributes         map[string]interface{} `json:"attributes,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resourceAttributes,omitempty"`
}

// SpansResult defines the result structure for span queries
type SpansResult struct {
	Spans      []TraceSpan `json:"spans"`
	TotalCount int         `json:"totalCount"`
	Took       int         `json:"tookMs"`
}

// TracesQueryParams defines parameters for querying traces
type TracesQueryParams struct {
	// Time range
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`

	// Resource identifiers
	Namespace     string `json:"namespace"`
	ProjectID     string `json:"projectId"`
	ComponentID   string `json:"componentId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`

	// Query options
	TraceID   string `json:"traceId,omitempty"`
	Limit     int    `json:"limit"`
	SortOrder string `json:"sortOrder"`
}

// TracesQueryResult defines the result structure for trace queries
type TracesQueryResult struct {
	Traces     []Trace `json:"traces"`
	TotalCount int     `json:"totalCount"`
	Took       int     `json:"tookMs"`
}

// Trace represents a distributed trace
type Trace struct {
	TraceID      string      `json:"traceId"`
	TraceName    string      `json:"traceName"`
	SpanCount    int         `json:"spanCount"`
	RootSpanID   string      `json:"rootSpanId"`
	RootSpanName string      `json:"rootSpanName"`
	RootSpanKind string      `json:"rootSpanKind"`
	StartTime    time.Time   `json:"startTime"`
	EndTime      time.Time   `json:"endTime"`
	DurationNs   int64       `json:"durationNs"`
	Spans        []TraceSpan `json:"spans,omitempty"`
}

// TraceSpan represents a span within a trace with all details
type TraceSpan struct {
	SpanID             string                 `json:"spanId"`
	Name               string                 `json:"name"`
	SpanKind           string                 `json:"spanKind,omitempty"`
	ParentSpanID       string                 `json:"parentSpanId,omitempty"`
	StartTime          time.Time              `json:"startTime"`
	EndTime            time.Time              `json:"endTime"`
	DurationNs         int64                  `json:"durationNs"`
	Attributes         map[string]interface{} `json:"attributes,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resourceAttributes,omitempty"`
}
