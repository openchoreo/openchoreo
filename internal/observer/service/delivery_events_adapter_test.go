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

	"github.com/openchoreo/openchoreo/internal/observer/aggregator"
)

func TestFetchDeliveryEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("sends unscoped reason-filtered query and maps events", func(t *testing.T) {
		var gotRequest map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/events/query" {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
				// t.Fatalf would Goexit this handler goroutine, leaving the client with
				// an empty body and surfacing as a confusing decode error instead.
				t.Errorf("decode request: %v", err)
				return
			}
			response := map[string]any{
				"events": []map[string]any{
					{
						"timestamp": time.UnixMilli(1000).UTC().Format(time.RFC3339Nano),
						"reason":    aggregator.ReasonDeploymentSucceeded,
						"message":   `{"rolloutId":"u1"}`,
						"metadata": map[string]any{
							"namespaceName":   "acme",
							"projectName":     "shop",
							"componentName":   "checkout",
							"environmentName": "dev",
						},
					},
				},
				"total":    1,
				"complete": true,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewLogsAdapter: %v", err)
		}

		events, complete, err := adapter.FetchDeliveryEvents(ctx, 0, 2000)
		if err != nil {
			t.Fatalf("FetchDeliveryEvents: %v", err)
		}
		if !complete {
			t.Error("a single-page sweep must report itself complete")
		}

		if _, hasScope := gotRequest["searchScope"]; hasScope {
			t.Error("expected request without searchScope for the org-wide sweep")
		}
		reasons, ok := gotRequest["reasons"].([]any)
		if !ok || len(reasons) != 4 {
			t.Errorf("expected 4 reasons in request, got %v", gotRequest["reasons"])
		}
		if gotRequest["sortOrder"] != "asc" {
			t.Errorf("sortOrder = %v, want asc (aggregator folds chronologically)", gotRequest["sortOrder"])
		}

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		e := events[0]
		if e.Reason != aggregator.ReasonDeploymentSucceeded {
			t.Errorf("reason = %q", e.Reason)
		}
		if e.TimestampMs != 1000 {
			t.Errorf("timestampMs = %d, want 1000", e.TimestampMs)
		}
		if e.Namespace != "acme" || e.ProjectName != "shop" || e.ComponentName != "checkout" || e.EnvironmentName != "dev" {
			t.Errorf("metadata mapping wrong: %+v", e)
		}
		if e.Message != `{"rolloutId":"u1"}` {
			t.Errorf("message = %q", e.Message)
		}
	})

	// Completeness is what the aggregator resumes on, and getting it wrong in the
	// permissive direction advances the watermark past events nobody read. It is
	// derived from `total` against the events returned, so every way `total` can
	// be wrong has to fail towards "not complete".
	t.Run("completeness is derived from total, erring towards not complete", func(t *testing.T) {
		for name, tc := range map[string]struct {
			total        any
			events       int
			wantComplete bool
		}{
			"total matches the page": {total: 2, events: 2, wantComplete: true},
			"total exceeds the page": {total: 9, events: 2, wantComplete: false},
			"empty window":           {total: 0, events: 0, wantComplete: true},
			// The contract requires total; an adapter that omits it decodes as zero,
			// which must not read as "nothing matched" when events came back.
			"total absent with events": {total: nil, events: 2, wantComplete: false},
			// Understating total is the failure the contract warns about -- it must
			// cost a re-read, never a skip.
			"total understated": {total: 1, events: 2, wantComplete: false},
		} {
			t.Run(name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					events := make([]map[string]any, 0, tc.events)
					for i := 0; i < tc.events; i++ {
						events = append(events, map[string]any{
							"timestamp": time.UnixMilli(int64(1000 + i)).UTC().Format(time.RFC3339Nano),
							"reason":    aggregator.ReasonDeploymentSucceeded,
							"message":   `{"rolloutId":"u1"}`,
						})
					}
					response := map[string]any{"events": events}
					if tc.total != nil {
						response["total"] = tc.total
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				defer server.Close()

				adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
				if err != nil {
					t.Fatalf("NewLogsAdapter: %v", err)
				}

				events, complete, err := adapter.FetchDeliveryEvents(ctx, 0, 10000)
				if err != nil {
					t.Fatalf("FetchDeliveryEvents: %v", err)
				}
				if len(events) != tc.events {
					t.Fatalf("expected %d events, got %d", tc.events, len(events))
				}
				if complete != tc.wantComplete {
					t.Errorf("complete = %v, want %v", complete, tc.wantComplete)
				}
			})
		}
	})

	// One request per sweep now: resumption is the aggregator's job, on timestamps.
	t.Run("issues a single request per sweep", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{}, "total": 0, "complete": true,
			})
		}))
		defer server.Close()

		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewLogsAdapter: %v", err)
		}
		if _, _, err := adapter.FetchDeliveryEvents(ctx, 0, 10000); err != nil {
			t.Fatalf("FetchDeliveryEvents: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected exactly 1 adapter call, got %d", calls)
		}
	})

	t.Run("adapter error surfaces", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewLogsAdapter: %v", err)
		}
		if _, _, err := adapter.FetchDeliveryEvents(ctx, 0, 1000); err == nil {
			t.Fatal("expected error from failing adapter")
		}
	})
}
