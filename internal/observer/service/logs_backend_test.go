// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

func newTestLogsBackend(baseURL string) *LogsBackend {
	return NewLogsBackend(LogsBackendConfig{
		BaseURL: baseURL,
		Timeout: 5 * time.Second,
	})
}

// ---------------------------------------------------------------------------
// NewLogsBackend
// ---------------------------------------------------------------------------

func TestNewLogsBackend_DefaultTimeout(t *testing.T) {
	b := NewLogsBackend(LogsBackendConfig{BaseURL: "http://example.com"})
	if b == nil {
		t.Fatal("expected non-nil LogsBackend")
	}
	if b.httpClient.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", b.httpClient.Timeout)
	}
}

func TestNewLogsBackend_CustomTimeout(t *testing.T) {
	b := NewLogsBackend(LogsBackendConfig{
		BaseURL: "http://example.com",
		Timeout: 10 * time.Second,
	})
	if b.httpClient.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", b.httpClient.Timeout)
	}
}

// ---------------------------------------------------------------------------
// GetComponentApplicationLogs
// ---------------------------------------------------------------------------

func TestLogsBackend_GetComponentApplicationLogs_Success(t *testing.T) {
	wantResult := observability.ComponentApplicationLogsResult{
		Logs: []observability.LogEntry{
			{Log: "test log", LogLevel: "INFO"},
		},
		TotalCount: 1,
		Took:       5,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/component-application-logs" {
			t.Errorf("path = %q, want /api/v1/component-application-logs", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wantResult)
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	params := observability.ComponentApplicationLogsParams{
		ComponentID: "comp-1",
		Namespace:   "ns",
		StartTime:   time.Now().Add(-1 * time.Hour),
		EndTime:     time.Now(),
	}

	result, err := backend.GetComponentApplicationLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", result.TotalCount)
	}
	if len(result.Logs) != 1 {
		t.Fatalf("len(Logs) = %d, want 1", len(result.Logs))
	}
	if result.Logs[0].Log != "test log" {
		t.Errorf("Log = %q, want %q", result.Logs[0].Log, "test log")
	}
}

func TestLogsBackend_GetComponentApplicationLogs_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	_, err := backend.GetComponentApplicationLogs(context.Background(), observability.ComponentApplicationLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for server error response, got nil")
	}
}

func TestLogsBackend_GetComponentApplicationLogs_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	_, err := backend.GetComponentApplicationLogs(context.Background(), observability.ComponentApplicationLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for invalid JSON response, got nil")
	}
}

func TestLogsBackend_GetComponentApplicationLogs_ConnectionRefused(t *testing.T) {
	// Grab an ephemeral port that nothing is listening on
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // close immediately so the port is not listening

	backend := newTestLogsBackend("http://" + addr)

	_, err = backend.GetComponentApplicationLogs(context.Background(), observability.ComponentApplicationLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for connection refused, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetWorkflowLogs
// ---------------------------------------------------------------------------

func TestLogsBackend_GetWorkflowLogs_Success(t *testing.T) {
	wantResult := observability.WorkflowLogsResult{
		Logs: []observability.WorkflowLogEntry{
			{Log: "workflow log", LogLevel: "DEBUG"},
		},
		TotalCount: 1,
		Took:       3,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/workflow-logs" {
			t.Errorf("path = %q, want /api/v1/workflow-logs", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wantResult)
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	params := observability.WorkflowLogsParams{
		Namespace:       "ns",
		WorkflowRunName: "run-abc",
		StartTime:       time.Now().Add(-1 * time.Hour),
		EndTime:         time.Now(),
	}

	result, err := backend.GetWorkflowLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", result.TotalCount)
	}
	if len(result.Logs) != 1 {
		t.Fatalf("len(Logs) = %d, want 1", len(result.Logs))
	}
	if result.Logs[0].Log != "workflow log" {
		t.Errorf("Log = %q, want %q", result.Logs[0].Log, "workflow log")
	}
}

func TestLogsBackend_GetWorkflowLogs_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	_, err := backend.GetWorkflowLogs(context.Background(), observability.WorkflowLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for server error response, got nil")
	}
}

func TestLogsBackend_GetWorkflowLogs_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{bad json"))
	}))
	defer srv.Close()

	backend := newTestLogsBackend(srv.URL)

	_, err := backend.GetWorkflowLogs(context.Background(), observability.WorkflowLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for invalid JSON response, got nil")
	}
}

func TestLogsBackend_GetWorkflowLogs_ConnectionRefused(t *testing.T) {
	// Grab an ephemeral port that nothing is listening on
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // close immediately so the port is not listening

	backend := newTestLogsBackend("http://" + addr)

	_, err = backend.GetWorkflowLogs(context.Background(), observability.WorkflowLogsParams{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})
	if err == nil {
		t.Error("expected error for connection refused, got nil")
	}
}
