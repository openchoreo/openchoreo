// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryAlertEntries_ReleaseLine(t *testing.T) {
	t.Parallel()

	store, err := New(BackendSQLite, "file:"+filepath.Join(t.TempDir(), "alerts.db"), slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx))

	for _, line := range []struct{ name, id string }{{"main", "line-main"}, {"next", "line-next"}, {"", ""}} {
		_, err := store.WriteAlertEntry(ctx, &AlertEntry{
			Timestamp:       "2026-03-07T10:20:30Z",
			AlertRuleName:   "high-error-rate-" + line.name,
			ComponentID:     "component-1",
			ReleaseLineName: line.name,
			ReleaseLineID:   line.id,
		})
		require.NoError(t, err)
	}

	params := QueryParams{StartTime: "2026-03-07T00:00:00Z", EndTime: "2026-03-08T00:00:00Z", ComponentID: "component-1"}
	_, total, err := store.QueryAlertEntries(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, 3, total, "a component-scoped query returns every line")

	params.ReleaseLineID = "line-next"
	entries, total, err := store.QueryAlertEntries(ctx, params)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "next", entries[0].ReleaseLineName)
	assert.Equal(t, "line-next", entries[0].ReleaseLineID)
}

func TestInitialize_AddsReleaseLineColumnsToExistingTable(t *testing.T) {
	t.Parallel()

	dsn := "file:" + filepath.Join(t.TempDir(), "alerts.db")
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	// alert_entries as created before release lines.
	_, err = db.Exec(`CREATE TABLE alert_entries (
		id TEXT PRIMARY KEY, timestamp_ns BIGINT NOT NULL, alert_rule_name TEXT NOT NULL,
		alert_rule_cr_name TEXT, alert_rule_cr_namespace TEXT, alert_value TEXT,
		namespace_name TEXT, component_name TEXT, environment_name TEXT, project_name TEXT,
		component_id TEXT, environment_id TEXT, project_id TEXT, incident_enabled BOOLEAN NOT NULL,
		severity TEXT, description TEXT, notification_channels TEXT,
		source_type TEXT, source_query TEXT, source_metric TEXT,
		condition_operator TEXT, condition_threshold REAL, condition_window TEXT, condition_interval TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO alert_entries VALUES ('old', 1772878830000000000, 'old-rule',
		'', '', '', '', '', '', '', '', '', '', false, '', '', '', '', '', '', '', 0, '', '');`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx))
	require.NoError(t, store.Initialize(ctx), "initializing again leaves the columns in place")

	entries, total, err := store.QueryAlertEntries(ctx, QueryParams{StartTime: "2026-03-07T00:00:00Z", EndTime: "2026-03-08T00:00:00Z"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Empty(t, entries[0].ReleaseLineName, "rows written before release lines have no line")
}
