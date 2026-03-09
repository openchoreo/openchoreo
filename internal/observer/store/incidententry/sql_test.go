// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteInitializeAndWriteIncidentEntry(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dsn := "file:" + filepath.Join(tempDir, "incidents.db")

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

	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "alt-1",
		Timestamp:       "2026-03-07T10:20:30Z",
		Status:          StatusTriggered,
		TriggerAiRca:    true,
		TriggeredAt:     "2026-03-07T10:20:30Z",
		Description:     "High error rate observed",
		NamespaceName:   "choreo-prod",
		ComponentName:   "payments",
		EnvironmentName: "prod",
		ProjectName:     "commerce",
		ComponentID:     "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		EnvironmentID:   "d4e5f6a7-8901-23de-f012-4567890abcde",
		ProjectID:       "b2c3d4e5-6789-01bc-def0-234567890abc",
	})
	if err != nil {
		t.Fatalf("failed to write incident entry: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	if _, statErr := os.Stat(filepath.Join(tempDir, "incidents.db")); statErr != nil {
		t.Fatalf("expected sqlite db file to exist: %v", statErr)
	}
}

func TestWriteIncidentEntryWithNilEntry(t *testing.T) {
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

	if _, err := store.WriteIncidentEntry(ctx, nil); err == nil {
		t.Fatal("expected error for nil incident entry")
	}
}

func TestWriteIncidentEntryWithMissingAlertID(t *testing.T) {
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

	if _, err := store.WriteIncidentEntry(ctx, &IncidentEntry{}); err == nil {
		t.Fatal("expected error for missing alert id")
	}
}

func TestQueryIncidentEntries(t *testing.T) {
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

	entries := []*IncidentEntry{
		{
			AlertID:         "a-1",
			Timestamp:       "2026-03-07T10:20:30Z",
			Status:          StatusTriggered,
			TriggerAiRca:    true,
			TriggeredAt:     "2026-03-07T10:20:30Z",
			Description:     "Issue one",
			NamespaceName:   "ns-1",
			ComponentName:   "comp-1",
			EnvironmentName: "dev",
			ProjectName:     "proj-1",
		},
		{
			AlertID:         "a-2",
			Timestamp:       "2026-03-07T10:22:30Z",
			Status:          StatusResolved,
			TriggerAiRca:    false,
			TriggeredAt:     "2026-03-07T10:21:00Z",
			ResolvedAt:      "2026-03-07T10:22:30Z",
			Description:     "Issue two",
			NamespaceName:   "ns-2",
			ComponentName:   "comp-2",
			EnvironmentName: "prod",
			ProjectName:     "proj-2",
		},
	}
	for _, entry := range entries {
		if _, err := store.WriteIncidentEntry(ctx, entry); err != nil {
			t.Fatalf("failed to write incident entry: %v", err)
		}
	}

	got, total, err := store.QueryIncidentEntries(ctx, QueryParams{
		StartTime:     "2026-03-07T10:00:00Z",
		EndTime:       "2026-03-07T11:00:00Z",
		NamespaceName: "ns-2",
		Limit:         10,
		SortOrder:     "desc",
	})
	if err != nil {
		t.Fatalf("failed to query incident entries: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].AlertID != "a-2" {
		t.Fatalf("expected alert a-2, got %s", got[0].AlertID)
	}
}
