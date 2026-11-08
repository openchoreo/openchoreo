// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/json"
	"time"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

// RawTracesQueryRequest represents the internal request for a raw trace query:
// the tracing backend's native request and its media type (application/json
// or text/plain).
type RawTracesQueryRequest struct {
	ContentType string
	Body        []byte
}

// RawTracesQueryResponse carries the tracing backend's own response unchanged.
type RawTracesQueryResponse struct {
	Body json.RawMessage
}

// TracesQueryRequest represents the internal request for querying traces
type TracesQueryRequest struct {
	StartTime         time.Time
	EndTime           time.Time
	Limit             int
	SortOrder         string
	IncludeAttributes bool
	SearchScope       ComponentSearchScope
}

// TracesQueryResponse represents the internal response for trace queries
type TracesQueryResponse struct {
	Traces []TraceInfo `json:"traces"`
	Total  int         `json:"total"`
	TookMs int         `json:"tookMs"`
}

// TraceInfo contains summary information about a trace
type TraceInfo struct {
	TraceID      string     `json:"traceId"`
	TraceName    string     `json:"traceName"`
	SpanCount    int        `json:"spanCount"`
	RootSpanID   string     `json:"rootSpanId"`
	RootSpanName string     `json:"rootSpanName"`
	RootSpanKind string     `json:"rootSpanKind"`
	StartTime    *time.Time `json:"startTime,omitempty"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	DurationNs   int64      `json:"durationNs,omitempty"`
	HasErrors    bool       `json:"hasErrors"`
}

// SpansQueryResponse represents the internal response for span queries
type SpansQueryResponse struct {
	Spans  []SpanInfo `json:"spans"`
	Total  int        `json:"total"`
	TookMs int        `json:"tookMs"`
}

// SpanInfo contains information about a span
type SpanInfo struct {
	SpanID             string                    `json:"spanId"`
	SpanName           string                    `json:"spanName"`
	SpanKind           string                    `json:"spanKind,omitempty"`
	ParentSpanID       string                    `json:"parentSpanId,omitempty"`
	StartTime          *time.Time                `json:"startTime,omitempty"`
	EndTime            *time.Time                `json:"endTime,omitempty"`
	DurationNs         int64                     `json:"durationNs,omitempty"`
	Status             *observability.SpanStatus `json:"status,omitempty"`
	Attributes         map[string]interface{}    `json:"attributes,omitempty"`
	ResourceAttributes map[string]interface{}    `json:"resourceAttributes,omitempty"`
}
