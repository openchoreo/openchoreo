// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

func newTestStores(t *testing.T) (deliveryinsights.Store, incidententry.IncidentEntryStore) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))

	store, err := deliveryinsights.New(deliveryinsights.BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.Initialize(context.Background()))

	incidents, err := incidententry.New("sqlite", dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, incidents.Close()) })
	require.NoError(t, incidents.Initialize(context.Background()))

	return store, incidents
}

func newTestAggregator(
	store deliveryinsights.Store,
	incidents incidententry.IncidentEntryStore,
	events EventsSource,
	now time.Time,
) *Aggregator {
	a := New(store, incidents, events, Config{
		Interval:          5 * time.Minute,
		Overlap:           10 * time.Minute,
		AttributionWindow: 24 * time.Hour,
		IncidentLookback:  30 * 24 * time.Hour,
	}, slog.Default())
	a.now = func() time.Time { return now }
	return a
}

func successFact(releaseUID string, readyMs int64) deliveryinsights.DeploymentFact {
	ready := readyMs
	return deliveryinsights.DeploymentFact{
		ReleaseUID:     releaseUID,
		OrgNamespace:   "default",
		ProjectUID:     "checkout",
		ComponentUID:   "checkout-api",
		EnvironmentUID: "production",
		ReadyMs:        &ready,
		Outcome:        deliveryinsights.OutcomeSuccess,
		UpdatedAtMs:    readyMs,
	}
}

func TestRunOnceProcessesIncidentsEndToEnd(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	deployedAt := now.Add(-2 * time.Hour)

	require.NoError(t, store.UpsertDeploymentFacts(ctx,
		[]deliveryinsights.DeploymentFact{successFact("rel-1", deployedAt.UnixMilli())}))

	triggered := deployedAt.Add(30 * time.Minute)
	resolved := triggered.Add(45 * time.Minute)
	_, err := incidents.WriteIncidentEntry(ctx, &incidententry.IncidentEntry{
		AlertID:         "alert-1",
		Timestamp:       triggered.Format(time.RFC3339Nano), // ingestion time inside the mocked window
		Status:          incidententry.StatusResolved,
		TriggeredAt:     triggered.Format(time.RFC3339Nano),
		ResolvedAt:      resolved.Format(time.RFC3339Nano),
		NamespaceName:   "default",
		ProjectName:     "checkout",
		ComponentName:   "checkout-api",
		EnvironmentName: "production",
		ProjectID:       "checkout",
		ComponentID:     "checkout-api",
		EnvironmentID:   "production",
	})
	require.NoError(t, err)

	agg := newTestAggregator(store, incidents, nil, now)
	require.NoError(t, agg.RunOnce(ctx))

	// The deployment is now failed-by-incident.
	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      deployedAt.Add(-time.Hour).UnixMilli(),
		EndMs:        now.UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, deliveryinsights.OutcomeFailed, facts[0].Outcome)
	assert.Equal(t, deliveryinsights.FailedByIncident, facts[0].FailedBy)

	// An incident-sourced recovery fact exists with the resolved duration.
	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      deployedAt.UnixMilli(),
		EndMs:        now.UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 1)
	assert.Equal(t, deliveryinsights.RecoverySourceIncident, recoveries[0].Source)
	require.NotNil(t, recoveries[0].DurationMs)
	assert.Equal(t, 45*time.Minute.Milliseconds(), *recoveries[0].DurationMs)

	// Rollups were recomputed: the daily bucket shows 1 deployment, 1 failed.
	rollups, err := store.QueryRollups(ctx, deliveryinsights.RollupQuery{
		ScopeType:   deliveryinsights.ScopeTypeComponent,
		ScopeUID:    "checkout-api",
		Granularity: deliveryinsights.GranularityDaily,
		StartMs:     deliveryinsights.BucketStartMs(deliveryinsights.GranularityDaily, deployedAt.UnixMilli()),
		EndMs:       now.UnixMilli() + 1,
	})
	require.NoError(t, err)
	require.Len(t, rollups, 1)
	assert.Equal(t, 1, rollups[0].DeployTotal)
	assert.Equal(t, 1, rollups[0].DeployFailed)
	assert.Equal(t, 1, rollups[0].RecoveryCount)

	// Watermark advanced to the tick start.
	wm, err := store.Watermark(ctx, watermarkSourceIncidents)
	require.NoError(t, err)
	assert.Equal(t, now.UnixMilli(), wm)

	// A second tick over the same data changes nothing (idempotency).
	agg2 := newTestAggregator(store, incidents, nil, now.Add(5*time.Minute))
	require.NoError(t, agg2.RunOnce(ctx))
	rollups2, err := store.QueryRollups(ctx, deliveryinsights.RollupQuery{
		ScopeType:   deliveryinsights.ScopeTypeComponent,
		ScopeUID:    "checkout-api",
		Granularity: deliveryinsights.GranularityDaily,
		StartMs:     deliveryinsights.BucketStartMs(deliveryinsights.GranularityDaily, deployedAt.UnixMilli()),
		EndMs:       now.UnixMilli() + 1,
	})
	require.NoError(t, err)
	require.Len(t, rollups2, 1)
	assert.Equal(t, 1, rollups2[0].DeployTotal, "re-processing must not double count")
}

