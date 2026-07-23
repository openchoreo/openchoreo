// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deliveryinsights

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

func newTestStore(t *testing.T) Store {
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

func msPtr(v int64) *int64 {
	return &v
}

func testFact(releaseUID string, readyMs int64) DeploymentFact {
	authored := readyMs - 2*time.Hour.Milliseconds()
	lead := readyMs - authored
	return DeploymentFact{
		ReleaseUID:       releaseUID,
		OrgNamespace:     "default",
		ProjectUID:       "proj-1",
		ComponentUID:     "comp-1",
		EnvironmentUID:   "env-prod",
		ProjectName:      "checkout",
		ComponentName:    "api",
		EnvironmentName:  "prod",
		ComponentRelease: "api-7",
		CommitSHA:        "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		CommitAuthoredMs: &authored,
		StartedMs:        msPtr(readyMs - time.Minute.Milliseconds()),
		ReadyMs:          &readyMs,
		Outcome:          OutcomeSuccess,
		LeadTimeMs:       &lead,
		UpdatedAtMs:      readyMs,
	}
}

func TestSQLiteInitializeCreatesSchemaOnDisk(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dsn := "file:" + filepath.Join(tempDir, "insights.db")

	store, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err, "failed to create store")
	t.Cleanup(func() {
		require.NoError(t, store.Close(), "failed to close store")
	})

	ctx := context.Background()
	require.NoError(t, store.Initialize(ctx), "failed to initialize store")
	// Re-initialize must be a no-op (migrations already applied).
	require.NoError(t, store.Initialize(ctx), "re-initialize should be idempotent")

	_, statErr := os.Stat(filepath.Join(tempDir, "insights.db"))
	require.NoError(t, statErr, "expected sqlite db file to exist")
}

func TestUnsupportedBackend(t *testing.T) {
	t.Parallel()

	_, err := New("mongodb", "dsn", slog.Default())
	require.Error(t, err)
}

func TestUpsertDeploymentFactIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ready := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()

	fact := testFact("rel-1", ready)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{fact}))
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{fact}))

	facts, total, err := store.QueryDeploymentFacts(ctx, FactQuery{
		OrgNamespace: "default",
		StartMs:      ready - 1000,
		EndMs:        ready + 1000,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "duplicate upsert must collapse to one fact")
	require.Len(t, facts, 1)
	assert.Equal(t, OutcomeSuccess, facts[0].Outcome)
	require.NotNil(t, facts[0].LeadTimeMs)
	assert.Equal(t, 2*time.Hour.Milliseconds(), *facts[0].LeadTimeMs)
}

func TestUpsertDeploymentFactFailureIsSticky(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ready := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()

	failed := testFact("rel-1", ready)
	failed.Outcome = OutcomeFailed
	failed.FailedBy = FailedByRollout
	failed.FailureReason = "CrashLoopBackOff"
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{failed}))

	// A later (out-of-order) success event must not overwrite the failure.
	success := testFact("rel-1", ready)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{success}))

	facts, _, err := store.QueryDeploymentFacts(ctx, FactQuery{
		OrgNamespace: "default",
		StartMs:      ready - 1000,
		EndMs:        ready + 1000,
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, OutcomeFailed, facts[0].Outcome)
	assert.Equal(t, FailedByRollout, facts[0].FailedBy)
	assert.Equal(t, "CrashLoopBackOff", facts[0].FailureReason)
}

func TestUpsertDeploymentFactMergesPhases(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	started := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	ready := started + time.Minute.Milliseconds()

	// Phase 1: DeploymentStarted — only start time known.
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{{
		ReleaseUID:     "rel-1",
		OrgNamespace:   "default",
		ProjectUID:     "proj-1",
		ComponentUID:   "comp-1",
		EnvironmentUID: "env-prod",
		StartedMs:      &started,
		Outcome:        OutcomeInProgress,
		UpdatedAtMs:    started,
	}}))

	// Phase 2: DeploymentSucceeded — ready time arrives; started must be preserved.
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{{
		ReleaseUID:     "rel-1",
		OrgNamespace:   "default",
		ProjectUID:     "proj-1",
		ComponentUID:   "comp-1",
		EnvironmentUID: "env-prod",
		ReadyMs:        &ready,
		Outcome:        OutcomeSuccess,
		UpdatedAtMs:    ready,
	}}))

	facts, _, err := store.QueryDeploymentFacts(ctx, FactQuery{
		OrgNamespace: "default",
		StartMs:      started - 1000,
		EndMs:        ready + 1000,
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, OutcomeSuccess, facts[0].Outcome)
	require.NotNil(t, facts[0].StartedMs)
	assert.Equal(t, started, *facts[0].StartedMs)
	require.NotNil(t, facts[0].ReadyMs)
	assert.Equal(t, ready, *facts[0].ReadyMs)
}

