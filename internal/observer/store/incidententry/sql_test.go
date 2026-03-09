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
		ComponentID:     "cmp-1",
		EnvironmentID:   "env-1",
		ProjectID:       "prj-1",
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
