// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		Status:          StatusActive,
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
			Status:          StatusActive,
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

func TestUpdateIncidentEntry_AcknowledgeAndResolve(t *testing.T) {
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

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-ack",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusActive,
		TriggerAiRca:    true,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		Description:     "Needs attention",
		NamespaceName:   "team-a",
		ComponentName:   "component-a",
		EnvironmentName: "dev",
		ProjectName:     "project-a",
	})
	if err != nil {
		t.Fatalf("failed to write incident entry: %v", err)
	}

	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	sqlStore := store.(*sqlStore)

	ackNotes := "ack-notes"
	ackDesc := "ack-desc"
	updated, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, &ackNotes, &ackDesc, now)
	if err != nil {
		t.Fatalf("failed to acknowledge incident: %v", err)
	}
	if updated.Status != StatusAcknowledged {
		t.Fatalf("expected status %q, got %q", StatusAcknowledged, updated.Status)
	}
	if updated.AcknowledgedAt == "" {
		t.Fatalf("expected acknowledgedAt to be set")
	}
	if updated.ResolvedAt != "" {
		t.Fatalf("expected resolvedAt to be empty, got %q", updated.ResolvedAt)
	}
	if updated.Notes != "ack-notes" {
		t.Fatalf("expected notes %q, got %q", "ack-notes", updated.Notes)
	}
	if updated.Description != "ack-desc" {
		t.Fatalf("expected description %q, got %q", "ack-desc", updated.Description)
	}

	resolveTime := now.Add(5 * time.Minute)
	resNotes := "res-notes"
	resDesc := "res-desc"
	resolved, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusResolved, &resNotes, &resDesc, resolveTime)
	if err != nil {
		t.Fatalf("failed to resolve incident: %v", err)
	}
	if resolved.Status != StatusResolved {
		t.Fatalf("expected status %q, got %q", StatusResolved, resolved.Status)
	}
	if resolved.ResolvedAt == "" {
		t.Fatalf("expected resolvedAt to be set")
	}
	if resolved.Notes != "res-notes" {
		t.Fatalf("expected notes %q, got %q", "res-notes", resolved.Notes)
	}
	if resolved.Description != "res-desc" {
		t.Fatalf("expected description %q, got %q", "res-desc", resolved.Description)
	}
}

func TestUpdateIncidentEntry_NotFound(t *testing.T) {
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

	sqlStore := store.(*sqlStore)
	_, err = sqlStore.UpdateIncidentEntry(ctx, "non-existent-id", StatusActive, nil, nil, time.Now())
	if err == nil {
		t.Fatal("expected error for non-existent incident id")
	}
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}