func TestRunOnceResolvesIncidentOnLaterTick(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	triggered := now.Add(-time.Hour)

	incidentID, err := incidents.WriteIncidentEntry(ctx, &incidententry.IncidentEntry{
		AlertID:       "alert-1",
		Timestamp:     triggered.Format(time.RFC3339Nano),
		Status:        incidententry.StatusActive,
		TriggeredAt:   triggered.Format(time.RFC3339Nano),
		NamespaceName: "default",
		ComponentID:   "checkout-api",
		EnvironmentID: "production",
	})
	require.NoError(t, err)

	agg := newTestAggregator(store, incidents, nil, now)
	require.NoError(t, agg.RunOnce(ctx))

	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		StartMs: triggered.Add(-time.Minute).UnixMilli(), EndMs: now.UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 1)
	assert.Nil(t, recoveries[0].RecoveredMs, "active incident must be an open episode")

	// Human resolves the incident well after its ingestion timestamp. Resolving
	// does not bump timestamp_ns, which is exactly why incidents are re-scanned
	// over a rolling lookback window instead of watermark-incrementally — the
	// resolution must land no matter when it happens.
	resolvedAt := now.Add(2 * time.Minute)
	_, err = incidents.UpdateIncidentEntry(ctx, incidentID,
		incidententry.StatusResolved, nil, nil, resolvedAt)
	require.NoError(t, err)

	// Second tick: the lookback rescan picks the resolution up and closes the episode.
	agg2 := newTestAggregator(store, incidents, nil, now.Add(5*time.Minute))
	require.NoError(t, agg2.RunOnce(ctx))

	recoveries, err = store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		StartMs: triggered.Add(-time.Minute).UnixMilli(), EndMs: now.Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 1)
	require.NotNil(t, recoveries[0].RecoveredMs, "resolution within overlap must close the episode")
}

type fakeEventsSource struct {
	events []DeliveryEvent
	// pageCap simulates the adapter's page cap: when >0 the sweep returns at most
	// this many events and reports itself incomplete.
	pageCap int
}

func (f *fakeEventsSource) FetchDeliveryEvents(
	_ context.Context, fromMs, toMs int64,
) ([]DeliveryEvent, bool, error) {
	var out []DeliveryEvent
	for _, e := range f.events {
		if e.TimestampMs >= fromMs && e.TimestampMs < toMs {
			out = append(out, e)
		}
	}
	if f.pageCap > 0 && len(out) > f.pageCap {
		return out[:f.pageCap], false, nil
	}
	return out, true, nil
}

func deliveryEvent(reason, releaseUID string, ts time.Time, extra map[string]string) DeliveryEvent {
	payload := map[string]string{
		"renderedReleaseUid":   releaseUID,
		"componentReleaseName": "checkout-api-7",
		"projectUid":           "checkout",
		"componentUid":         "checkout-api",
		"environmentUid":       "production",
	}
	for k, v := range extra {
		payload[k] = v
	}
	raw, _ := json.Marshal(payload)
	return DeliveryEvent{
		Reason:          reason,
		TimestampMs:     ts.UnixMilli(),
		Namespace:       "default",
		ProjectName:     "checkout",
		ComponentName:   "checkout-api",
		EnvironmentName: "production",
		Message:         string(raw),
	}
}

