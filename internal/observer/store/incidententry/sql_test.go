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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteInitializeAndWriteIncidentEntry(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dsn := "file:" + filepath.Join(tempDir, "incidents.db")

	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "alt-1",
		Timestamp:             "2026-03-07T10:20:30Z",
		Status:                StatusActive,
		TriggerAiRca:          true,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           "2026-03-07T10:20:30Z",
		Description:           "High error rate observed",
		NamespaceName:         "choreo-prod",
		ComponentName:         "payments",
		EnvironmentName:       "prod",
		ProjectName:           "commerce",
		ComponentID:           "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		EnvironmentID:         "d4e5f6a7-8901-23de-f012-4567890abcde",
		ProjectID:             "b2c3d4e5-6789-01bc-def0-234567890abc",
	})
	require.NoError(t, err, "failed to write incident entry")
	require.NotEmpty(t, id)

	_, statErr := os.Stat(filepath.Join(tempDir, "incidents.db"))
	require.NoError(t, statErr, "expected sqlite db file to exist")
}

func TestWriteIncidentEntryWithNilEntry(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	_, err = store.WriteIncidentEntry(ctx, nil)
	require.Error(t, err, "expected error for nil incident entry")
}

func TestWriteIncidentEntryWithMissingAlertID(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	_, err = store.WriteIncidentEntry(ctx, &IncidentEntry{})
	require.Error(t, err, "expected error for missing alert id")
}

func TestQueryIncidentEntries(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	entries := []*IncidentEntry{
		{
			AlertID:               "a-1",
			Timestamp:             "2026-03-07T10:20:30Z",
			Status:                StatusActive,
			TriggerAiRca:          true,
			TriggerAiCostAnalysis: false,
			TriggeredAt:           "2026-03-07T10:20:30Z",
			Description:           "Issue one",
			NamespaceName:         "ns-1",
			ComponentName:         "comp-1",
			EnvironmentName:       "dev",
			ProjectName:           "proj-1",
		},
		{
			AlertID:               "a-2",
			Timestamp:             "2026-03-07T10:22:30Z",
			Status:                StatusResolved,
			TriggerAiRca:          false,
			TriggerAiCostAnalysis: false,
			TriggeredAt:           "2026-03-07T10:21:00Z",
			ResolvedAt:            "2026-03-07T10:22:30Z",
			Description:           "Issue two",
			NamespaceName:         "ns-2",
			ComponentName:         "comp-2",
			EnvironmentName:       "prod",
			ProjectName:           "proj-2",
		},
	}
	for _, entry := range entries {
		_, err := store.WriteIncidentEntry(ctx, entry)
		require.NoError(t, err, "failed to write incident entry")
	}

	got, total, err := store.QueryIncidentEntries(ctx, QueryParams{
		StartTime:     "2026-03-07T10:00:00Z",
		EndTime:       "2026-03-07T11:00:00Z",
		NamespaceName: "ns-2",
		Limit:         10,
		SortOrder:     "desc",
	})
	require.NoError(t, err, "failed to query incident entries")
	require.Equal(t, 1, total)
	require.Len(t, got, 1)
	assert.Equal(t, "a-2", got[0].AlertID)
}

func TestUpdateIncidentEntry_AcknowledgeAndResolve(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-ack",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusActive,
		TriggerAiRca:          true,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		Description:           "Needs attention",
		NamespaceName:         "team-a",
		ComponentName:         "component-a",
		EnvironmentName:       "dev",
		ProjectName:           "project-a",
	})
	require.NoError(t, err, "failed to write incident entry")

	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	sqlStore := store.(*sqlStore)

	ackNotes := "ack-notes"
	ackDesc := "ack-desc"
	updated, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, &ackNotes, &ackDesc, now)
	require.NoError(t, err, "failed to acknowledge incident")
	assert.Equal(t, StatusAcknowledged, updated.Status)
	assert.NotEmpty(t, updated.AcknowledgedAt)
	assert.Empty(t, updated.ResolvedAt)
	assert.Equal(t, "ack-notes", updated.Notes)
	assert.Equal(t, "ack-desc", updated.Description)

	resolveTime := now.Add(5 * time.Minute)
	resNotes := "res-notes"
	resDesc := "res-desc"
	resolved, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusResolved, &resNotes, &resDesc, resolveTime)
	require.NoError(t, err, "failed to resolve incident")
	assert.Equal(t, StatusResolved, resolved.Status)
	assert.NotEmpty(t, resolved.ResolvedAt)
	assert.Equal(t, "res-notes", resolved.Notes)
	assert.Equal(t, "res-desc", resolved.Description)
}

func TestUpdateIncidentEntry_NotFound(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	sqlStore := store.(*sqlStore)
	_, err = sqlStore.UpdateIncidentEntry(ctx, "non-existent-id", StatusActive, nil, nil, time.Now())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncidentNotFound)
}

