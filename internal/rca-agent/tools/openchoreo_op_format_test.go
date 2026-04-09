// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	obstypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestFormatLogs(t *testing.T) {
	t.Parallel()

	t.Run("empty logs", func(t *testing.T) {
		t.Parallel()
		raw := mustJSON(t, obstypes.LogsQueryResponse{Logs: nil})
		assert.Equal(t, "No logs found", formatLogs(raw))
	})

	t.Run("invalid JSON returns raw", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "not json", formatLogs("not json"))
	})

	t.Run("entries grouped by component", func(t *testing.T) {
		t.Parallel()
		data := obstypes.LogsQueryResponse{
			Logs: []obstypes.LogEntry{
				{
					Timestamp: "2026-03-07T10:00:00Z",
					Log:       "connection refused",
					Level:     "ERROR",
					Metadata: &obstypes.LogMetadata{
						ComponentName:   "api-server",
						ProjectName:     "myproj",
						EnvironmentName: "prod",
						NamespaceName:   "default",
					},
				},
				{
					Timestamp: "2026-03-07T10:00:01Z",
					Log:       "retrying",
					Level:     "WARN",
					Metadata: &obstypes.LogMetadata{
						ComponentName:   "api-server",
						ProjectName:     "myproj",
						EnvironmentName: "prod",
						NamespaceName:   "default",
					},
				},
			},
			Total: 2,
		}

		result := formatLogs(mustJSON(t, data))
		assert.Contains(t, result, "api-server")
		assert.Contains(t, result, "connection refused")
		assert.Contains(t, result, "retrying")
	})

	t.Run("multiple components", func(t *testing.T) {
		t.Parallel()
		data := obstypes.LogsQueryResponse{
			Logs: []obstypes.LogEntry{
				{Timestamp: "2026-03-07T10:00:00Z", Log: "log from A", Metadata: &obstypes.LogMetadata{ComponentName: "comp-a"}},
				{Timestamp: "2026-03-07T10:00:01Z", Log: "log from B", Metadata: &obstypes.LogMetadata{ComponentName: "comp-b"}},
			},
		}

		result := formatLogs(mustJSON(t, data))
		assert.Contains(t, result, "comp-a")
		assert.Contains(t, result, "comp-b")
	})
}

func TestFormatMetrics(t *testing.T) {
	t.Parallel()

	t.Run("empty metrics", func(t *testing.T) {
		t.Parallel()
		raw := mustJSON(t, obstypes.ResourceMetricsQueryResponse{})
		assert.Equal(t, "No metrics data available", formatMetrics(raw))
	})

	t.Run("invalid JSON returns raw", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "{bad}", formatMetrics("{bad}"))
	})

	t.Run("with cpu and memory data", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
		data := obstypes.ResourceMetricsQueryResponse{
			CPUUsage: []obstypes.MetricsTimeSeriesItem{
				{Timestamp: now, Value: 0.5},
				{Timestamp: now.Add(time.Minute), Value: 0.6},
				{Timestamp: now.Add(2 * time.Minute), Value: 0.55},
			},
			CPURequests: []obstypes.MetricsTimeSeriesItem{
				{Timestamp: now, Value: 1.0},
			},
			MemoryUsage: []obstypes.MetricsTimeSeriesItem{
				{Timestamp: now, Value: 512 * 1024 * 1024},
				{Timestamp: now.Add(time.Minute), Value: 520 * 1024 * 1024},
			},
			MemoryRequests: []obstypes.MetricsTimeSeriesItem{
				{Timestamp: now, Value: 1024 * 1024 * 1024},
			},
		}

		result := formatMetrics(mustJSON(t, data))
		assert.NotEmpty(t, result)
		assert.NotEqual(t, "No metrics data available", result)
	})
}

