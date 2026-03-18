// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestConvertTracesResponseToGen(t *testing.T) {
	now := time.Now()
	resp := &types.TracesQueryResponse{
		Traces: []types.TraceInfo{
			{
				TraceID:      "trace-1",
				TraceName:    "test-trace",
				SpanCount:    3,
				RootSpanID:   "span-1",
				RootSpanName: "root",
				RootSpanKind: "INTERNAL",
				StartTime:    &now,
				EndTime:      &now,
				DurationNs:   1000000,
			},
		},
		Total:  1,
		TookMs: 10,
	}

	genResp := convertTracesResponseToGen(resp)
	require.NotNil(t, genResp)
}

func TestConvertTracesResponseToGen_NilResponse(t *testing.T) {
	genResp := convertTracesResponseToGen(nil)
	assert.Nil(t, genResp)
}

func TestConvertTracesResponseToGen_EmptyTraces(t *testing.T) {
	resp := &types.TracesQueryResponse{
		Traces: []types.TraceInfo{},
		Total:  0,
		TookMs: 5,
	}

	genResp := convertTracesResponseToGen(resp)
	require.NotNil(t, genResp)
}

func TestConvertTracesResponseToGen_MultipleTraces(t *testing.T) {
	now := time.Now()
	resp := &types.TracesQueryResponse{
		Traces: []types.TraceInfo{
			{
				TraceID:      "trace-1",
				TraceName:    "http.request",
				SpanCount:    2,
				RootSpanID:   "span-1",
				RootSpanName: "http.request",
				StartTime:    &now,
				EndTime:      &now,
			},
			{
				TraceID:      "trace-2",
				TraceName:    "grpc.request",
				SpanCount:    3,
				RootSpanID:   "span-2",
				RootSpanName: "grpc.request",
				StartTime:    &now,
				EndTime:      &now,
			},
		},
		Total:  2,
		TookMs: 15,
	}

	genResp := convertTracesResponseToGen(resp)
	require.NotNil(t, genResp)
}

func TestConvertSpansResponseToGen(t *testing.T) {
	now := time.Now()
	resp := &types.SpansQueryResponse{
		Spans: []types.SpanInfo{
			{
				SpanID:    "span-1",
				SpanName:  "http.request",
				StartTime: &now,
				EndTime:   &now,
			},
		},
		Total:  1,
		TookMs: 5,
	}

	genResp := convertSpansResponseToGen(resp)
	require.NotNil(t, genResp)
}

func TestConvertSpansResponseToGen_NilResponse(t *testing.T) {
	genResp := convertSpansResponseToGen(nil)
	assert.Nil(t, genResp)
}

func TestConvertSpansResponseToGen_MultipleSpans(t *testing.T) {
	now := time.Now()
	end1 := now.Add(100 * time.Millisecond)
	start2 := now.Add(20 * time.Millisecond)
	end2 := now.Add(80 * time.Millisecond)

	resp := &types.SpansQueryResponse{
		Spans: []types.SpanInfo{
			{
				SpanID:       "span-1",
				SpanName:     "http.request",
				ParentSpanID: "",
				StartTime:    &now,
				EndTime:      &end1,
				DurationNs:   100000000,
			},
			{
				SpanID:       "span-2",
				SpanName:     "db.query",
				ParentSpanID: "span-1",
				StartTime:    &start2,
				EndTime:      &end2,
				DurationNs:   60000000,
			},
		},
		Total:  2,
		TookMs: 5,
	}

	genResp := convertSpansResponseToGen(resp)
	require.NotNil(t, genResp)
}

func TestConvertSpanDetailsToGen(t *testing.T) {
	now := time.Now()
	span := &types.SpanInfo{
		SpanID:       "span-1",
		SpanName:     "http.request",
		ParentSpanID: "span-0",
		StartTime:    &now,
		EndTime:      &now,
		DurationNs:   1000000,
		Attributes: map[string]interface{}{
			"http.method": "GET",
		},
		ResourceAttributes: map[string]interface{}{
			"service.name": "test-service",
		},
	}

	spanData := convertSpanDetailsToGen(span)
	require.NotNil(t, spanData)
	assert.Equal(t, "span-1", spanData["spanId"])
}

func TestConvertSpanDetailsToGen_NilSpan(t *testing.T) {
	spanData := convertSpanDetailsToGen(nil)
	assert.Nil(t, spanData)
}

func TestConvertSpanDetailsToGen_NoParent(t *testing.T) {
	now := time.Now()
	span := &types.SpanInfo{
		SpanID:       "span-1",
		SpanName:     "http.request",
		ParentSpanID: "",
		StartTime:    &now,
		EndTime:      &now,
		DurationNs:   1000000,
	}

	spanData := convertSpanDetailsToGen(span)
	require.NotNil(t, spanData)
	// Parent span ID should not be in the response at all
	_, ok := spanData["parentSpanId"]
	assert.False(t, ok, "Expected parentSpanId to be absent from response")
}

func TestConvertSpanDetailsToGen_WithAttributes(t *testing.T) {
	now := time.Now()
	attrs := map[string]interface{}{
		"http.method":      "POST",
		"http.url":         "http://example.com/api",
		"http.status_code": 200,
	}
	resourceAttrs := map[string]interface{}{
		"service.name":    "my-service",
		"service.version": "1.0.0",
	}

	span := &types.SpanInfo{
		SpanID:             "span-1",
		SpanName:           "http.request",
		StartTime:          &now,
		EndTime:            &now,
		DurationNs:         1000000,
		Attributes:         attrs,
		ResourceAttributes: resourceAttrs,
	}

	spanData := convertSpanDetailsToGen(span)
	require.NotNil(t, spanData)
	assert.NotNil(t, spanData["attributes"])
	assert.NotNil(t, spanData["resourceAttributes"])
}

func TestDerefInt(t *testing.T) {
	val := 100
	result := derefInt(&val, 50)
	assert.Equal(t, 100, result)

	result = derefInt(nil, 50)
	assert.Equal(t, 50, result)
}

func TestDerefString(t *testing.T) {
	val := "test"
	result := derefString(&val)
	assert.Equal(t, "test", result)

	result = derefString(nil)
	assert.Equal(t, "", result)
}
