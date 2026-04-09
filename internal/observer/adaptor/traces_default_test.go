// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"encoding/json"
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
			SpanKind:            "SERVER",
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
	assert.Equal(t, "SERVER", trace.RootSpanKind)
	assert.Equal(t, "SERVER", trace.Spans[0].SpanKind)
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

func TestParseTracesAggregation(t *testing.T) {
	t.Run("Valid aggregation response", func(t *testing.T) {
		raw := json.RawMessage(`{
			"trace_count": { "value": 42 },
			"traces": {
				"buckets": [
					{
						"key": "trace-aaa",
						"doc_count": 5,
						"earliest_span": {
							"hits": {
								"hits": [
									{
										"_id": "hit1",
										"_source": { "spanId": "span-root", "name": "GET /api", "parentSpanId": "", "startTime": "2024-01-01T00:00:00.123456789Z" }
									}
								]
							}
						},
						"root_span": {
							"doc_count": 1,
							"hit": {
								"hits": {
									"hits": [
										{
											"_id": "hit1-root",
											"_source": { "spanId": "span-root", "name": "GET /api" }
										}
									]
								}
							}
						},
						"latest_span": {
							"hits": {
								"hits": [
									{
										"_id": "hit1-end",
										"_source": { "endTime": "2024-01-01T00:00:01.987654321Z" }
									}
								]
							}
						},
						"min_start_time": { "value": 1704067200123 }
					},
					{
						"key": "trace-bbb",
						"doc_count": 3,
						"earliest_span": {
							"hits": {
								"hits": [
									{
										"_id": "hit2",
										"_source": { "spanId": "span-root-2", "name": "POST /submit", "parentSpanId": "", "startTime": "2024-01-01T00:00:02.111222333Z" }
									}
								]
							}
						},
						"root_span": {
							"doc_count": 1,
							"hit": {
								"hits": {
									"hits": [
										{
											"_id": "hit2-root",
											"_source": { "spanId": "span-root-2", "name": "POST /submit" }
										}
									]
								}
							}
						},
						"latest_span": {
							"hits": {
								"hits": [
									{
										"_id": "hit2-end",
										"_source": { "endTime": "2024-01-01T00:00:03.444555666Z" }
									}
								]
							}
						},
						"min_start_time": { "value": 1704067202111 }
					}
				]
			}
		}`)

		result, err := parseTracesAggregation(raw)
		require.NoError(t, err)

		assert.Equal(t, 42, result.TraceCount.Value)
		assert.Len(t, result.Traces.Buckets, 2)

		bucket := result.Traces.Buckets[0]
		assert.Equal(t, "trace-aaa", bucket.Key)
		assert.Equal(t, 5, bucket.DocCount)

		require.Len(t, bucket.EarliestSpan.Hits.Hits, 1)
		earliestSource := bucket.EarliestSpan.Hits.Hits[0].Source
		assert.Equal(t, "span-root", earliestSource["spanId"])
		assert.Equal(t, "GET /api", earliestSource["name"])
		assert.Equal(t, "2024-01-01T00:00:00.123456789Z", earliestSource["startTime"])

		require.Len(t, bucket.LatestSpan.Hits.Hits, 1)
		latestSource := bucket.LatestSpan.Hits.Hits[0].Source
		assert.Equal(t, "2024-01-01T00:00:01.987654321Z", latestSource["endTime"])
	})

	t.Run("Empty aggregation", func(t *testing.T) {
		result, err := parseTracesAggregation(nil)
		require.NoError(t, err)
		assert.Equal(t, 0, result.TraceCount.Value)
		assert.Empty(t, result.Traces.Buckets)
	})
}