// episodeReleaseUID is the rollout the episode fixtures below share.
const episodeReleaseUID = "rel-ep"

// deliveryEventEpisode is deliveryEvent with a numeric failureEpisode. The payload
// field is an int32, so it cannot come through the string-valued extras map.
func deliveryEventEpisode(
	reason string, ts time.Time, episode int32, extra map[string]string,
) DeliveryEvent {
	e := deliveryEvent(reason, episodeReleaseUID, ts, extra)
	var payload map[string]any
	if err := json.Unmarshal([]byte(e.Message), &payload); err != nil {
		panic(err)
	}
	payload["failureEpisode"] = episode
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	e.Message = string(raw)
	return e
}

func TestRunOnceFoldsDeliveryEvents(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	started := now.Add(-30 * time.Minute)
	ready := started.Add(90 * time.Second)
	authored := started.Add(-4 * time.Hour)

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentStarted, "rel-1", started, nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-1", ready, map[string]string{
			"commit":           "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			"commitAuthoredAt": authored.Format(time.RFC3339Nano),
		}),
		// A second release fails and later recovers (health episode).
		deliveryEvent(ReasonDeploymentFailed, "rel-2", started.Add(5*time.Minute), map[string]string{
			"failureReason": "CrashLoopBackOff",
		}),
		deliveryEvent(ReasonDeploymentRecovered, "rel-2", started.Add(25*time.Minute), nil),
	}}

	agg := newTestAggregator(store, incidents, source, now)
	require.NoError(t, agg.RunOnce(ctx))

	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      started.Add(-time.Hour).UnixMilli(),
		EndMs:        now.UnixMilli(),
		SortOrder:    "ASC",
	})
	require.NoError(t, err)
	require.Len(t, facts, 2)

	// rel-1: Started + Succeeded merged into one success fact with lead time.
	assert.Equal(t, "rel-1", facts[0].ReleaseUID)
	assert.Equal(t, deliveryinsights.OutcomeSuccess, facts[0].Outcome)
	require.NotNil(t, facts[0].StartedMs)
	require.NotNil(t, facts[0].ReadyMs)
	require.NotNil(t, facts[0].LeadTimeMs)
	assert.Equal(t, ready.Sub(authored).Milliseconds(), *facts[0].LeadTimeMs)

	// rel-2: failed by rollout, and its health recovery episode is closed.
	assert.Equal(t, "rel-2", facts[1].ReleaseUID)
	assert.Equal(t, deliveryinsights.OutcomeFailed, facts[1].Outcome)
	assert.Equal(t, deliveryinsights.FailedByRollout, facts[1].FailedBy)
	assert.Equal(t, "CrashLoopBackOff", facts[1].FailureReason)

	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      started.UnixMilli(),
		EndMs:        now.UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 1)
	assert.Equal(t, deliveryinsights.RecoverySourceHealth, recoveries[0].Source)
	require.NotNil(t, recoveries[0].DurationMs)
	assert.Equal(t, 20*time.Minute.Milliseconds(), *recoveries[0].DurationMs)

	// Rollups reflect both facts.
	rollups, err := store.QueryRollups(ctx, deliveryinsights.RollupQuery{
		ScopeType:   deliveryinsights.ScopeTypeComponent,
		ScopeUID:    "checkout-api",
		Granularity: deliveryinsights.GranularityDaily,
		StartMs:     deliveryinsights.BucketStartMs(deliveryinsights.GranularityDaily, started.UnixMilli()),
		EndMs:       now.UnixMilli() + 1,
	})
	require.NoError(t, err)
	require.Len(t, rollups, 1)
	assert.Equal(t, 2, rollups[0].DeployTotal)
	assert.Equal(t, 1, rollups[0].DeploySuccess)
	assert.Equal(t, 1, rollups[0].DeployFailed)

	// Events watermark advanced.
	wm, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	assert.Equal(t, now.UnixMilli(), wm)
}

