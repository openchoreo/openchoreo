// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// opensearchSearchResponse is a minimal OpenSearch search response used in tests.
type opensearchSearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []map[string]interface{} `json:"hits"`
	} `json:"hits"`
}

// newMockOpenSearchServer starts an httptest.Server that responds to:
//   - GET / (Info call during client init)
//   - POST /_search or POST /*/_search (search calls)
func newMockOpenSearchServer(t *testing.T, searchResp opensearchSearchResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// OpenSearch client calls GET / on init for Info()
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "mock-node",
				"version": map[string]interface{}{
					"number":       "1.3.0",
					"distribution": "opensearch",
				},
				"tagline": "The OpenSearch Project: https://opensearch.org/",
			})
			return
		}

		// Search requests
		_ = json.NewEncoder(w).Encode(searchResp)
	}))
}

func newTestAdaptor(t *testing.T, serverURL string) *DefaultLogsAdaptor {
	t.Helper()
	cfg := &config.OpenSearchConfig{
		Address:      serverURL,
		Username:     "admin",
		Password:     "admin",
		IndexPrefix:  "container-logs-",
		IndexPattern: "container-logs-*",
	}
	a, err := NewDefaultLogsAdaptor(cfg, slog.Default())
	if err != nil {
		t.Fatalf("failed to create DefaultLogsAdaptor: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// GetComponentApplicationLogs
// ---------------------------------------------------------------------------

func TestDefaultLogsAdaptor_GetComponentApplicationLogs_EmptyResult(t *testing.T) {
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 0
	resp.Hits.Hits = []map[string]interface{}{}
	resp.Took = 5

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	params := observability.ComponentApplicationLogsParams{
		Namespace: "ns",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:     100,
		SortOrder: "desc",
	}

	result, err := a.GetComponentApplicationLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Logs) != 0 {
		t.Errorf("len(Logs) = %d, want 0", len(result.Logs))
	}
	if result.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", result.TotalCount)
	}
	if result.Took != 5 {
		t.Errorf("Took = %d, want 5", result.Took)
	}
}

func TestDefaultLogsAdaptor_GetComponentApplicationLogs_WithLogs(t *testing.T) {
	hit := map[string]interface{}{
		"_id":    "hit-1",
		"_score": nil,
		"_source": map[string]interface{}{
			"log":        "hello from component",
			"@timestamp": "2024-01-01T00:00:00Z",
		},
	}
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 1
	resp.Hits.Hits = []map[string]interface{}{hit}
	resp.Took = 10

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	params := observability.ComponentApplicationLogsParams{
		Namespace: "ns",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:     100,
		SortOrder: "desc",
	}

	result, err := a.GetComponentApplicationLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", result.TotalCount)
	}
	if result.Took != 10 {
		t.Errorf("Took = %d, want 10", result.Took)
	}
}

func TestDefaultLogsAdaptor_GetComponentApplicationLogs_WithFilters(t *testing.T) {
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 0
	resp.Hits.Hits = []map[string]interface{}{}

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	params := observability.ComponentApplicationLogsParams{
		Namespace:     "ns",
		ProjectID:     "proj-uid",
		ComponentID:   "comp-uid",
		EnvironmentID: "env-uid",
		SearchPhrase:  "error",
		LogLevels:     []string{"ERROR", "WARN"},
		StartTime:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:         50,
		SortOrder:     "asc",
	}

	result, err := a.GetComponentApplicationLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------------------------------------------------------------------------
// GetWorkflowLogs
// ---------------------------------------------------------------------------

func TestDefaultLogsAdaptor_GetWorkflowLogs_EmptyResult(t *testing.T) {
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 0
	resp.Hits.Hits = []map[string]interface{}{}
	resp.Took = 3

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	params := observability.WorkflowLogsParams{
		Namespace:       "ns",
		WorkflowRunName: "run-abc",
		StartTime:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:           100,
		SortOrder:       "desc",
	}

	result, err := a.GetWorkflowLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Logs) != 0 {
		t.Errorf("len(Logs) = %d, want 0", len(result.Logs))
	}
	if result.Took != 3 {
		t.Errorf("Took = %d, want 3", result.Took)
	}
}

func TestDefaultLogsAdaptor_GetWorkflowLogs_DefaultLimitAndSortOrder(t *testing.T) {
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 0
	resp.Hits.Hits = []map[string]interface{}{}

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	// Zero limit and empty sort order — adaptor should apply defaults
	params := observability.WorkflowLogsParams{
		Namespace: "ns",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:     0,
		SortOrder: "",
	}

	result, err := a.GetWorkflowLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestDefaultLogsAdaptor_GetWorkflowLogs_WithSearchPhrase(t *testing.T) {
	resp := opensearchSearchResponse{}
	resp.Hits.Total.Value = 2
	resp.Hits.Hits = []map[string]interface{}{
		{
			"_id":    "wf-hit-1",
			"_score": 1.0,
			"_source": map[string]interface{}{
				"log":        "step 1 started",
				"@timestamp": "2024-01-01T00:00:00Z",
			},
		},
		{
			"_id":    "wf-hit-2",
			"_score": 0.9,
			"_source": map[string]interface{}{
				"log":        "step 2 started",
				"@timestamp": "2024-01-01T00:01:00Z",
			},
		},
	}
	resp.Took = 8

	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	a := newTestAdaptor(t, srv.URL)

	params := observability.WorkflowLogsParams{
		Namespace:       "ns",
		WorkflowRunName: "run-xyz",
		SearchPhrase:    "step",
		StartTime:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:           10,
		SortOrder:       "asc",
	}

	result, err := a.GetWorkflowLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", result.TotalCount)
	}
	if len(result.Logs) != 2 {
		t.Errorf("len(Logs) = %d, want 2", len(result.Logs))
	}
}

// ---------------------------------------------------------------------------
// NewDefaultLogsAdaptor — constructor
// ---------------------------------------------------------------------------

func TestNewDefaultLogsAdaptor_Success(t *testing.T) {
	resp := opensearchSearchResponse{}
	srv := newMockOpenSearchServer(t, resp)
	defer srv.Close()

	cfg := &config.OpenSearchConfig{
		Address:     srv.URL,
		Username:    "admin",
		Password:    "admin",
		IndexPrefix: "logs-",
	}

	a, err := NewDefaultLogsAdaptor(cfg, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Error("expected non-nil DefaultLogsAdaptor")
	}
}
