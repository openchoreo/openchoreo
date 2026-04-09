// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore is a helper that every test calls. It:
//  1. Creates a SQLite in-memory database (unique per test via t.Name())
//  2. Initializes the schema (CREATE TABLE, indexes)
//  3. Registers cleanup to close the DB when the test ends
//
// This means each test starts with a fresh, empty database.
func newTestStore(t *testing.T) ReportStore {
	t.Helper() // marks this as a helper so test failures point to the caller, not here

	// t.Name() returns something like "TestUpsertReport/insert_new_report".
	// We use it as the DB name so each test (and sub-test) gets its own database.
	// "mode=memory" = in-memory, "cache=shared" = accessible within the same process.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared",
		strings.ReplaceAll(t.Name(), "/", "-"))

	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")

	// t.Cleanup runs this function when the test finishes, even if it fails.
	// This ensures we always close the database connection.
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")

	return store
}

// newTestEntry creates a ReportEntry with sensible defaults.
// You can override fields after calling this.
func newTestEntry(reportID, status string) *ReportEntry {
	return &ReportEntry{
		ReportID:        reportID,
		AlertID:         "alert-" + reportID,
		Status:          status,
		Timestamp:       "2026-03-07T10:00:00Z",
		NamespaceName:   "test-ns",
		ProjectName:     "test-project",
		EnvironmentName: "dev",
		ComponentName:   "test-component",
		EnvironmentUID:  "env-uid-1",
		ProjectUID:      "proj-uid-1",
	}
}

// This test verifies the basic insert path: write a report, read it back,
// and confirm all fields match.
func TestUpsertReport_InsertNew(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	summary := "high error rate detected"
	report := `{"root_cause": "OOM"}`
	entry := newTestEntry("rpt-1", "completed")
	entry.Summary = &summary
	entry.Report = &report

	// Insert
	err := store.UpsertReport(ctx, entry)
	require.NoError(t, err)

	// Read back
	got, err := store.GetReport(ctx, "rpt-1")
	require.NoError(t, err)

	// Check every field. assert (not require) so we see ALL mismatches at once.
	assert.Equal(t, "rpt-1", got.ReportID)
	assert.Equal(t, "alert-rpt-1", got.AlertID)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, "high error rate detected", *got.Summary)
	assert.Equal(t, "test-ns", got.NamespaceName)
	assert.Equal(t, "test-project", got.ProjectName)
	assert.Equal(t, "dev", got.EnvironmentName)
	assert.Equal(t, "test-component", got.ComponentName)
	assert.Equal(t, "env-uid-1", got.EnvironmentUID)
	assert.Equal(t, "proj-uid-1", got.ProjectUID)
	assert.Equal(t, `{"root_cause": "OOM"}`, *got.Report)
}

// This test verifies the "upsert" behavior: if a report with the same ID
// already exists, the second write should UPDATE it rather than fail.
func TestUpsertReport_UpdateExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// First insert: status "pending", no summary
	entry := newTestEntry("rpt-1", "pending")
	require.NoError(t, store.UpsertReport(ctx, entry))

	// Second insert (same report_id): status changes to "completed", adds summary
	summary := "root cause found"
	report := `{"findings": []}`
	entry.Status = "completed"
	entry.Summary = &summary
	entry.Report = &report
	require.NoError(t, store.UpsertReport(ctx, entry))

	// Verify the update took effect
	got, err := store.GetReport(ctx, "rpt-1")
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, "root cause found", *got.Summary)
	assert.Equal(t, `{"findings": []}`, *got.Report)
}

// The upsert query has COALESCE(excluded.report, rca_reports.report),
// meaning: if the new report value is NULL, keep the existing one.
// This test verifies that behavior.
func TestUpsertReport_PreservesExistingReportWhenNewIsNil(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert with a report
	report := `{"root_cause": "OOM"}`
	entry := newTestEntry("rpt-1", "completed")
	entry.Report = &report
	require.NoError(t, store.UpsertReport(ctx, entry))

	// Upsert again with nil report — should keep the old one
	entry.Report = nil
	entry.Status = "failed"
	require.NoError(t, store.UpsertReport(ctx, entry))

	got, err := store.GetReport(ctx, "rpt-1")
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	require.NotNil(t, got.Report, "existing report should be preserved")
	assert.Equal(t, `{"root_cause": "OOM"}`, *got.Report)
}

func TestUpsertReport_NilEntry(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.UpsertReport(context.Background(), nil)
	require.Error(t, err)
}

// When Timestamp is empty, UpsertReport should fill it with the current time
// rather than storing an empty string.
func TestUpsertReport_DefaultsTimestamp(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	entry := newTestEntry("rpt-1", "pending")
	entry.Timestamp = "" // empty
	require.NoError(t, store.UpsertReport(ctx, entry))

	got, err := store.GetReport(ctx, "rpt-1")
	require.NoError(t, err)
	assert.NotEmpty(t, got.Timestamp, "timestamp should be auto-filled")
}