func TestQueryDeploymentFactsScopeFilters(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	prod := testFact("rel-prod", base+1000)
	dev := testFact("rel-dev", base+2000)
	dev.EnvironmentUID = "env-dev"
	dev.EnvironmentName = "dev"
	otherComponent := testFact("rel-other", base+3000)
	otherComponent.ComponentUID = "comp-2"
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{prod, dev, otherComponent}))

	all, total, err := store.QueryDeploymentFacts(ctx, FactQuery{
		OrgNamespace: "default", StartMs: base, EndMs: base + 10_000,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, all, 3)

	prodOnly, _, err := store.QueryDeploymentFacts(ctx, FactQuery{
		ComponentUID: "comp-1", EnvironmentUID: "env-prod",
		StartMs: base, EndMs: base + 10_000,
	})
	require.NoError(t, err)
	require.Len(t, prodOnly, 1)
	assert.Equal(t, "rel-prod", prodOnly[0].ReleaseUID)

	// Default sort order is DESC on the deployment moment.
	assert.Equal(t, "rel-other", all[0].ReleaseUID)
	asc, _, err := store.QueryDeploymentFacts(ctx, FactQuery{
		OrgNamespace: "default", StartMs: base, EndMs: base + 10_000, SortOrder: "asc",
	})
	require.NoError(t, err)
	assert.Equal(t, "rel-prod", asc[0].ReleaseUID)
}

func TestQueryLeadTimesExcludesMissingAndNegative(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	withLead := testFact("rel-1", base+1000)
	noProvenance := testFact("rel-2", base+2000)
	noProvenance.CommitSHA = ""
	noProvenance.CommitAuthoredMs = nil
	noProvenance.LeadTimeMs = nil
	negative := testFact("rel-3", base+3000)
	negative.LeadTimeMs = msPtr(-5000)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []DeploymentFact{withLead, noProvenance, negative}))

	leadTimes, err := store.QueryLeadTimes(ctx, FactQuery{
		OrgNamespace: "default", StartMs: base, EndMs: base + 10_000,
	})
	require.NoError(t, err)
	require.Len(t, leadTimes, 1, "missing and negative lead times must be excluded")
	assert.Equal(t, 2*time.Hour.Milliseconds(), leadTimes[0])
}

func TestRecoveryFactsAndDurations(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	failureStart := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	recovered := failureStart + 30*time.Minute.Milliseconds()

	closed := RecoveryFact{
		ID:               "inc-1",
		OrgNamespace:     "default",
		ProjectUID:       "proj-1",
		ComponentUID:     "comp-1",
		EnvironmentUID:   "env-prod",
		IncidentID:       "inc-1",
		Severity:         "critical",
		Source:           RecoverySourceIncident,
		FailureStartedMs: failureStart,
		RecoveredMs:      &recovered,
		// DurationMs deliberately nil: the store derives it from recovered - started.
	}
	open := RecoveryFact{
		ID:               "inc-2",
		OrgNamespace:     "default",
		ComponentUID:     "comp-1",
		EnvironmentUID:   "env-prod",
		Source:           RecoverySourceHealth,
		FailureStartedMs: failureStart + 1000,
	}
	require.NoError(t, store.UpsertRecoveryFacts(ctx, []RecoveryFact{closed, open}))

	durations, err := store.QueryRecoveryDurations(ctx, FactQuery{
		OrgNamespace: "default",
		StartMs:      failureStart - 1000,
		EndMs:        failureStart + time.Hour.Milliseconds(),
	})
	require.NoError(t, err)
	require.Len(t, durations, 1, "open failures must be excluded from MTTR")
	assert.Equal(t, 30*time.Minute.Milliseconds(), durations[0])
}

