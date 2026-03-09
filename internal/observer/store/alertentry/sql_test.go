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
		ComponentID:          "cmp-1",
		EnvironmentID:        "env-1",
		ProjectID:            "prj-1",
		IncidentEnabled:      true,
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