func TestUpdateIncidentEntry_PreservesOmittedFields(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-preserve",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusActive,
		TriggerAiRca:          true,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		Notes:                 "original-notes",
		Description:           "original-description",
		NamespaceName:         "team-a",
		ComponentName:         "component-a",
		EnvironmentName:       "dev",
		ProjectName:           "project-a",
	})
	require.NoError(t, err, "failed to write incident entry")

	sqlStore := store.(*sqlStore)
	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)

	// Omit notes and description (pass nil) - should preserve existing values
	updated, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, now)
	require.NoError(t, err, "failed to update incident")
	assert.Equal(t, "original-notes", updated.Notes)
	assert.Equal(t, "original-description", updated.Description)

	// Verify persisted: query back and check
	entries, _, err := store.QueryIncidentEntries(ctx, QueryParams{
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
		Limit:     10,
	})
	require.NoError(t, err, "failed to query")
	var found *IncidentEntry
	for i := range entries {
		if entries[i].ID == id {
			found = &entries[i]
			break
		}
	}
	require.NotNil(t, found, "incident not found after update")
	assert.Equal(t, "original-notes", found.Notes)
	assert.Equal(t, "original-description", found.Description)
}

func TestUpdateIncidentEntry_ForwardOnlyTransitions(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	sqlStore := store.(*sqlStore)

	// Write an already-resolved incident.
	resolvedID, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-resolved",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusResolved,
		TriggerAiRca:          false,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		ResolvedAt:            createdAt.Add(time.Minute).Format(time.RFC3339Nano),
		NamespaceName:         "ns",
		ComponentName:         "comp",
		EnvironmentName:       "env",
		ProjectName:           "proj",
	})
	require.NoError(t, err, "failed to write resolved incident")

	// Backward transition: resolved -> acknowledged should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, resolvedID, StatusAcknowledged, nil, nil, time.Now())
	require.ErrorIs(t, err, ErrInvalidStatusTransition)

	// Backward transition: resolved -> active should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, resolvedID, StatusActive, nil, nil, time.Now())
	require.ErrorIs(t, err, ErrInvalidStatusTransition)

	// Write an acknowledged incident.
	ackID, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-ack",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusAcknowledged,
		TriggerAiRca:          false,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		AcknowledgedAt:        createdAt.Add(30 * time.Second).Format(time.RFC3339Nano),
		NamespaceName:         "ns",
		ComponentName:         "comp",
		EnvironmentName:       "env",
		ProjectName:           "proj",
	})
	require.NoError(t, err, "failed to write acknowledged incident")

	// Backward transition: acknowledged -> active should fail.
	_, err = sqlStore.UpdateIncidentEntry(ctx, ackID, StatusActive, nil, nil, time.Now())
	require.ErrorIs(t, err, ErrInvalidStatusTransition)

	// Allowed transition: acknowledged -> resolved should succeed.
	updated, err := sqlStore.UpdateIncidentEntry(ctx, ackID, StatusResolved, nil, nil, time.Now())
	require.NoError(t, err, "expected acknowledged->resolved to succeed")
	assert.Equal(t, StatusResolved, updated.Status)
}

func TestUpdateIncidentEntry_TimestampsSetOnce(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-ts-once",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusActive,
		TriggerAiRca:          false,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		NamespaceName:         "ns",
		ComponentName:         "comp",
		EnvironmentName:       "env",
		ProjectName:           "proj",
	})
	require.NoError(t, err, "failed to write incident")

	sqlStore := store.(*sqlStore)
	ackTime1 := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	first, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, ackTime1)
	require.NoError(t, err, "first acknowledge failed")
	firstAckAt := first.AcknowledgedAt

	// Re-acknowledge with a later time; acknowledgedAt must NOT change.
	ackTime2 := ackTime1.Add(10 * time.Minute)
	second, err := sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, ackTime2)
	require.NoError(t, err, "second acknowledge failed")
	assert.Equal(t, firstAckAt, second.AcknowledgedAt)
}

func TestUpdateIncidentEntry_RowsAffectedZeroIsNotFound(t *testing.T) {
	t.Parallel()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	// Write then hard-delete a row to simulate a row disappearing between SELECT and UPDATE.
	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:               "a-deleted",
		Timestamp:             createdAt.Format(time.RFC3339Nano),
		Status:                StatusActive,
		TriggerAiRca:          false,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           createdAt.Format(time.RFC3339Nano),
		NamespaceName:         "ns",
		ComponentName:         "comp",
		EnvironmentName:       "env",
		ProjectName:           "proj",
	})
	require.NoError(t, err, "failed to write incident")

	// Delete the row directly so the UPDATE inside the transaction finds 0 rows.
	sqlStore := store.(*sqlStore)
	_, err = sqlStore.db.ExecContext(ctx, "DELETE FROM incident_entries WHERE id = ?", id)
	require.NoError(t, err, "failed to delete row")

	_, err = sqlStore.UpdateIncidentEntry(ctx, id, StatusAcknowledged, nil, nil, time.Now())
	require.ErrorIs(t, err, ErrIncidentNotFound)
}
