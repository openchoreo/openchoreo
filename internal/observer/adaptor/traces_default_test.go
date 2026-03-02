// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

const (
	testTraceID = "trace-1"
	testSpanID  = "span-1"
)

func TestConvertToObservabilityTrace_SingleSpan(t *testing.T) {
	now := time.Now()
	spans := []opensearch.Span{
		{
			SpanID:              testSpanID,
			Name:                "http.request",
			ParentSpanID:        "",
			StartTime:           now,
			EndTime:             now.Add(100 * time.Millisecond),
			DurationNanoseconds: 100000000,
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	if trace.TraceID != testTraceID {
		t.Errorf("Expected traceId %q, got %s", testTraceID, trace.TraceID)
	}

	if trace.SpanCount != 1 {
		t.Errorf("Expected 1 span, got %d", trace.SpanCount)
	}

	if trace.RootSpanID != testSpanID {
		t.Errorf("Expected rootSpanId %q, got %s", testSpanID, trace.RootSpanID)
	}

	if trace.RootSpanName != "http.request" {
		t.Errorf("Expected rootSpanName 'http.request', got %s", trace.RootSpanName)
	}
}

func TestConvertToObservabilityTrace_MultipleSpans(t *testing.T) {
	now := time.Now()
	spans := []opensearch.Span{
		{
			SpanID:              testSpanID,
			Name:                "http.request",
			ParentSpanID:        "",
			StartTime:           now,
			EndTime:             now.Add(100 * time.Millisecond),
			DurationNanoseconds: 100000000,
		},
		{
			SpanID:              "span-2",
			Name:                "db.query",
			ParentSpanID:        testSpanID,
			StartTime:           now.Add(20 * time.Millisecond),
			EndTime:             now.Add(80 * time.Millisecond),
			DurationNanoseconds: 60000000,
		},
		{
			SpanID:              "span-3",
			Name:                "cache.get",
			ParentSpanID:        testSpanID,
			StartTime:           now.Add(90 * time.Millisecond),
			EndTime:             now.Add(95 * time.Millisecond),
			DurationNanoseconds: 5000000,
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	if trace.TraceID != testTraceID {
		t.Errorf("Expected traceId %q, got %s", testTraceID, trace.TraceID)
	}

	if trace.SpanCount != 3 {
		t.Errorf("Expected 3 spans, got %d", trace.SpanCount)
	}

	if trace.RootSpanID != testSpanID {
		t.Errorf("Expected rootSpanId %q, got %s", testSpanID, trace.RootSpanID)
	}

	if len(trace.Spans) != 3 {
		t.Errorf("Expected 3 spans in trace, got %d", len(trace.Spans))
	}
}

func TestConvertToObservabilityTrace_EmptySpans(t *testing.T) {
	spans := []opensearch.Span{}
	trace := convertToObservabilityTrace(testTraceID, spans)

	if trace.TraceID != testTraceID {
		t.Errorf("Expected traceId %q, got %s", testTraceID, trace.TraceID)
	}

	if trace.SpanCount != 0 {
		t.Errorf("Expected 0 spans, got %d", trace.SpanCount)
	}
}

func TestConvertToObservabilityTrace_RootSpanDetection(t *testing.T) {
	now := time.Now()
	// Test that the first span with no parent is detected as root
	spans := []opensearch.Span{
		{
			SpanID:       testSpanID,
			Name:         "http.request",
			ParentSpanID: "span-0", // This has a parent, but span-0 is not in the list
			StartTime:    now,
			EndTime:      now.Add(100 * time.Millisecond),
		},
		{
			SpanID:       "span-2",
			Name:         "db.query",
			ParentSpanID: "", // This has no parent - should be detected as root
			StartTime:    now.Add(20 * time.Millisecond),
			EndTime:      now.Add(80 * time.Millisecond),
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	// The root span should be the one with no parent
	if trace.RootSpanID != "span-2" {
		t.Errorf("Expected rootSpanId 'span-2', got %s", trace.RootSpanID)
	}

	if trace.RootSpanName != "db.query" {
		t.Errorf("Expected rootSpanName 'db.query', got %s", trace.RootSpanName)
	}
}

func TestConvertToObservabilityTrace_TraceName(t *testing.T) {
	now := time.Now()
	spans := []opensearch.Span{
		{
			SpanID:       testSpanID,
			Name:         "user.request",
			ParentSpanID: "",
			StartTime:    now,
			EndTime:      now.Add(100 * time.Millisecond),
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	if trace.TraceName != "user.request" {
		t.Errorf("Expected traceName 'user.request', got %s", trace.TraceName)
	}
}

func TestConvertToObservabilityTrace_TimeCalculation(t *testing.T) {
	startTime := time.Now()
	endTime := startTime.Add(500 * time.Millisecond)

	spans := []opensearch.Span{
		{
			SpanID:              testSpanID,
			Name:                "http.request",
			ParentSpanID:        "",
			StartTime:           startTime,
			EndTime:             endTime,
			DurationNanoseconds: 500000000,
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	expectedDuration := int64(500000000)
	if trace.DurationNs != expectedDuration {
		t.Errorf("Expected duration %d, got %d", expectedDuration, trace.DurationNs)
	}

	if !trace.StartTime.Equal(startTime) {
		t.Errorf("Expected start time %v, got %v", startTime, trace.StartTime)
	}

	if !trace.EndTime.Equal(endTime) {
		t.Errorf("Expected end time %v, got %v", endTime, trace.EndTime)
	}
}

func TestConvertToObservabilityTrace_SpanAttributes(t *testing.T) {
	now := time.Now()
	attrs := map[string]interface{}{
		"http.method": "POST",
		"http.url":    "http://example.com",
	}
	resourceAttrs := map[string]interface{}{
		"service.name": "my-service",
	}

	spans := []opensearch.Span{
		{
			SpanID:              testSpanID,
			Name:                "http.request",
			ParentSpanID:        "",
			StartTime:           now,
			EndTime:             now.Add(100 * time.Millisecond),
			DurationNanoseconds: 100000000,
			Attributes:          attrs,
			ResourceAttributes:  resourceAttrs,
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	if len(trace.Spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(trace.Spans))
	}

	span := trace.Spans[0]
	if span.Attributes == nil {
		t.Errorf("Expected attributes, got nil")
	}
	if span.ResourceAttributes == nil {
		t.Errorf("Expected resource attributes, got nil")
	}
}

func TestConvertToObservabilityTrace_ComplexHierarchy(t *testing.T) {
	now := time.Now()
	spans := []opensearch.Span{
		{
			SpanID:       testSpanID,
			Name:         "http.request",
			ParentSpanID: "",
			StartTime:    now,
			EndTime:      now.Add(200 * time.Millisecond),
		},
		{
			SpanID:       "span-2",
			Name:         "db.query.1",
			ParentSpanID: testSpanID,
			StartTime:    now.Add(10 * time.Millisecond),
			EndTime:      now.Add(100 * time.Millisecond),
		},
		{
			SpanID:       "span-3",
			Name:         "db.query.2",
			ParentSpanID: testSpanID,
			StartTime:    now.Add(110 * time.Millisecond),
			EndTime:      now.Add(200 * time.Millisecond),
		},
		{
			SpanID:       "span-4",
			Name:         "cache.set",
			ParentSpanID: "span-2",
			StartTime:    now.Add(50 * time.Millisecond),
			EndTime:      now.Add(60 * time.Millisecond),
		},
	}

	trace := convertToObservabilityTrace(testTraceID, spans)

	if trace.SpanCount != 4 {
		t.Errorf("Expected 4 spans, got %d", trace.SpanCount)
	}

	// Verify the root span is correctly identified
	if trace.RootSpanID != testSpanID {
		t.Errorf("Expected rootSpanId %q, got %s", testSpanID, trace.RootSpanID)
	}

	// Verify all spans are in the trace
	if len(trace.Spans) != 4 {
		t.Errorf("Expected 4 spans in trace, got %d", len(trace.Spans))
	}
}
