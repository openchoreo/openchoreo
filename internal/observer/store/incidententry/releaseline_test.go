// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
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