func TestRunOnceSkipsMalformedEvents(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	source := &fakeEventsSource{events: []DeliveryEvent{
		{Reason: ReasonDeploymentSucceeded, TimestampMs: now.Add(-time.Hour).UnixMilli(),
			Namespace: "default", Message: "not json"},
		{Reason: "SomethingElse", TimestampMs: now.Add(-time.Hour).UnixMilli(),
			Namespace: "default", Message: `{"renderedReleaseUid":"rel-x"}`},
	}}

	agg := newTestAggregator(store, incidents, source, now)
	require.NoError(t, agg.RunOnce(ctx), "malformed events must not fail the tick")

	_, total, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		StartMs: 0, EndMs: now.UnixMilli(),
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

// The incident lookback window starts from the tick time, not from the watermark, so a
// read that stopped at its row limit would re-read the same first page on every tick and
// never attribute the remainder. Paging is what makes the window drain.
func TestProcessIncidentsPagesThroughTheWindow(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	const incidentCount = 7
	for i := 0; i < incidentCount; i++ {
		triggered := now.Add(-time.Duration(incidentCount-i) * time.Hour)
		_, err := incidents.WriteIncidentEntry(ctx, &incidententry.IncidentEntry{
			AlertID:         fmt.Sprintf("alert-%d", i),
			Timestamp:       triggered.Format(time.RFC3339Nano),
			Status:          incidententry.StatusResolved,
			TriggeredAt:     triggered.Format(time.RFC3339Nano),
			ResolvedAt:      triggered.Add(10 * time.Minute).Format(time.RFC3339Nano),
			NamespaceName:   "default",
			ProjectName:     "checkout",
			ComponentName:   "checkout-api",
			EnvironmentName: "production",
			ProjectID:       "checkout",
			ComponentID:     "checkout-api",
			EnvironmentID:   "production",
		})
		require.NoError(t, err)
	}

	agg := newTestAggregator(store, incidents, nil, now)
	agg.incidentPageSize = 2 // force several pages
	require.NoError(t, agg.RunOnce(ctx))

	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      now.Add(-24 * time.Hour).UnixMilli(),
		EndMs:        now.UnixMilli() + 1,
	})
	require.NoError(t, err)
	assert.Len(t, recoveries, incidentCount,
		"every incident in the window must be folded, not just the first page")
}

// A capped delivery-event sweep must leave the watermark where it actually got to.
// Advancing it to the tick time would narrow the next window past the unread
// remainder, silently dropping those events.
func TestRunOnceHoldsEventsWatermarkBackOnCappedSweep(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	first := now.Add(-3 * time.Hour)
	second := now.Add(-2 * time.Hour)
	third := now.Add(-time.Hour)

	source := &fakeEventsSource{
		events: []DeliveryEvent{
			deliveryEvent(ReasonDeploymentSucceeded, "rel-1", first, nil),
			deliveryEvent(ReasonDeploymentSucceeded, "rel-2", second, nil),
			deliveryEvent(ReasonDeploymentSucceeded, "rel-3", third, nil),
		},
		pageCap: 2,
	}

	agg := newTestAggregator(store, incidents, source, now)
	require.NoError(t, agg.RunOnce(ctx))

	watermark, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	assert.Equal(t, second.UnixMilli(), watermark,
		"watermark must stop at the last swept event, not jump to the tick time")

	// The next tick, now uncapped, must still see the event the cap left behind.
	source.pageCap = 0
	require.NoError(t, agg.RunOnce(ctx))

	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      first.Add(-time.Hour).UnixMilli(),
		EndMs:        now.UnixMilli() + 1,
		SortOrder:    "ASC",
	})
	require.NoError(t, err)
	require.Len(t, facts, 3, "the event skipped by the cap must be picked up next tick")
}

