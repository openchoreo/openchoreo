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
				t.Fatalf("decode request: %v", err)
			}
			response := map[string]any{
				"events": []map[string]any{
					{
						"timestamp": time.UnixMilli(1000).UTC().Format(time.RFC3339Nano),
						"reason":    aggregator.ReasonDeploymentSucceeded,
						"message":   `{"renderedReleaseUid":"u1"}`,
						"metadata": map[string]any{
							"namespaceName":   "acme",
							"projectName":     "shop",
							"componentName":   "checkout",
							"environmentName": "dev",
						},
					},
				},
				"total": 1,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewLogsAdapter: %v", err)
		}

		events, err := adapter.FetchDeliveryEvents(ctx, 0, 2000)
		if err != nil {
			t.Fatalf("FetchDeliveryEvents: %v", err)
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
		if e.Message != `{"renderedReleaseUid":"u1"}` {
			t.Errorf("message = %q", e.Message)
		}
	})

	t.Run("follows nextCursor until exhausted", func(t *testing.T) {
		var cursors []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			cursor, _ := req["searchAfter"].(string)
			cursors = append(cursors, cursor)

			response := map[string]any{
				"events": []map[string]any{
					{
						"timestamp": time.UnixMilli(int64(len(cursors)) * 1000).UTC().Format(time.RFC3339Nano),
						"reason":    aggregator.ReasonDeploymentStarted,
						"message":   "{}",
					},
				},
			}
			if len(cursors) < 3 {
				response["nextCursor"] = "cursor-" + cursor + "x"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewLogsAdapter: %v", err)
		}

		events, err := adapter.FetchDeliveryEvents(ctx, 0, 10000)
		if err != nil {
			t.Fatalf("FetchDeliveryEvents: %v", err)
		}
		if len(events) != 3 {
			t.Errorf("expected 3 events across pages, got %d", len(events))
		}
		if len(cursors) != 3 || cursors[0] != "" || cursors[1] == "" || cursors[2] == "" {
			t.Errorf("cursor sequence wrong: %v", cursors)
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
		if _, err := adapter.FetchDeliveryEvents(ctx, 0, 1000); err == nil {
			t.Fatal("expected error from failing adapter")
		}
	})
}
