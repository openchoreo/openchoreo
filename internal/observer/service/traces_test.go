// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

func newTestTracesService() *TracesService {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &TracesService{
		config: cfg,
		logger: logger,
	}
}

func TestNewTracingAdapter_ConfigValidation(t *testing.T) {
	// Test with default timeout
	cfg1 := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 0,
	}
	adapter1, err := NewTracingAdapter(cfg1)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter1 == nil || adapter1.client == nil {
		t.Fatal("Expected non-nil adapter with initialized client")
	}

	// Test with custom timeout
	cfg2 := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 60 * time.Second,
	}
	adapter2, err := NewTracingAdapter(cfg2)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter2 == nil || adapter2.client == nil {
		t.Fatal("Expected non-nil adapter with initialized client")
	}
}

func TestTracesService_ConvertToResponse(t *testing.T) {
	now := time.Now()
	result := &observability.TracesQueryResult{
		Traces: []observability.Trace{
			{
				TraceID:      "trace-1",
				SpanCount:    2,
				StartTime:    now,
				EndTime:      now.Add(1 * time.Second),
				DurationNs:   1000000000,
				RootSpanID:   "span-1",
				RootSpanName: "http.request",
				TraceName:    "http.request",
				RootSpanKind: "INTERNAL",
			},
		},
		TotalCount: 1,
		Took:       10,
	}

	service := newTestTracesService()

	resp := service.convertToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(resp.Traces))
	}

	if resp.Traces[0].TraceID != "trace-1" {
		t.Errorf("Expected traceId 'trace-1', got %s", resp.Traces[0].TraceID)
	}

	if resp.Total != 1 {
		t.Errorf("Expected total 1, got %d", resp.Total)
	}

	if resp.TookMs != 10 {
		t.Errorf("Expected tookMs 10, got %d", resp.TookMs)
	}
}

func TestTracesService_ConvertToResponse_MultipleTraces(t *testing.T) {
	now := time.Now()
	result := &observability.TracesQueryResult{
		Traces: []observability.Trace{
			{
				TraceID:      "trace-1",
				SpanCount:    2,
				StartTime:    now,
				EndTime:      now.Add(100 * time.Millisecond),
				DurationNs:   100000000,
				RootSpanID:   "span-1",
				RootSpanName: "http.request",
				TraceName:    "http.request",
			},
			{
				TraceID:      "trace-2",
				SpanCount:    3,
				StartTime:    now,
				EndTime:      now.Add(200 * time.Millisecond),
				DurationNs:   200000000,
				RootSpanID:   "span-2",
				RootSpanName: "grpc.request",
				TraceName:    "grpc.request",
			},
		},
		TotalCount: 2,
		Took:       15,
	}

	service := newTestTracesService()

	resp := service.convertToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Traces) != 2 {
		t.Errorf("Expected 2 traces, got %d", len(resp.Traces))
	}

	if resp.Total != 2 {
		t.Errorf("Expected total 2, got %d", resp.Total)
	}
}

func TestTracesService_ConvertToResponse_EmptyTraces(t *testing.T) {
	result := &observability.TracesQueryResult{
		Traces:     []observability.Trace{},
		TotalCount: 0,
		Took:       5,
	}

	service := newTestTracesService()

	resp := service.convertToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Traces) != 0 {
		t.Errorf("Expected 0 traces, got %d", len(resp.Traces))
	}

	if resp.Total != 0 {
		t.Errorf("Expected total 0, got %d", resp.Total)
	}
}