// The livelock case: more capped events than fit in one sweep, all inside the Overlap
// window. Resuming at watermark - Overlap would refill the page cap before reaching the
// previous stop point, so every tick would re-read the same events and the sweep would
// never reach the later ones. Successive ticks must drain the window.
func TestRunOnceCappedSweepInsideOverlapStillDrains(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// Six events packed into one minute, well inside the 10-minute Overlap.
	const eventCount = 6
	events := make([]DeliveryEvent, 0, eventCount)
	for i := 0; i < eventCount; i++ {
		ts := now.Add(-5*time.Minute + time.Duration(i)*10*time.Second)
		events = append(events, deliveryEvent(
			ReasonDeploymentSucceeded, fmt.Sprintf("rel-%d", i), ts, nil))
	}
	source := &fakeEventsSource{events: events, pageCap: 2}

	// Each tick is capped at 2 events, so draining 6 needs several ticks. Bound the
	// loop so a regression fails the test instead of hanging it.
	tick := now
	for i := 0; i < 10; i++ {
		agg := newTestAggregator(store, incidents, source, tick)
		require.NoError(t, agg.RunOnce(ctx))

		facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
			OrgNamespace: "default",
			StartMs:      now.Add(-time.Hour).UnixMilli(),
			EndMs:        tick.UnixMilli() + 1,
		})
		require.NoError(t, err)
		if len(facts) == eventCount {
			return // drained
		}
		tick = tick.Add(5 * time.Minute)
	}

	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		OrgNamespace: "default",
		StartMs:      now.Add(-time.Hour).UnixMilli(),
		EndMs:        tick.UnixMilli() + 1,
	})
	require.NoError(t, err)
	t.Fatalf("capped sweep never drained the overlap window: folded %d of %d events",
		len(facts), eventCount)
}

// TestSuccessiveFailureEpisodesStayDistinct pins that each failure->recovery cycle
// of one rollout is its own MTTR sample.
//
// The recovery-fact ID used to key on the rollout alone, so episode 2's Recovered
// merged into episode 1's row. Because the store deliberately preserves the
// original failure_started_ms on merge, the surviving duration ran from episode 1's
// failure to episode 2's recovery -- spanning the healthy interval between them.
// Two one-hour outages twenty hours apart therefore reported a single ~21h
// recovery instead of two 1h ones.
func TestSuccessiveFailureEpisodesStayDistinct(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	failed1 := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	recovered1 := failed1.Add(time.Hour)
	failed2 := recovered1.Add(20 * time.Hour)
	recovered2 := failed2.Add(time.Hour)

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEventEpisode(ReasonDeploymentFailed, failed1, 1,
			map[string]string{"failureReason": "CrashLoopBackOff"}),
		deliveryEventEpisode(ReasonDeploymentRecovered, recovered1, 1, nil),
		deliveryEventEpisode(ReasonDeploymentFailed, failed2, 2,
			map[string]string{"failureReason": "CrashLoopBackOff"}),
		deliveryEventEpisode(ReasonDeploymentRecovered, recovered2, 2, nil),
	}}

	a := newTestAggregator(store, incidents, source, now)
	require.NoError(t, a.RunOnce(ctx))

	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		StartMs: failed1.Add(-time.Hour).UnixMilli(),
		EndMs:   now.UnixMilli(),
		Limit:   deliveryinsights.MaxQueryLimit,
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 2, "each failure->recovery cycle is its own MTTR sample")

	for _, r := range recoveries {
		require.NotNil(t, r.RecoveredMs, "both episodes must be closed")
		require.NotNil(t, r.DurationMs)
		require.Equal(t, time.Hour.Milliseconds(), *r.DurationMs,
			"duration must cover the outage only, not the healthy interval between episodes")
	}
}

