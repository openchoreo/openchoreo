// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

func TestNewTracingAdapter_DefaultTimeout(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 0, // Should use default
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_CustomTimeout(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 60 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_BaseURLSet(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://traces-adapter.example.com:9000",
		Timeout: 30 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_ClientInitialized(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 30 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestTracingAdapter_QuerySpanDetails_ForwardsScopeAndConverts(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spanId":"span-1","spanName":"http.request"}`))
	}))
	defer srv.Close()

	adapter, err := NewTracingAdapter(TracingAdapterConfig{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	detail, err := adapter.QuerySpanDetails(context.Background(), "trace-1", "span-1",
		observability.TracesQueryParams{Namespace: "ns", ProjectID: "proj-uid", ComponentID: "comp-uid", EnvironmentID: "env-uid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1alpha1/traces/trace-1/spans/span-1" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	scope, _ := gotBody["searchScope"].(map[string]interface{})
	if scope == nil {
		t.Fatalf("expected searchScope in body, got %v", gotBody)
	}
	if scope["namespace"] != "ns" {
		t.Errorf("expected namespace=ns, got %v", scope["namespace"])
	}
	if scope["project"] != "proj-uid" {
		t.Errorf("expected project=proj-uid, got %v", scope["project"])
	}
	if scope["component"] != "comp-uid" {
		t.Errorf("expected component=comp-uid, got %v", scope["component"])
	}
	if scope["environment"] != "env-uid" {
		t.Errorf("expected environment=env-uid, got %v", scope["environment"])
	}
	if detail.SpanID != "span-1" {
		t.Errorf("expected spanId=span-1, got %s", detail.SpanID)
	}
}

// buildGenSpansResponse constructs a gen.TraceSpansQueryResponse from a raw map
// using JSON round-trip, avoiding the need to deal with anonymous struct types.
func buildGenSpansResponse(t *testing.T, raw map[string]interface{}) *gen.TraceSpansQueryResponse {
	t.Helper()
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("failed to marshal spans response: %v", err)
	}
	var resp gen.TraceSpansQueryResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("failed to unmarshal spans response: %v", err)
	}
	return &resp
}

func TestConvertSpansAdapterResponse_WithAttributes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	total := 1
	tookMs := 5

	resp := buildGenSpansResponse(t, map[string]interface{}{
		"total":  total,
		"tookMs": tookMs,
		"spans": []map[string]interface{}{
			{
				"spanId":    "span-1",
				"spanName":  "http.request",
				"startTime": now.Format(time.RFC3339Nano),
				"endTime":   now.Format(time.RFC3339Nano),
				"attributes": map[string]interface{}{
					"http.method":      "GET",
					"http.status_code": float64(200),
				},
				"resourceAttributes": map[string]interface{}{
					"service.name": "my-service",
				},
			},
		},
	})

	result := convertSpansAdapterResponse(resp)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(result.Spans))
	}
	span := result.Spans[0]
	if span.SpanID != "span-1" {
		t.Errorf("Expected spanId=span-1, got %s", span.SpanID)
	}
	if span.Attributes == nil {
		t.Fatal("Expected Attributes to be populated")
	}
	if span.Attributes["http.method"] != "GET" {
		t.Errorf("Expected http.method=GET, got %v", span.Attributes["http.method"])
	}
	if span.ResourceAttributes == nil {
		t.Fatal("Expected ResourceAttributes to be populated")
	}
	if span.ResourceAttributes["service.name"] != "my-service" {
		t.Errorf("Expected service.name=my-service, got %v", span.ResourceAttributes["service.name"])
	}
}

func TestConvertSpansAdapterResponse_NilAttributes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	resp := buildGenSpansResponse(t, map[string]interface{}{
		"total":  1,
		"tookMs": 5,
		"spans": []map[string]interface{}{
			{
				"spanId":    "span-1",
				"spanName":  "http.request",
				"startTime": now.Format(time.RFC3339Nano),
				"endTime":   now.Format(time.RFC3339Nano),
				// attributes and resourceAttributes intentionally absent
			},
		},
	})

	result := convertSpansAdapterResponse(resp)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(result.Spans))
	}
	span := result.Spans[0]
	if span.Attributes != nil {
		t.Errorf("Expected Attributes to be nil, got %v", span.Attributes)
	}
	if span.ResourceAttributes != nil {
		t.Errorf("Expected ResourceAttributes to be nil, got %v", span.ResourceAttributes)
	}
}
