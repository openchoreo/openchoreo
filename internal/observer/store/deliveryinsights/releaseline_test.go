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

func lineFact(releaseUID, lineUID string, readyMs int64) DeploymentFact {
	f := testFact(releaseUID, readyMs)
	f.ReleaseLineUID = lineUID
	return f
}

func TestDeploymentFacts_ReleaseLine(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	deployed := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{
		lineFact("rel-main", "line-main", deployed),
		lineFact("rel-next-1", "line-next", deployed+time.Minute.Milliseconds()),
		lineFact("rel-next-2", "line-next", deployed+2*time.Minute.Milliseconds()),
	}))

	window := FactQuery{ComponentUID: "comp-1", StartMs: deployed - 1000, EndMs: deployed + time.Hour.Milliseconds()}
	counts, err := store.CountDeployments(ctx, window)
	require.NoError(t, err)
	assert.Equal(t, 3, counts.Total, "the component counts every line")

	window.ReleaseLineUID = "line-next"
	counts, err = store.CountDeployments(ctx, window)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Total)

	facts, _, err := store.QueryDeploymentFacts(ctx, window)
	require.NoError(t, err)
	require.Len(t, facts, 2)
	assert.Equal(t, "line-next", facts[0].ReleaseLineUID)

	// A later write without the line keeps the line already recorded.
	update := testFact("rel-next-1", deployed+time.Minute.Milliseconds())
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{update}))
	counts, err = store.CountDeployments(ctx, window)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Total)
}

func TestAttributeIncident_ReleaseLine(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	deployed := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	window := 24 * time.Hour.Milliseconds()
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{
		lineFact("rel-next", "line-next", deployed),
		// main deployed later, so it is the component's live deployment at the trigger.
		lineFact("rel-main", "line-main", deployed+10*time.Minute.Milliseconds()),
	}))

	result, err := store.AttributeIncident(ctx, "comp-1", "env-prod", "line-next", "inc-next",
		deployed+30*time.Minute.Milliseconds(), window)
	require.NoError(t, err)
	assert.True(t, result.Attributed)
	assert.Equal(t, "rel-next", result.ReleaseUID, "an incident of a line is attributed to that line")
}

func TestBuildRollups_ReleaseLine(t *testing.T) {
	t.Parallel()

	deployed := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	rollups := BuildRollups([]DeploymentFact{
		lineFact("rel-main", "line-main", deployed),
		lineFact("rel-next", "line-next", deployed),
		testFact("rel-plain", deployed),
	}, nil, deployed)

	deploys := func(scopeType, scopeUID, environmentUID string) int {
		for _, r := range rollups {
			if r.ScopeType == scopeType && r.ScopeUID == scopeUID && r.EnvironmentUID == environmentUID &&
				r.Granularity == GranularityDaily {
				return r.DeployTotal
			}
		}
		return 0
	}
	assert.Equal(t, 3, deploys(ScopeTypeComponent, "comp-1", ""))
	assert.Equal(t, 1, deploys(ScopeTypeReleaseLine, "line-next", ""))
	assert.Equal(t, 1, deploys(ScopeTypeReleaseLine, "line-next", "env-prod"))
	assert.Equal(t, 0, deploys(ScopeTypeReleaseLine, "", ""), "facts without a line have no line rollup")
}