// TestRecomputeKeepsWeeksStraddlingAMonthBoundaryWhole pins that a weekly bucket
// beginning in the previous month is not rebuilt from a partial fact set.
//
// recomputeRollups used to snap the fact read to the *monthly* boundary of the
// earliest touched moment, but BuildRollups emits daily, weekly and monthly
// buckets. 2026-09-01 is a Tuesday, so its week starts Mon 2026-08-31: a tick
// touching Sep 1 read only September's facts and then replaced the Aug-31 weekly
// bucket -- UpsertRollups replaces, never increments -- with a count covering
// September alone. This fires on essentially every tick during the first days of
// any month whose 1st is not a Monday, and the wrong value persists.
func TestRecomputeKeepsWeeksStraddlingAMonthBoundaryWhole(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()

	aug31 := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC) // Monday, week start
	sep01 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)  // Tuesday, same week
	weekStartMs := deliveryinsights.BucketStartMs(deliveryinsights.GranularityWeekly, sep01.UnixMilli())
	require.Equal(t,
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC).UnixMilli(), weekStartMs,
		"fixture assumes the week containing Sep 1 starts Aug 31")

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentSucceeded, "rel-aug31", aug31, nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-sep01", sep01, nil),
	}}

	// First tick folds both deployments and builds the week correctly.
	require.NoError(t, newTestAggregator(store, incidents, source,
		sep01.Add(time.Hour)).RunOnce(ctx))
	weekly := func() deliveryinsights.MetricRollup {
		got, err := store.QueryRollups(ctx, deliveryinsights.RollupQuery{
			ScopeType:   deliveryinsights.ScopeTypeComponent,
			ScopeUID:    "checkout-api",
			Granularity: deliveryinsights.GranularityWeekly,
			StartMs:     weekStartMs,
			EndMs:       weekStartMs + 1,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		return got[0]
	}
	require.Equal(t, 2, weekly().DeployTotal, "both deployments fall in the Aug-31 week")

	// A later tick that touches only September must not shrink that week. The
	// September-only event is new, so the recompute is driven by a Sep 1 moment.
	source.events = append(source.events,
		deliveryEvent(ReasonDeploymentSucceeded, "rel-sep02",
			time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC), nil))
	require.NoError(t, newTestAggregator(store, incidents, source,
		time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)).RunOnce(ctx))

	require.Equal(t, 3, weekly().DeployTotal,
		"the Aug-31 week must still count its August deployment after a September tick")
}

// TestOneUnattributableEventDoesNotWedgeTheTick pins that a single event with no
// org namespace cannot stall ingestion.
//
// UpsertDeploymentFacts validates the whole slice before writing any of it and
// returns on the first error, so one such event used to write none of the batch,
// fail the tick, and leave the watermark unmoved -- so the same bad event was
// re-read on every tick, forever, and no delivery data landed at all.
func TestOneUnattributableEventDoesNotWedgeTheTick(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	good := now.Add(-20 * time.Minute)
	bad := now.Add(-15 * time.Minute)

	unenriched := deliveryEvent(ReasonDeploymentSucceeded, "rel-bad", bad, nil)
	unenriched.Namespace = "" // collector enrichment missing
	// Strip the payload's namespace too, so neither source can supply it.
	unenriched.Message = strings.ReplaceAll(unenriched.Message, `"orgNamespace":"default",`, "")

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentSucceeded, "rel-good", good, nil),
		unenriched,
	}}

	a := newTestAggregator(store, incidents, source, now)
	require.NoError(t, a.RunOnce(ctx), "one unattributable event must not fail the tick")

	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		StartMs: good.Add(-time.Hour).UnixMilli(),
		EndMs:   now.UnixMilli(),
		Limit:   deliveryinsights.MaxQueryLimit,
	})
	require.NoError(t, err)
	require.Len(t, facts, 1, "the good event must still be written")
	require.Equal(t, "rel-good", facts[0].ReleaseUID)

	// The watermark must have advanced, or the bad event is re-read forever.
	wm, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	require.Equal(t, now.UnixMilli(), wm, "watermark must advance past the skipped event")
}

// TestIncompleteSweepWithNoEventsHoldsPosition covers the ordering that keeps an
// incomplete sweep from advancing past a remainder it never read.
func TestIncompleteSweepWithNoEventsHoldsPosition(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// Reports itself incomplete while returning nothing.
	source := &emptyIncompleteSource{}
	require.NoError(t, newTestAggregator(store, incidents, source, now).RunOnce(ctx))

	wm, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	require.NotEqual(t, now.UnixMilli(), wm,
		"an incomplete sweep with no events must not advance the watermark to tickStart")
}

type emptyIncompleteSource struct{}

func (e *emptyIncompleteSource) FetchDeliveryEvents(
	_ context.Context, _, _ int64,
) ([]DeliveryEvent, bool, error) {
	return nil, false, nil
}
