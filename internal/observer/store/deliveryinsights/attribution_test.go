// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deliveryinsights

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttributeIncidentMarksLiveDeployment(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	deployed := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	window := 24 * time.Hour.Milliseconds()

	older := testFact("rel-old", deployed-2*time.Hour.Milliseconds())
	live := testFact("rel-live", deployed)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{older, live}))

	// Incident 30 minutes after the live deployment attributes to it, not the older one.
	result, err := store.AttributeIncident(ctx, "comp-1", "env-prod", "inc-1",
		deployed+30*time.Minute.Milliseconds(), window)
	require.NoError(t, err)
	assert.True(t, result.Attributed)
	assert.Equal(t, "rel-live", result.ReleaseUID)
	assert.Equal(t, deployed, result.OccurredMs)

	facts, _, err := store.QueryDeploymentFacts(ctx, FactQuery{
		ComponentUID: "comp-1", StartMs: deployed - 1000, EndMs: deployed + 1000,
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, OutcomeFailed, facts[0].Outcome)
	assert.Equal(t, FailedByIncident, facts[0].FailedBy)
	assert.Equal(t, "inc-1", facts[0].IncidentID)

	// Re-processing the same incident is a no-op (fact already attributed).
	again, err := store.AttributeIncident(ctx, "comp-1", "env-prod", "inc-1",
		deployed+30*time.Minute.Milliseconds(), window)
	require.NoError(t, err)
	assert.False(t, again.Attributed, "already-attributed fact must not re-attribute")
	assert.Equal(t, "rel-live", again.ReleaseUID, "match is still reported for rollup touching")
}

func TestAttributeIncidentRespectsWindowAndPrecedence(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	deployed := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	window := 24 * time.Hour.Milliseconds()

	fact := testFact("rel-1", deployed)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{fact}))

	// Outside the attribution window: no match at all.
	result, err := store.AttributeIncident(ctx, "comp-1", "env-prod", "inc-late",
		deployed+window+1000, window)
	require.NoError(t, err)
	assert.False(t, result.Attributed)
	assert.Empty(t, result.ReleaseUID)

	// Before any deployment: no match.
	result, err = store.AttributeIncident(ctx, "comp-1", "env-prod", "inc-early",
		deployed-1000, window)
	require.NoError(t, err)
	assert.Empty(t, result.ReleaseUID)

	// Rollout failures take precedence over incident attribution.
	rolloutFailed := testFact("rel-2", deployed+time.Hour.Milliseconds())
	rolloutFailed.Outcome = OutcomeFailed
	rolloutFailed.FailedBy = FailedByRollout
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{rolloutFailed}))

	result, err = store.AttributeIncident(ctx, "comp-1", "env-prod", "inc-2",
		deployed+90*time.Minute.Milliseconds(), window)
	require.NoError(t, err)
	assert.Equal(t, "rel-2", result.ReleaseUID)
	assert.False(t, result.Attributed, "rollout failure must not be overwritten")

	// Unscoped incidents (no component/environment UID) are skipped silently.
	result, err = store.AttributeIncident(ctx, "", "", "inc-3", deployed, window)
	require.NoError(t, err)
	assert.Empty(t, result.ReleaseUID)
}

func TestRecoveryUpsertDerivesDurationFromMergedRow(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	failedAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	recoveredAt := failedAt + 45*time.Minute.Milliseconds()

	// Phase 1: the Failed event opens the episode.
	require.NoError(t, store.UpsertRecoveryFacts(ctx, []RecoveryFact{{
		ID: "health-rel-1", OrgNamespace: "default", ComponentUID: "comp-1",
		EnvironmentUID: "env-prod", Source: RecoverySourceHealth,
		FailureStartedMs: failedAt,
	}}))

	// Phase 2: the Recovered event closes it — carrying its own timestamp as the
	// failure start (the folder has no memory of the original Failed event).
	require.NoError(t, store.UpsertRecoveryFacts(ctx, []RecoveryFact{{
		ID: "health-rel-1", OrgNamespace: "default", ComponentUID: "comp-1",
		EnvironmentUID: "env-prod", Source: RecoverySourceHealth,
		FailureStartedMs: recoveredAt, RecoveredMs: &recoveredAt,
	}}))

	facts, err := store.QueryRecoveryFacts(ctx, FactQuery{
		ComponentUID: "comp-1", StartMs: failedAt - 1000, EndMs: recoveredAt + 1000,
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, failedAt, facts[0].FailureStartedMs, "failure start must not move on merge")
	require.NotNil(t, facts[0].DurationMs)
	assert.Equal(t, 45*time.Minute.Milliseconds(), *facts[0].DurationMs,
		"duration must derive from the merged row, not the Recovered write alone")
}