func TestFormatTraces(t *testing.T) {
	t.Parallel()

	t.Run("empty traces", func(t *testing.T) {
		t.Parallel()
		raw := mustJSON(t, obstypes.TracesQueryResponse{Traces: nil})
		assert.Equal(t, "No traces found", formatTraces(raw))
	})

	t.Run("invalid JSON returns raw", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "invalid", formatTraces("invalid"))
	})

	t.Run("with trace data", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
		data := obstypes.TracesQueryResponse{
			Traces: []obstypes.TraceInfo{
				{
					TraceID:    "abc123",
					TraceName:  "GET /api/health",
					SpanCount:  5,
					DurationNs: 150_000_000,
					StartTime:  &now,
				},
			},
			Total: 1,
		}

		result := formatTraces(mustJSON(t, data))
		assert.Contains(t, result, "abc123")
		assert.Contains(t, result, "GET /api/health")
	})
}

func TestFormatTraceSpans(t *testing.T) {
	t.Parallel()

	t.Run("empty spans", func(t *testing.T) {
		t.Parallel()
		raw := mustJSON(t, obstypes.SpansQueryResponse{Spans: nil})
		assert.Equal(t, "No spans found", formatTraceSpans(raw))
	})

	t.Run("invalid JSON returns raw", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "bad", formatTraceSpans("bad"))
	})

	t.Run("with parent-child spans", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
		later := now.Add(100 * time.Millisecond)

		data := obstypes.SpansQueryResponse{
			Spans: []obstypes.SpanInfo{
				{
					SpanID: "span-root", SpanName: "GET /api/items",
					StartTime: &now, EndTime: &later, DurationNs: 100_000_000,
					ResourceAttributes: map[string]interface{}{"service.name": "api-gateway"},
				},
				{
					SpanID: "span-child", SpanName: "SELECT items", ParentSpanID: "span-root",
					StartTime: &now, EndTime: &later, DurationNs: 50_000_000,
					ResourceAttributes: map[string]interface{}{"service.name": "db-service"},
				},
			},
			Total: 2,
		}

		result := formatTraceSpans(mustJSON(t, data))
		assert.Contains(t, result, "GET /api/items")
		assert.Contains(t, result, "SELECT items")
		assert.Contains(t, result, "api-gateway")
		assert.Contains(t, result, "db-service")
	})
}

func TestBuildSpanTree_Ordering(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(10 * time.Millisecond)
	t3 := t1.Add(20 * time.Millisecond)

	spans := []obstypes.SpanInfo{
		{SpanID: "c", SpanName: "child-late", ParentSpanID: "a", StartTime: &t3, DurationNs: 5_000_000},
		{SpanID: "b", SpanName: "child-early", ParentSpanID: "a", StartTime: &t2, DurationNs: 5_000_000},
		{SpanID: "a", SpanName: "root", StartTime: &t1, DurationNs: 30_000_000},
	}

	tree := buildSpanTree(spans)
	require.Len(t, tree, 3)
	assert.Equal(t, "root", tree[0].SpanName)
	assert.Equal(t, 0, tree[0].Depth)
	assert.Equal(t, "child-early", tree[1].SpanName)
	assert.Equal(t, 1, tree[1].Depth)
	assert.Equal(t, "child-late", tree[2].SpanName)
	assert.Equal(t, 1, tree[2].Depth)
}

func TestBuildSpanTree_NilStartTime(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)

	spans := []obstypes.SpanInfo{
		{SpanID: "c", SpanName: "nil-time-c", ParentSpanID: "", StartTime: nil},
		{SpanID: "a", SpanName: "has-time", ParentSpanID: "", StartTime: &t1},
		{SpanID: "b", SpanName: "nil-time-b", ParentSpanID: "", StartTime: nil},
	}

	// Run multiple times to verify determinism.
	for range 5 {
		tree := buildSpanTree(spans)
		require.Len(t, tree, 3)
		// Non-nil sorts first.
		assert.Equal(t, "has-time", tree[0].SpanName)
		// Nil spans sorted by ID: "b" < "c".
		assert.Equal(t, "nil-time-b", tree[1].SpanName)
		assert.Equal(t, "nil-time-c", tree[2].SpanName)
	}
}