func TestTracesService_ConvertSpansToResponse(t *testing.T) {
	now := time.Now()
	result := &observability.TracesQueryResult{
		Traces: []observability.Trace{
			{
				TraceID:   "trace-1",
				SpanCount: 2,
				Spans: []observability.TraceSpan{
					{
						SpanID:    "span-1",
						Name:      "http.request",
						StartTime: now,
						EndTime:   now.Add(100 * time.Millisecond),
					},
					{
						SpanID:       "span-2",
						Name:         "db.query",
						ParentSpanID: "span-1",
						StartTime:    now.Add(20 * time.Millisecond),
						EndTime:      now.Add(80 * time.Millisecond),
					},
				},
			},
		},
		TotalCount: 2,
		Took:       5,
	}

	service := newTestTracesService()

	resp := service.convertSpansToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(resp.Spans))
	}

	if resp.Total != 2 {
		t.Errorf("Expected total 2, got %d", resp.Total)
	}

	if resp.Spans[0].SpanID != "span-1" {
		t.Errorf("Expected first span ID 'span-1', got %s", resp.Spans[0].SpanID)
	}

	if resp.Spans[1].SpanID != "span-2" {
		t.Errorf("Expected second span ID 'span-2', got %s", resp.Spans[1].SpanID)
	}
}

func TestTracesService_ConvertSpansToResponse_MultipleTraces(t *testing.T) {
	result := &observability.TracesQueryResult{
		Traces: []observability.Trace{
			{
				TraceID: "trace-1",
				Spans: []observability.TraceSpan{
					{
						SpanID: "span-1",
						Name:   "http.request",
					},
					{
						SpanID: "span-2",
						Name:   "db.query",
					},
				},
			},
			{
				TraceID: "trace-2",
				Spans: []observability.TraceSpan{
					{
						SpanID: "span-3",
						Name:   "grpc.request",
					},
				},
			},
		},
		TotalCount: 3,
		Took:       10,
	}

	service := newTestTracesService()

	resp := service.convertSpansToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Spans) != 3 {
		t.Errorf("Expected 3 spans total, got %d", len(resp.Spans))
	}

	if resp.Total != 3 {
		t.Errorf("Expected total 3, got %d", resp.Total)
	}
}

func TestTracesService_ConvertSpansToResponse_EmptyTraces(t *testing.T) {
	result := &observability.TracesQueryResult{
		Traces:     []observability.Trace{},
		TotalCount: 0,
		Took:       5,
	}

	service := newTestTracesService()

	resp := service.convertSpansToResponse(result)
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Spans) != 0 {
		t.Errorf("Expected 0 spans, got %d", len(resp.Spans))
	}

	if resp.Total != 0 {
		t.Errorf("Expected total 0, got %d", resp.Total)
	}
}

func TestTraceInfo_DataIntegrity(t *testing.T) {
	now := time.Now()
	result := &observability.TracesQueryResult{
		Traces: []observability.Trace{
			{
				TraceID:      "trace-123",
				SpanCount:    5,
				StartTime:    now,
				EndTime:      now.Add(1 * time.Second),
				DurationNs:   1000000000,
				RootSpanID:   "root-span",
				RootSpanName: "root-operation",
				RootSpanKind: "INTERNAL",
				TraceName:    "root-operation",
			},
		},
		TotalCount: 1,
		Took:       20,
	}

	service := newTestTracesService()

	resp := service.convertToResponse(result)
	trace := resp.Traces[0]

	if trace.TraceID != "trace-123" {
		t.Errorf("Expected traceId 'trace-123', got %s", trace.TraceID)
	}
	if trace.SpanCount != 5 {
		t.Errorf("Expected spanCount 5, got %d", trace.SpanCount)
	}
	if trace.RootSpanID != "root-span" {
		t.Errorf("Expected rootSpanId 'root-span', got %s", trace.RootSpanID)
	}
	if trace.RootSpanName != "root-operation" {
		t.Errorf("Expected rootSpanName 'root-operation', got %s", trace.RootSpanName)
	}
	if trace.RootSpanKind != "INTERNAL" {
		t.Errorf("Expected rootSpanKind 'INTERNAL', got %s", trace.RootSpanKind)
	}
	if trace.TraceName != "root-operation" {
		t.Errorf("Expected traceName 'root-operation', got %s", trace.TraceName)
	}
	if trace.DurationNs != 1000000000 {
		t.Errorf("Expected durationNs 1000000000, got %d", trace.DurationNs)
	}
}