func TestGetReport_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, err := store.GetReport(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestListReports_FilterByStatus(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert 3 reports with different statuses
	for _, s := range []string{"pending", "completed", "failed"} {
		entry := newTestEntry("rpt-"+s, s)
		require.NoError(t, store.UpsertReport(ctx, entry))
	}

	// Query only "completed"
	results, total, err := store.ListReports(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		Status:    "completed",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "rpt-completed", results[0].ReportID)
}

func TestListReports_FilterByProjectAndEnvironment(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Two reports, different projects
	e1 := newTestEntry("rpt-1", "completed")
	e1.ProjectUID = "proj-A"
	e1.EnvironmentUID = "env-A"
	require.NoError(t, store.UpsertReport(ctx, e1))

	e2 := newTestEntry("rpt-2", "completed")
	e2.ProjectUID = "proj-B"
	e2.EnvironmentUID = "env-B"
	require.NoError(t, store.UpsertReport(ctx, e2))

	results, total, err := store.ListReports(ctx, QueryParams{
		StartTime:      "2026-03-07T00:00:00Z",
		EndTime:        "2026-03-08T00:00:00Z",
		ProjectUID:     "proj-A",
		EnvironmentUID: "env-A",
		Limit:          10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "rpt-1", results[0].ReportID)
}

func TestListReports_SortOrder(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert reports with different timestamps
	e1 := newTestEntry("rpt-early", "completed")
	e1.Timestamp = "2026-03-07T08:00:00Z"
	require.NoError(t, store.UpsertReport(ctx, e1))

	e2 := newTestEntry("rpt-late", "completed")
	e2.Timestamp = "2026-03-07T20:00:00Z"
	require.NoError(t, store.UpsertReport(ctx, e2))

	// DESC — latest first
	results, _, err := store.ListReports(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		SortOrder: "DESC",
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "rpt-late", results[0].ReportID)
	assert.Equal(t, "rpt-early", results[1].ReportID)

	// ASC — earliest first
	results, _, err = store.ListReports(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		SortOrder: "ASC",
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "rpt-early", results[0].ReportID)
	assert.Equal(t, "rpt-late", results[1].ReportID)
}

func TestListReports_Limit(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert 5 reports
	for i := 0; i < 5; i++ {
		entry := newTestEntry(fmt.Sprintf("rpt-%d", i), "completed")
		require.NoError(t, store.UpsertReport(ctx, entry))
	}

	// Ask for only 2
	results, total, err := store.ListReports(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		Limit:     2,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, total, "total should reflect ALL matching rows")
	assert.Len(t, results, 2, "returned rows should respect the limit")
}

func TestListReports_TimeRange(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// One inside the range, one outside
	inside := newTestEntry("rpt-inside", "completed")
	inside.Timestamp = "2026-03-07T12:00:00Z"
	require.NoError(t, store.UpsertReport(ctx, inside))

	outside := newTestEntry("rpt-outside", "completed")
	outside.Timestamp = "2026-03-09T12:00:00Z"
	require.NoError(t, store.UpsertReport(ctx, outside))

	results, total, err := store.ListReports(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "rpt-inside", results[0].ReportID)
}

func TestListReports_InvalidSortOrder(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, _, err := store.ListReports(context.Background(), QueryParams{
		StartTime: "2026-03-07T00:00:00Z",
		EndTime:   "2026-03-08T00:00:00Z",
		SortOrder: "INVALID",
		Limit:     10,
	})
	require.Error(t, err)
}

func TestListReports_InvalidTimestamp(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, _, err := store.ListReports(context.Background(), QueryParams{
		StartTime: "not-a-timestamp",
		EndTime:   "2026-03-08T00:00:00Z",
		Limit:     10,
	})
	require.Error(t, err)
}

func TestUpdateActionStatuses(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert a report with an initial report JSON
	report := `{"actions": [{"id": "a1", "status": "pending"}]}`
	entry := newTestEntry("rpt-1", "completed")
	entry.Report = &report
	require.NoError(t, store.UpsertReport(ctx, entry))

	// Update the action statuses
	updated := `{"actions": [{"id": "a1", "status": "applied"}]}`
	err := store.UpdateActionStatuses(ctx, "rpt-1", updated)
	require.NoError(t, err)

	// Verify
	got, err := store.GetReport(ctx, "rpt-1")
	require.NoError(t, err)
	require.NotNil(t, got.Report)
	assert.Equal(t, updated, *got.Report)
}

func TestUpdateActionStatuses_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.UpdateActionStatuses(context.Background(), "nonexistent", `{}`)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