func TestRollupUpsertAndQuery(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	day1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	day2 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC).UnixMilli()

	rollup := MetricRollup{
		ScopeType:     ScopeTypeComponent,
		ScopeUID:      "comp-1",
		Granularity:   GranularityDaily,
		BucketStartMs: day1,
		DeployTotal:   5,
		DeploySuccess: 4,
		DeployFailed:  1,
		LeadTimeP50Ms: msPtr(3_600_000),
		ComputedAtMs:  day2,
	}
	require.NoError(t, store.UpsertRollups(ctx, []MetricRollup{rollup}))

	// Recompute replaces the bucket in place.
	rollup.DeployTotal = 6
	rollup.DeploySuccess = 5
	require.NoError(t, store.UpsertRollups(ctx, []MetricRollup{rollup}))

	got, err := store.QueryRollups(ctx, RollupQuery{
		ScopeType:   ScopeTypeComponent,
		ScopeUID:    "comp-1",
		Granularity: GranularityDaily,
		StartMs:     day1,
		EndMs:       day2,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 6, got[0].DeployTotal)
	assert.Equal(t, 5, got[0].DeploySuccess)
	require.NotNil(t, got[0].LeadTimeP50Ms)
	assert.Equal(t, int64(3_600_000), *got[0].LeadTimeP50Ms)
	assert.Nil(t, got[0].MTTRMeanMs)

	// The environment-sliced row is distinct from the unsliced row.
	sliced := rollup
	sliced.EnvironmentUID = "env-prod"
	sliced.DeployTotal = 2
	require.NoError(t, store.UpsertRollups(ctx, []MetricRollup{sliced}))

	unsliced, err := store.QueryRollups(ctx, RollupQuery{
		ScopeType: ScopeTypeComponent, ScopeUID: "comp-1",
		Granularity: GranularityDaily, StartMs: day1, EndMs: day2,
	})
	require.NoError(t, err)
	require.Len(t, unsliced, 1)
	assert.Equal(t, 6, unsliced[0].DeployTotal)

	prodSlice, err := store.QueryRollups(ctx, RollupQuery{
		ScopeType: ScopeTypeComponent, ScopeUID: "comp-1", EnvironmentUID: "env-prod",
		Granularity: GranularityDaily, StartMs: day1, EndMs: day2,
	})
	require.NoError(t, err)
	require.Len(t, prodSlice, 1)
	assert.Equal(t, 2, prodSlice[0].DeployTotal)
}

func TestWatermark(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	wm, err := store.Watermark(ctx, "events")
	require.NoError(t, err)
	assert.Equal(t, int64(0), wm, "missing watermark must read as zero")

	require.NoError(t, store.SetWatermark(ctx, "events", 1_000))
	require.NoError(t, store.SetWatermark(ctx, "events", 2_000))
	require.NoError(t, store.SetWatermark(ctx, "incidents", 500))

	wm, err = store.Watermark(ctx, "events")
	require.NoError(t, err)
	assert.Equal(t, int64(2_000), wm)

	wm, err = store.Watermark(ctx, "incidents")
	require.NoError(t, err)
	assert.Equal(t, int64(500), wm)
}

func TestValidationErrors(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	err := store.UpsertDeploymentFacts(ctx, []DeploymentFact{{OrgNamespace: "default"}})
	require.Error(t, err, "missing release UID must be rejected")

	err = store.UpsertDeploymentFacts(ctx, []DeploymentFact{{
		ReleaseUID: "rel-1", OrgNamespace: "default", Outcome: "unknown",
	}})
	require.Error(t, err, "unsupported outcome must be rejected")

	err = store.UpsertRecoveryFacts(ctx, []RecoveryFact{{
		ID: "r-1", OrgNamespace: "default", Source: "guess", FailureStartedMs: 1,
	}})
	require.Error(t, err, "unsupported recovery source must be rejected")

	err = store.UpsertRollups(ctx, []MetricRollup{{
		ScopeType: "cluster", ScopeUID: "x", Granularity: GranularityDaily,
	}})
	require.Error(t, err, "unsupported scope type must be rejected")

	_, err = store.QueryRollups(ctx, RollupQuery{
		ScopeType: ScopeTypeOrg, ScopeUID: "default", Granularity: "hourly",
		StartMs: 0, EndMs: 1,
	})
	require.Error(t, err, "unsupported granularity must be rejected")

	_, _, err = store.QueryDeploymentFacts(ctx, FactQuery{SortOrder: "sideways"})
	require.Error(t, err, "invalid sort order must be rejected")
}

