// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	assert.Equal(t, testTraceID, trace.TraceID)
	assert.Equal(t, 1, trace.SpanCount)
	assert.Equal(t, testSpanID, trace.RootSpanID)
	assert.Equal(t, "http.request", trace.RootSpanName)
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

	assert.Equal(t, testTraceID, trace.TraceID)
	assert.Equal(t, 3, trace.SpanCount)
	assert.Equal(t, testSpanID, trace.RootSpanID)
	assert.Len(t, trace.Spans, 3)
}

func TestConvertToObservabilityTrace_EmptySpans(t *testing.T) {
	spans := []opensearch.Span{}
	trace := convertToObservabilityTrace(testTraceID, spans)

	assert.Equal(t, testTraceID, trace.TraceID)
	assert.Equal(t, 0, trace.SpanCount)
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
	assert.Equal(t, "span-2", trace.RootSpanID)
	assert.Equal(t, "db.query", trace.RootSpanName)
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

	assert.Equal(t, "user.request", trace.TraceName)
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

	assert.Equal(t, int64(500000000), trace.DurationNs)
	assert.True(t, trace.StartTime.Equal(startTime), "Expected start time %v, got %v", startTime, trace.StartTime)
	assert.True(t, trace.EndTime.Equal(endTime), "Expected end time %v, got %v", endTime, trace.EndTime)
}

func TestConvertToObservabilityTrace_SpanAttributes(t *testing.T) {
	now := time.Now()
	attrs := map[string]any{
		"http.method": "POST",
		"http.url":    "http://example.com",
	}
	resourceAttrs := map[string]any{
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

	require.Len(t, trace.Spans, 1)

	span := trace.Spans[0]
	assert.NotNil(t, span.Attributes)
	assert.NotNil(t, span.ResourceAttributes)
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

	assert.Equal(t, 4, trace.SpanCount)

	// Verify the root span is correctly identified
	assert.Equal(t, testSpanID, trace.RootSpanID)

	// Verify all spans are in the trace
	assert.Len(t, trace.Spans, 4)
}
