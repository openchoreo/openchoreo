// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"log/slog"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// newTestLogsService creates a LogsService with minimal dependencies for unit-testing
// the pure conversion / resolve methods (no OpenSearch connection required).
const testMyNS = "my-ns"

func newTestLogsService(t *testing.T) *LogsService {
	t.Helper()
	logger := slog.Default()
	resolver := NewResourceResolver(&config.ResolverConfig{
		// Empty URL → fetchResourceUID returns error → fallback to name-as-UID
		OpenChoreoAPIURL: "",
	}, logger)

	return &LogsService{
		logsBackend:    nil,
		defaultAdaptor: nil,
		config: &config.Config{
			Experimental: config.ExperimentalConfig{UseLogsBackend: false},
		},
		resolver: resolver,
		logger:   logger,
	}
}

// ---------------------------------------------------------------------------
// resolveSearchScope
// ---------------------------------------------------------------------------

func TestLogsService_ResolveSearchScope_NilScope(t *testing.T) {
	svc := newTestLogsService(t)
	_, err := svc.resolveSearchScope(nil)

	if err == nil {
		t.Fatal("expected error for nil scope")
	}
}

func TestLogsService_ResolveSearchScope_WorkflowScope(t *testing.T) {
	svc := newTestLogsService(t)

	searchScope := &types.SearchScope{
		Workflow: &types.WorkflowSearchScope{
			Namespace:       testMyNS,
			WorkflowRunName: "run-abc123",
		},
	}

	scope, err := svc.resolveSearchScope(searchScope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !scope.IsWorkflowScope {
		t.Error("expected IsWorkflowScope=true")
	}
	if scope.NamespaceName != testMyNS {
		t.Errorf("NamespaceName = %q, want %q", scope.NamespaceName, testMyNS)
	}
	if scope.WorkflowRunName != "run-abc123" {
		t.Errorf("WorkflowRunName = %q, want %q", scope.WorkflowRunName, "run-abc123")
	}
}

func TestLogsService_ResolveSearchScope_ComponentScope(t *testing.T) {
	svc := newTestLogsService(t)

	searchScope := &types.SearchScope{
		Component: &types.ComponentSearchScope{
			Namespace:   testMyNS,
			Project:     "my-project",
			Component:   "my-component",
			Environment: "dev",
		},
	}

	scope, err := svc.resolveSearchScope(searchScope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scope.IsWorkflowScope {
		t.Error("expected IsWorkflowScope=false for component scope")
	}
	if scope.NamespaceName != testMyNS {
		t.Errorf("NamespaceName = %q, want %q", scope.NamespaceName, testMyNS)
	}
	// Resolver is not configured → falls back to name as UID
	if scope.ProjectUID != "my-project" {
		t.Errorf("ProjectUID = %q, want %q (fallback)", scope.ProjectUID, "my-project")
	}
	if scope.ComponentUID != "my-component" {
		t.Errorf("ComponentUID = %q, want %q (fallback)", scope.ComponentUID, "my-component")
	}
	if scope.EnvironmentUID != "dev" {
		t.Errorf("EnvironmentUID = %q, want %q (fallback)", scope.EnvironmentUID, "dev")
	}
}

func TestLogsService_ResolveSearchScope_EmptyComponentScope(t *testing.T) {
	svc := newTestLogsService(t)

	searchScope := &types.SearchScope{
		Component: &types.ComponentSearchScope{
			// All fields empty
		},
	}

	scope, err := svc.resolveSearchScope(searchScope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scope.IsWorkflowScope {
		t.Error("expected IsWorkflowScope=false")
	}
	if scope.ProjectUID != "" {
		t.Errorf("ProjectUID = %q, want empty", scope.ProjectUID)
	}
	if scope.ComponentUID != "" {
		t.Errorf("ComponentUID = %q, want empty", scope.ComponentUID)
	}
}

// ---------------------------------------------------------------------------
// convertComponentLogsToResponse
// ---------------------------------------------------------------------------

func TestLogsService_ConvertComponentLogsToResponse_EmptyResult(t *testing.T) {
	svc := newTestLogsService(t)

	result := &observability.ComponentApplicationLogsResult{
		Logs:       []observability.LogEntry{},
		TotalCount: 0,
		Took:       5,
	}

	resp := svc.convertComponentLogsToResponse(result)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Logs) != 0 {
		t.Errorf("len(Logs) = %d, want 0", len(resp.Logs))
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
	if resp.TookMs != 5 {
		t.Errorf("TookMs = %d, want 5", resp.TookMs)
	}
}

func TestLogsService_ConvertComponentLogsToResponse_WithLogs(t *testing.T) {
	svc := newTestLogsService(t)

	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	result := &observability.ComponentApplicationLogsResult{
		Logs: []observability.LogEntry{
			{
				Timestamp:       ts,
				Log:             "hello world",
				LogLevel:        "INFO",
				ComponentName:   "comp-a",
				ProjectName:     "proj-a",
				EnvironmentName: "dev",
				NamespaceName:   "ns-a",
				ComponentID:     "uid-comp",
				ProjectID:       "uid-proj",
				EnvironmentID:   "uid-env",
				ContainerName:   "app",
				PodName:         "pod-1",
				PodNamespace:    "ns-a",
			},
		},
		TotalCount: 1,
		Took:       12,
	}

	resp := svc.convertComponentLogsToResponse(result)

	if len(resp.Logs) != 1 {
		t.Fatalf("len(Logs) = %d, want 1", len(resp.Logs))
	}

	entry := resp.Logs[0]
	wantTimestamp := ts.Format(time.RFC3339)
	if entry.Timestamp != wantTimestamp {
		t.Errorf("Timestamp = %q, want %q", entry.Timestamp, wantTimestamp)
	}
	if entry.Log != "hello world" {
		t.Errorf("Log = %q, want %q", entry.Log, "hello world")
	}
	if entry.Level != "INFO" {
		t.Errorf("Level = %q, want %q", entry.Level, "INFO")
	}
	if entry.Metadata == nil {
		t.Fatal("expected non-nil Metadata")
	}
	if entry.Metadata.ComponentName != "comp-a" {
		t.Errorf("Metadata.ComponentName = %q, want %q", entry.Metadata.ComponentName, "comp-a")
	}
	if entry.Metadata.ComponentUID != "uid-comp" {
		t.Errorf("Metadata.ComponentUID = %q, want %q", entry.Metadata.ComponentUID, "uid-comp")
	}
	if entry.Metadata.ProjectUID != "uid-proj" {
		t.Errorf("Metadata.ProjectUID = %q, want %q", entry.Metadata.ProjectUID, "uid-proj")
	}
	if entry.Metadata.EnvironmentUID != "uid-env" {
		t.Errorf("Metadata.EnvironmentUID = %q, want %q", entry.Metadata.EnvironmentUID, "uid-env")
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if resp.TookMs != 12 {
		t.Errorf("TookMs = %d, want 12", resp.TookMs)
	}
}

// ---------------------------------------------------------------------------
// convertWorkflowLogsToResponse
// ---------------------------------------------------------------------------

func TestLogsService_ConvertWorkflowLogsToResponse_EmptyResult(t *testing.T) {
	svc := newTestLogsService(t)

	result := &observability.WorkflowLogsResult{
		Logs:       []observability.WorkflowLogEntry{},
		TotalCount: 0,
		Took:       3,
	}
	scope := &internalSearchScope{
		NamespaceName:   "ns",
		WorkflowRunName: "run-1",
		IsWorkflowScope: true,
	}

	resp := svc.convertWorkflowLogsToResponse(result, scope)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Logs) != 0 {
		t.Errorf("len(Logs) = %d, want 0", len(resp.Logs))
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
	if resp.TookMs != 3 {
		t.Errorf("TookMs = %d, want 3", resp.TookMs)
	}
}

func TestLogsService_ConvertWorkflowLogsToResponse_WithLogs(t *testing.T) {
	svc := newTestLogsService(t)

	ts := time.Date(2024, 6, 15, 8, 30, 0, 0, time.UTC)
	result := &observability.WorkflowLogsResult{
		Logs: []observability.WorkflowLogEntry{
			{
				Timestamp:    ts,
				Log:          "workflow step started",
				LogLevel:     "DEBUG",
				PodName:      "pod-xyz",
				PodNamespace: "wf-ns",
			},
		},
		TotalCount: 42,
		Took:       7,
	}
	scope := &internalSearchScope{
		NamespaceName:   "wf-ns",
		WorkflowRunName: "run-xyz",
		IsWorkflowScope: true,
	}

	resp := svc.convertWorkflowLogsToResponse(result, scope)

	if len(resp.Logs) != 1 {
		t.Fatalf("len(Logs) = %d, want 1", len(resp.Logs))
	}

	entry := resp.Logs[0]
	wantTimestamp := ts.Format(time.RFC3339)
	if entry.Timestamp != wantTimestamp {
		t.Errorf("Timestamp = %q, want %q", entry.Timestamp, wantTimestamp)
	}
	if entry.Log != "workflow step started" {
		t.Errorf("Log = %q, want %q", entry.Log, "workflow step started")
	}
	if entry.Level != "DEBUG" {
		t.Errorf("Level = %q, want %q", entry.Level, "DEBUG")
	}
	if entry.Metadata == nil {
		t.Fatal("expected non-nil Metadata")
	}
	if entry.Metadata.NamespaceName != "wf-ns" {
		t.Errorf("Metadata.NamespaceName = %q, want %q", entry.Metadata.NamespaceName, "wf-ns")
	}
	if entry.Metadata.PodName != "pod-xyz" {
		t.Errorf("Metadata.PodName = %q, want %q", entry.Metadata.PodName, "pod-xyz")
	}
	if resp.Total != 42 {
		t.Errorf("Total = %d, want 42", resp.Total)
	}
}

// ---------------------------------------------------------------------------
// NewLogsService — constructor error path
// ---------------------------------------------------------------------------

func TestNewLogsService_FailsWithBadOpenSearchConfig(t *testing.T) {
	logger := slog.Default()
	resolver := NewResourceResolver(&config.ResolverConfig{}, logger)

	// Provide a config where the OpenSearch address is unreachable.
	// opensearch.NewClient does not fail on bad address, so service creation succeeds.
	// This test verifies the constructor returns a valid service for a default-like config.
	cfg := &config.Config{
		OpenSearch: config.OpenSearchConfig{
			Address:     "http://localhost:19200", // unlikely to be running
			Username:    "admin",
			Password:    "admin",
			IndexPrefix: "test-",
		},
	}

	svc, err := NewLogsService(nil, resolver, cfg, logger)
	// OpenSearch client creation does not fail if address is unreachable
	// (it only fails on invalid config). So we expect no error here.
	if err != nil {
		t.Fatalf("NewLogsService returned unexpected error: %v", err)
	}
	if svc == nil {
		t.Error("expected non-nil LogsService")
	}
}