func TestBucketStartMs(t *testing.T) {
	t.Parallel()

	// Wednesday 2026-07-08 15:30 UTC.
	instant := time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC).UnixMilli()

	daily := time.UnixMilli(BucketStartMs(GranularityDaily, instant)).UTC()
	assert.Equal(t, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), daily)

	weekly := time.UnixMilli(BucketStartMs(GranularityWeekly, instant)).UTC()
	assert.Equal(t, time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC), weekly, "week starts Monday")
	assert.Equal(t, time.Monday, weekly.Weekday())

	monthly := time.UnixMilli(BucketStartMs(GranularityMonthly, instant)).UTC()
	assert.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), monthly)

	// A Sunday belongs to the week of the previous Monday.
	sunday := time.Date(2026, 7, 5, 23, 59, 0, 0, time.UTC).UnixMilli()
	weekOfSunday := time.UnixMilli(BucketStartMs(GranularityWeekly, sunday)).UTC()
	assert.Equal(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC), weekOfSunday)
}

func TestPercentileAndMean(t *testing.T) {
	t.Parallel()

	assert.Nil(t, Percentile(nil, 0.5))
	assert.Nil(t, Mean(nil))

	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	assert.Equal(t, int64(50), *Percentile(values, 0.50))
	assert.Equal(t, int64(80), *Percentile(values, 0.75))
	assert.Equal(t, int64(100), *Percentile(values, 0.95))
	assert.Equal(t, int64(55), *Mean(values))

	single := []int64{42}
	assert.Equal(t, int64(42), *Percentile(single, 0.5))
	assert.Equal(t, int64(42), *Percentile(single, 0.95))
}

func TestBuildRollups(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	success := testFact("rel-1", day.UnixMilli())
	failed := testFact("rel-2", day.Add(time.Hour).UnixMilli())
	failed.Outcome = OutcomeFailed
	failed.FailedBy = FailedByRollout
	failed.LeadTimeMs = nil
	inProgress := testFact("rel-3", day.Add(2*time.Hour).UnixMilli())
	inProgress.Outcome = OutcomeInProgress

	recovered := day.Add(3 * time.Hour).UnixMilli()
	duration := 45 * time.Minute.Milliseconds()
	recovery := RecoveryFact{
		ID: "inc-1", OrgNamespace: "default", ProjectUID: "proj-1",
		ComponentUID: "comp-1", EnvironmentUID: "env-prod",
		Source:           RecoverySourceIncident,
		FailureStartedMs: day.Add(2 * time.Hour).UnixMilli(),
		RecoveredMs:      &recovered,
		DurationMs:       &duration,
	}

	rollups := BuildRollups(
		[]DeploymentFact{success, failed, inProgress},
		[]RecoveryFact{recovery},
		day.UnixMilli(),
	)

	// 3 scope types × 2 env slices × 3 granularities = 18 buckets (single day/week/month).
	assert.Len(t, rollups, 18)

	var componentDaily *MetricRollup
	for i := range rollups {
		r := &rollups[i]
		if r.ScopeType == ScopeTypeComponent && r.EnvironmentUID == "" &&
			r.Granularity == GranularityDaily {
			componentDaily = r
			break
		}
	}
	require.NotNil(t, componentDaily)
	assert.Equal(t, 2, componentDaily.DeployTotal, "in-progress facts must not be counted")
	assert.Equal(t, 1, componentDaily.DeploySuccess)
	assert.Equal(t, 1, componentDaily.DeployFailed)
	require.NotNil(t, componentDaily.LeadTimeP50Ms)
	assert.Equal(t, 2*time.Hour.Milliseconds(), *componentDaily.LeadTimeP50Ms)
	assert.Equal(t, 1, componentDaily.RecoveryCount)
	require.NotNil(t, componentDaily.MTTRMeanMs)
	assert.Equal(t, duration, *componentDaily.MTTRMeanMs)
	assert.Equal(t, BucketStartMs(GranularityDaily, day.UnixMilli()), componentDaily.BucketStartMs)
}
