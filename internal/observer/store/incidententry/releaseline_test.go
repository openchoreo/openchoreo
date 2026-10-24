// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncidentEntries_ReleaseLine(t *testing.T) {
	t.Parallel()

	store, err := New(BackendSQLite, "file:"+filepath.Join(t.TempDir(), "incidents.db"), slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx))

	var nextID string
	for _, line := range []struct{ name, id string }{{"main", "line-main"}, {"next", "line-next"}} {
		id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
			AlertID:         "alert-" + line.name,
			Timestamp:       "2026-03-07T10:20:30Z",
			ComponentID:     "component-1",
			ReleaseLineName: line.name,
			ReleaseLineID:   line.id,
		})
		require.NoError(t, err)
		if line.name == "next" {
			nextID = id
		}
	}

	params := QueryParams{StartTime: "2026-03-07T00:00:00Z", EndTime: "2026-03-08T00:00:00Z", ComponentID: "component-1"}
	_, total, err := store.QueryIncidentEntries(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	params.ReleaseLineID = "line-next"
	entries, total, err := store.QueryIncidentEntries(ctx, params)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "next", entries[0].ReleaseLineName)

	got, err := store.GetIncidentEntry(ctx, nextID)
	require.NoError(t, err)
	assert.Equal(t, "line-next", got.ReleaseLineID)
}

func TestIncidentEntries_DefaultLineIncludesRowsFromBeforeReleaseLines(t *testing.T) {
	t.Parallel()

	dsn := "file:" + filepath.Join(t.TempDir(), "incidents.db")
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx))

	for _, line := range []struct{ name, id string }{{"old", ""}, {"", ""}, {"next", "line-next"}} {
		_, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
			AlertID:         "alert-" + line.name,
			Timestamp:       "2026-03-07T10:20:30Z",
			ComponentID:     "component-1",
			ReleaseLineName: line.name,
			ReleaseLineID:   line.id,
		})
		require.NoError(t, err)
	}
	// A row stored before release lines has no value in the release line columns.
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE incident_entries SET release_line_name = NULL, release_line_id = NULL WHERE alert_id = 'alert-old'`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, total, err := store.QueryIncidentEntries(ctx, QueryParams{
		StartTime: "2026-03-07T00:00:00Z", EndTime: "2026-03-08T00:00:00Z", ComponentID: "component-1", DefaultLineOnly: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "the default line covers rows with an empty or missing release line")
}