func TestParseTracesAggregation_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid json}`)
	_, err := parseTracesAggregation(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal aggregation result")
}

func TestBuildTraceFromBucket_EmptyBucket(t *testing.T) {
	bucket := opensearch.TraceBucket{
		Key:      "trace-empty",
		DocCount: 0,
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-empty", trace.TraceID)
	assert.Equal(t, 0, trace.SpanCount)
	assert.Empty(t, trace.RootSpanID)
	assert.Empty(t, trace.RootSpanName)
	assert.True(t, trace.StartTime.IsZero())
	assert.True(t, trace.EndTime.IsZero())
	assert.Equal(t, int64(0), trace.DurationNs)
}

func TestBuildTraceFromBucket_NilSource(t *testing.T) {
	bucket := opensearch.TraceBucket{
		Key:      "trace-nil",
		DocCount: 1,
		EarliestSpan: opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit1", Source: nil},
				},
			},
		},
		LatestSpan: opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit2", Source: nil},
				},
			},
		},
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-nil", trace.TraceID)
	assert.Equal(t, 1, trace.SpanCount)
	assert.Empty(t, trace.RootSpanID)
	assert.True(t, trace.StartTime.IsZero())
	assert.True(t, trace.EndTime.IsZero())
	assert.Equal(t, int64(0), trace.DurationNs)
}

func TestBuildTraceFromBucket_InvalidTimestamps(t *testing.T) {
	makeTopHits := func(source map[string]interface{}) opensearch.AggTopHitsValue {
		return opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit1", Source: source},
				},
			},
		}
	}

	bucket := opensearch.TraceBucket{
		Key:      "trace-bad-ts",
		DocCount: 1,
		EarliestSpan: makeTopHits(map[string]interface{}{
			"spanId":    "span-1",
			"name":      "test",
			"startTime": "not-a-timestamp",
		}),
		LatestSpan: makeTopHits(map[string]interface{}{
			"endTime": "also-not-a-timestamp",
		}),
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-bad-ts", trace.TraceID)
	assert.Equal(t, "span-1", trace.RootSpanID)
	assert.Equal(t, "test", trace.RootSpanName)
	assert.True(t, trace.StartTime.IsZero())
	assert.True(t, trace.EndTime.IsZero())
	assert.Equal(t, int64(0), trace.DurationNs)
}

func TestBuildTraceFromBucket_OnlyStartTime(t *testing.T) {
	makeTopHits := func(source map[string]interface{}) opensearch.AggTopHitsValue {
		return opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit1", Source: source},
				},
			},
		}
	}

	bucket := opensearch.TraceBucket{
		Key:      "trace-partial",
		DocCount: 1,
		EarliestSpan: makeTopHits(map[string]interface{}{
			"spanId":    "span-1",
			"name":      "test",
			"startTime": "2024-01-01T10:00:00Z",
		}),
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-partial", trace.TraceID)
	assert.False(t, trace.StartTime.IsZero())
	assert.True(t, trace.EndTime.IsZero())
	assert.Equal(t, int64(0), trace.DurationNs)
}

func TestBuildTraceFromBucket(t *testing.T) {
	makeTopHits := func(source map[string]interface{}) opensearch.AggTopHitsValue {
		return opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit1", Source: source},
				},
			},
		}
	}

	makeFilteredTopHits := func(source map[string]interface{}) opensearch.AggFilteredTopHits {
		return opensearch.AggFilteredTopHits{
			DocCount: 1,
			Hit: opensearch.AggTopHitsValue{
				Hits: struct {
					Hits []opensearch.Hit `json:"hits"`
				}{
					Hits: []opensearch.Hit{
						{ID: "root-hit", Source: source},
					},
				},
			},
		}
	}

	bucket := opensearch.TraceBucket{
		Key:      "trace-123",
		DocCount: 7,
		EarliestSpan: makeTopHits(map[string]interface{}{
			"spanId":       "root-span-id",
			"name":         "GET /users",
			"parentSpanId": "",
			"startTime":    "2024-01-01T10:00:00.123456789Z",
		}),
		RootSpan: makeFilteredTopHits(map[string]interface{}{
			"spanId": "root-span-id",
			"name":   "GET /users",
			"kind":   "SERVER",
		}),
		LatestSpan: makeTopHits(map[string]interface{}{
			"endTime": "2024-01-01T10:00:02.623456789Z",
		}),
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-123", trace.TraceID)
	assert.Equal(t, 7, trace.SpanCount)
	assert.Equal(t, "root-span-id", trace.RootSpanID)
	assert.Equal(t, "GET /users", trace.RootSpanName)
	assert.Equal(t, "GET /users", trace.TraceName)
	assert.Equal(t, "SERVER", trace.RootSpanKind)
	assert.Equal(t, int64(2500000000), trace.DurationNs)
	assert.Equal(t, "2024-01-01T10:00:00.123456789Z", trace.StartTime.Format(time.RFC3339Nano))
	assert.Equal(t, "2024-01-01T10:00:02.623456789Z", trace.EndTime.Format(time.RFC3339Nano))
}

func TestBuildTraceFromBucket_RootSpanDiffersFromEarliestSpan(t *testing.T) {
	makeTopHits := func(source map[string]interface{}) opensearch.AggTopHitsValue {
		return opensearch.AggTopHitsValue{
			Hits: struct {
				Hits []opensearch.Hit `json:"hits"`
			}{
				Hits: []opensearch.Hit{
					{ID: "hit1", Source: source},
				},
			},
		}
	}

	makeFilteredTopHits := func(source map[string]interface{}) opensearch.AggFilteredTopHits {
		return opensearch.AggFilteredTopHits{
			DocCount: 1,
			Hit: opensearch.AggTopHitsValue{
				Hits: struct {
					Hits []opensearch.Hit `json:"hits"`
				}{
					Hits: []opensearch.Hit{
						{ID: "root-hit", Source: source},
					},
				},
			},
		}
	}

	bucket := opensearch.TraceBucket{
		Key:      "trace-456",
		DocCount: 5,
		EarliestSpan: makeTopHits(map[string]interface{}{
			"spanId":       "child-span-id",
			"name":         "DB query",
			"parentSpanId": "root-span-id",
			"startTime":    "2024-01-01T10:00:00.000000000Z",
		}),
		RootSpan: makeFilteredTopHits(map[string]interface{}{
			"spanId": "root-span-id",
			"name":   "GET /users",
			"kind":   "CLIENT",
		}),
		LatestSpan: makeTopHits(map[string]interface{}{
			"endTime": "2024-01-01T10:00:02.000000000Z",
		}),
	}

	trace := buildTraceFromBucket(bucket)

	assert.Equal(t, "trace-456", trace.TraceID)
	assert.Equal(t, 5, trace.SpanCount)
	// Root span info should come from root_span, not earliest_span
	assert.Equal(t, "root-span-id", trace.RootSpanID)
	assert.Equal(t, "GET /users", trace.RootSpanName)
	assert.Equal(t, "GET /users", trace.TraceName)
	assert.Equal(t, "CLIENT", trace.RootSpanKind)
	// Start time should still come from earliest span
	assert.Equal(t, "2024-01-01T10:00:00Z", trace.StartTime.Format(time.RFC3339Nano))
	assert.Equal(t, int64(2000000000), trace.DurationNs)
}
