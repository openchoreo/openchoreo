// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteInitializeAndWriteAlertEntry(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dsn := "file:" + filepath.Join(tempDir, "alerts.db")

	store, err := New(BackendSQLite, dsn, slog.Default())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close store: %v", closeErr)
		}
	})

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	id, err := store.WriteAlertEntry(ctx, &AlertEntry{
		Timestamp:            "2026-03-07T10:20:30Z",
		AlertRuleName:        "high-error-rate",
		AlertRuleCRName:      "payment-error-rule",
		AlertRuleCRNamespace: "openchoreo-observability-plane",
		AlertValue:           "18",
		NamespaceName:        "choreo-prod",
		ComponentName:        "payments",
		EnvironmentName:      "prod",
		ProjectName:          "commerce",
		ComponentID:          "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		EnvironmentID:        "d4e5f6a7-8901-23de-f012-4567890abcde",
		ProjectID:            "b2c3d4e5-6789-01bc-def0-234567890abc",
		IncidentEnabled:      true,
		Severity:             "critical",
		Description:          "Payment error rate exceeds threshold",
		NotificationChannels: `["email-main"]`,
		SourceType:           "log",
		SourceQuery:          "level=error",
		ConditionOperator:    "gt",
		ConditionThreshold:   10,
		ConditionWindow:      "5m0s",
		ConditionInterval:    "1m0s",
	})
	if err != nil {
		t.Fatalf("failed to write alert entry: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	if _, statErr := os.Stat(filepath.Join(tempDir, "alerts.db")); statErr != nil {
		t.Fatalf("expected sqlite db file to exist: %v", statErr)
	}
}

func TestWriteAlertEntryWithNilEntry(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close store: %v", closeErr)
		}
	})

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	if _, err := store.WriteAlertEntry(ctx, nil); err == nil {
		t.Fatal("expected error for nil alert entry")
	}
}

func TestQueryAlertEntries(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close store: %v", closeErr)
		}
	})

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	entries := []*AlertEntry{
		{
			Timestamp:            "2026-03-07T10:20:30Z",
			AlertRuleName:        "rule-a",
			AlertRuleCRName:      "rule-a",
			AlertRuleCRNamespace: "ns-1",
			AlertValue:           "11",
			NamespaceName:        "ns-1",
			ComponentName:        "comp-1",
			EnvironmentName:      "dev",
			ProjectName:          "proj-1",
			ProjectID:            "b2c3d4e5-6789-01bc-def0-234567890abc",
			ComponentID:          "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			EnvironmentID:        "d4e5f6a7-8901-23de-f012-4567890abcde",
		},
		{
			Timestamp:            "2026-03-07T10:21:30Z",
			AlertRuleName:        "rule-b",
			AlertRuleCRName:      "rule-b",
			AlertRuleCRNamespace: "ns-2",
			AlertValue:           "22",
			NamespaceName:        "ns-2",
			ComponentName:        "comp-2",
			EnvironmentName:      "prod",
			ProjectName:          "proj-2",
			ProjectID:            "e5f6a7b8-9012-34ef-0123-567890abcdef",
			ComponentID:          "f6a7b8c9-0123-45f0-1234-67890abcdef0",
			EnvironmentID:        "a7b8c9d0-1234-56f1-2345-7890abcdef01",
		},
	}
	for _, entry := range entries {
		if _, err := store.WriteAlertEntry(ctx, entry); err != nil {
			t.Fatalf("failed to write alert entry: %v", err)
		}
	}

	got, total, err := store.QueryAlertEntries(ctx, QueryParams{
		StartTime:     "2026-03-07T10:00:00Z",
		EndTime:       "2026-03-07T11:00:00Z",
		NamespaceName: "ns-2",
		EnvironmentID: "a7b8c9d0-1234-56f1-2345-7890abcdef01",
		Limit:         10,
		SortOrder:     "desc",
	})
	if err != nil {
		t.Fatalf("failed to query alert entries: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].AlertRuleName != "rule-b" {
		t.Fatalf("expected rule-b, got %s", got[0].AlertRuleName)
	}
}