func TestUpdateIncidentEntry_PreservesOmittedFields(t *testing.T) {
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

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-preserve",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusActive,
		TriggerAiRca:    true,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		Notes:           "original-notes",
		Description:     "original-description",
		NamespaceName:   "team-a",
		ComponentName:   "component-a",
		EnvironmentName: "dev",
		ProjectName:     "project-a",
	})
	if err != nil {
		t.Fatalf("failed to write incident entry: %v", err)
	}

	sqlStore := store.(*sqlStore)
	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)

	// Omit notes and description (pass nil) - should preserve existing values
	updated, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, now)
	if err != nil {
		t.Fatalf("failed to update incident: %v", err)
	}
	if updated.Notes != "original-notes" {
		t.Fatalf("expected notes preserved %q, got %q", "original-notes", updated.Notes)
	}
	if updated.Description != "original-description" {
		t.Fatalf("expected description preserved %q, got %q", "original-description", updated.Description)
	}

	// Verify persisted: query back and check
	entries, _, err := store.QueryIncidentEntries(ctx, QueryParams{
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	var found *IncidentEntry
	for i := range entries {
		if entries[i].ID == id {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatal("incident not found after update")
	}
	if found.Notes != "original-notes" {
		t.Fatalf("expected persisted notes %q, got %q", "original-notes", found.Notes)
	}
	if found.Description != "original-description" {
		t.Fatalf("expected persisted description %q, got %q", "original-description", found.Description)
	}
}

func TestUpdateIncidentEntry_ForwardOnlyTransitions(t *testing.T) {
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

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	sqlStore := store.(*sqlStore)

	// Write an already-resolved incident.
	resolvedID, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-resolved",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusResolved,
		TriggerAiRca:    false,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		ResolvedAt:      createdAt.Add(time.Minute).Format(time.RFC3339Nano),
		NamespaceName:   "ns",
		ComponentName:   "comp",
		EnvironmentName: "env",
		ProjectName:     "proj",
	})
	if err != nil {
		t.Fatalf("failed to write resolved incident: %v", err)
	}

	// Backward transition: resolved → acknowledged should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, resolvedID, StatusAcknowledged, nil, nil, time.Now())
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition for resolved→acknowledged, got %v", err)
	}

	// Backward transition: resolved → active should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, resolvedID, StatusActive, nil, nil, time.Now())
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition for resolved→active, got %v", err)
	}

	// Write an acknowledged incident.
	ackID, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-ack",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusAcknowledged,
		TriggerAiRca:    false,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		AcknowledgedAt:  createdAt.Add(30 * time.Second).Format(time.RFC3339Nano),
		NamespaceName:   "ns",
		ComponentName:   "comp",
		EnvironmentName: "env",
		ProjectName:     "proj",
	})
	if err != nil {
		t.Fatalf("failed to write acknowledged incident: %v", err)
	}

	// Backward transition: acknowledged → active should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, ackID, StatusActive, nil, nil, time.Now())
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition for acknowledged→active, got %v", err)
	}

	// Allowed transition: acknowledged → resolved should succeed.
	updated, err := sqlStore.UpdateIncidentEntry(ctx, ackID, StatusResolved, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("expected acknowledged→resolved to succeed, got %v", err)
	}
	if updated.Status != StatusResolved {
		t.Fatalf("expected status %q, got %q", StatusResolved, updated.Status)
	}
}

func TestUpdateIncidentEntry_TimestampsSetOnce(t *testing.T) {
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

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-ts-once",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusActive,
		TriggerAiRca:    false,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		NamespaceName:   "ns",
		ComponentName:   "comp",
		EnvironmentName: "env",
		ProjectName:     "proj",
	})
	if err != nil {
		t.Fatalf("failed to write incident: %v", err)
	}

	sqlStore := store.(*sqlStore)
	ackTime1 := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	first, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, ackTime1)
	if err != nil {
		t.Fatalf("first acknowledge failed: %v", err)
	}
	firstAckAt := first.AcknowledgedAt

	// Re-acknowledge with a later time; acknowledgedAt must NOT change.
	ackTime2 := ackTime1.Add(10 * time.Minute)
	second, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, ackTime2)
	if err != nil {
		t.Fatalf("second acknowledge failed: %v", err)
	}
	if second.AcknowledgedAt != firstAckAt {
		t.Fatalf("expected acknowledgedAt to remain %q, got %q", firstAckAt, second.AcknowledgedAt)
	}
}

func TestUpdateIncidentEntry_RowsAffectedZeroIsNotFound(t *testing.T) {
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

	// Write then hard-delete a row to simulate a row disappearing between SELECT and UPDATE.
	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-deleted",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusActive,
		TriggerAiRca:    false,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		NamespaceName:   "ns",
		ComponentName:   "comp",
		EnvironmentName: "env",
		ProjectName:     "proj",
	})
	if err != nil {
		t.Fatalf("failed to write incident: %v", err)
	}

	// Delete the row directly so the UPDATE inside the transaction finds 0 rows.
	sqlStore := store.(*sqlStore)
	if _, err := sqlStore.db.ExecContext(ctx, "DELETE FROM incident_entries WHERE id = ?", id); err != nil {
		t.Fatalf("failed to delete row: %v", err)
	}

	_, err = sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, time.Now())
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound when row deleted between SELECT and UPDATE, got %v", err)
	}
}
