// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore returns an initialized in-memory SQLite store.
func newTestStore(t *testing.T) IncidentEntryStore {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})
	require.NoError(t, store.Initialize(context.Background()), "failed to initialize store")
	return store
}

// TestGetIncidentEntry covers the read the authorization check depends on: it
// must return the incident's namespace/project/component so UpdateIncident can
// be authorized against the incident's own hierarchy rather than a wildcard.
func TestGetIncidentEntry(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	createdAt := time.Date(2026, 3, 7, 10, 20, 30, 0, time.UTC)
	id, err := store.WriteIncidentEntry(ctx, &IncidentEntry{
		AlertID:         "a-get",
		Timestamp:       createdAt.Format(time.RFC3339Nano),
		Status:          StatusActive,
		TriggeredAt:     createdAt.Format(time.RFC3339Nano),
		Description:     "desc",
		Notes:           "note",
		NamespaceName:   "team-a",
		ProjectName:     "proj-a",
		ComponentName:   "component-a",
		EnvironmentName: "env-a",
	})
	require.NoError(t, err, "failed to write incident")

	got, err := store.GetIncidentEntry(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, id, got.ID)
	assert.Equal(t, "a-get", got.AlertID)
	assert.Equal(t, StatusActive, got.Status)
	assert.Equal(t, "note", got.Notes)
	assert.Equal(t, "desc", got.Description)

	// The hierarchy fields are the reason this method exists.
	assert.Equal(t, "team-a", got.NamespaceName)
	assert.Equal(t, "proj-a", got.ProjectName)
	assert.Equal(t, "component-a", got.ComponentName)
	assert.Equal(t, "env-a", got.EnvironmentName)
}

func TestGetIncidentEntry_NotFound(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.GetIncidentEntry(context.Background(), "missing")
	require.ErrorIs(t, err, ErrIncidentNotFound,
		"an unknown id must be distinguishable, so the authz wrapper can surface a 404")
}

func TestGetIncidentEntry_EmptyID(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.GetIncidentEntry(context.Background(), "  ")
	require.Error(t, err)
}
