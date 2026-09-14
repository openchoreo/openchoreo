// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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
		"rolloutId":            releaseUID,
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
			Namespace: "default", Message: `{"rolloutId":"rel-x"}`},
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
		All:     true,
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
	// deliveryEvent's payload carries no namespace key, so clearing the enrichment
	// above leaves neither source able to supply one -- which is the case under
	// test. Asserted rather than assumed: this previously stripped a key by string
	// match, and the helper had stopped emitting it, so the replacement was a no-op
	// and the test passed while relying on something it did not check.
	require.NotContains(t, unenriched.Message, "namespaceName",
		"the payload must carry no namespace for this event to be unattributable")

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentSucceeded, "rel-good", good, nil),
		unenriched,
	}}

	a := newTestAggregator(store, incidents, source, now)
	require.NoError(t, a.RunOnce(ctx), "one unattributable event must not fail the tick")

	facts, _, err := store.QueryDeploymentFacts(ctx, deliveryinsights.FactQuery{
		StartMs: good.Add(-time.Hour).UnixMilli(),
		EndMs:   now.UnixMilli(),
		All:     true,
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

// TestProcessIncidentsPagesOnIngestionTimeNotTriggerTime pins that the incident
// cursor advances on the column the query pages by.
//
// QueryIncidentEntries filters and orders on timestamp_ns, the ingestion time.
// Advancing the cursor by the page's newest TriggeredAt instead moves it past
// entries whose ingestion time sorts between the two the moment those fields
// diverge, and the incident watermark then advances over them, so they are never
// folded: their recoveries never reach MTTR and their attribution never lands.
//
// Every entry the alert path writes today sets both fields from one value, so
// this is a property of the paging logic rather than a live defect -- which is
// exactly why it needs pinning: nothing in this package would notice if the two
// columns started to differ.
func TestProcessIncidentsPagesOnIngestionTimeNotTriggerTime(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// Ingested a minute apart, but each alert claims it fired an hour later than
	// the entry ingested after it. Paging on TriggeredAt jumps the cursor past
	// the entries in between.
	const incidentCount = 6
	for i := 0; i < incidentCount; i++ {
		ingested := now.Add(-time.Duration(incidentCount-i) * time.Minute)
		triggered := ingested.Add(time.Hour)
		_, err := incidents.WriteIncidentEntry(ctx, &incidententry.IncidentEntry{
			AlertID:         fmt.Sprintf("skew-alert-%d", i),
			Timestamp:       ingested.Format(time.RFC3339Nano),
			Status:          incidententry.StatusResolved,
			TriggeredAt:     triggered.Format(time.RFC3339Nano),
			ResolvedAt:      triggered.Add(5 * time.Minute).Format(time.RFC3339Nano),
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
		EndMs:        now.Add(24 * time.Hour).UnixMilli(),
	})
	require.NoError(t, err)
	assert.Len(t, recoveries, incidentCount,
		"paging must not skip entries whose ingestion time sorts before a later trigger time")
}

// TestOnlyTheLeaseHolderAggregates pins that scaling the Observer does not mean
// scaling the sweeps.
//
// Two aggregators share one store, as two replicas of a scaled Observer do. The
// one without the lease must do nothing at all, because concurrent sweeps share
// watermarks and would overwrite each other's resume positions.
//
// The follower runs on a later clock so that a tick it should not have run shows
// up as a moved watermark. Nothing probes the lease mid-test: AcquireLease is the
// only way to read it and it renews on success, so probing would rewrite the
// expiry the test depends on.
func TestOnlyTheLeaseHolderAggregates(t *testing.T) {
	ctx := context.Background()
	store, incidents := newTestStores(t)
	leaderNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	followerNow := leaderNow.Add(5 * time.Minute) // well inside the leader's TTL

	leader := newTestAggregator(store, incidents, nil, leaderNow)
	follower := newTestAggregator(store, incidents, nil, followerNow)
	require.NotEqual(t, leader.holder, follower.holder,
		"two replicas must not share a holder id, or each could renew the other's lease")

	leader.tick(ctx)
	require.Equal(t, leaderNow.UnixMilli(), mustWatermark(t, store),
		"the lease holder should have aggregated")

	follower.tick(ctx)
	require.Equal(t, leaderNow.UnixMilli(), mustWatermark(t, store),
		"a replica without the lease must not aggregate, and must leave the watermark alone")
}

func mustWatermark(t *testing.T, store deliveryinsights.Store) int64 {
	t.Helper()
	wm, err := store.Watermark(context.Background(), watermarkSourceIncidents)
	require.NoError(t, err)
	return wm
}

// TestLeaseHandsOverWhenTheHolderStops pins that a graceful shutdown does not
// pause aggregation for a whole TTL.
func TestLeaseHandsOverWhenTheHolderStops(t *testing.T) {
	ctx := context.Background()
	store, incidents := newTestStores(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	leader := newTestAggregator(store, incidents, nil, now)
	successor := newTestAggregator(store, incidents, nil, now)

	leader.tick(ctx)
	successor.tick(ctx) // refused while the leader holds it

	leader.releaseLease()

	// Same instant, so nothing has expired: the successor can only get in because
	// the lease was handed back.
	successor.tick(ctx)
	held, err := store.AcquireLease(ctx, aggregationLease, successor.holder, now.UnixMilli(), 1)
	require.NoError(t, err)
	require.True(t, held, "a released lease must be takeable before its TTL elapses")
}

// TestLeaseStoreFailureSkipsTheTick pins the safe direction of the failure.
// If the lease cannot be read, the tick must be skipped rather than run: a
// delayed tick costs latency, whereas two replicas sweeping at once corrupts
// the watermarks.
func TestLeaseStoreFailureSkipsTheTick(t *testing.T) {
	ctx := context.Background()
	store, incidents := newTestStores(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	agg := newTestAggregator(store, incidents, nil, now)
	agg.store = leaseErrorStore{Store: store}

	agg.tick(ctx)

	wm, err := store.Watermark(ctx, watermarkSourceIncidents)
	require.NoError(t, err)
	require.Zero(t, wm, "a tick must not proceed when the lease cannot be acquired")
}

// leaseErrorStore fails only lease acquisition, leaving every other call intact,
// so the test isolates the lease failure from a broken store.
//
// It returns true alongside the error deliberately. A store that merely returned
// (false, err) would be skipped by the not-held branch, and the test would pass
// whether or not the error was handled at all. Returning the contradictory pair
// forces the question the invariant is really about: on an error the tick must be
// skipped regardless of what the boolean claims.
type leaseErrorStore struct {
	deliveryinsights.Store
}

func (s leaseErrorStore) AcquireLease(
	context.Context, string, string, int64, int64,
) (bool, error) {
	return true, errors.New("lease store unavailable")
}

// handoverStore grants the lease once and refuses every renewal after it, which
// is what a replica sees when its lease expired mid-tick and another replica took
// over. grants counts acquisitions so a test can tell renewal from acquisition.
type handoverStore struct {
	deliveryinsights.Store
	mu     sync.Mutex
	grants int
}

func (s *handoverStore) AcquireLease(
	context.Context, string, string, int64, int64,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants++
	return s.grants == 1, nil
}

func (s *handoverStore) grantCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grants
}

// TestTickIsAbandonedWhenTheLeaseIsLost pins that holding the lease at the start
// of a tick is not taken as holding it for the whole tick.
//
// A sweep can outlast the TTL — the first tick after enabling reads a 30-day
// incident window plus whatever event backlog exists — and an expired lease is
// takeable. Without renewal the original replica keeps writing facts and
// watermarks while its successor does the same, which is the corruption the lease
// exists to prevent.
func TestTickIsAbandonedWhenTheLeaseIsLost(t *testing.T) {
	store, incidents := newTestStores(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	agg := newTestAggregator(store, incidents, nil, now)
	handover := &handoverStore{Store: store}
	agg.store = handover
	agg.renewInterval = 2 * time.Millisecond

	// Stand in for a long RunOnce: hold the tick context open and watch for it
	// being cancelled out from under us.
	tickCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	renewed := agg.holdLease(tickCtx, cancel)

	select {
	case <-tickCtx.Done():
		// The renewer noticed the lease was gone and stopped the tick.
	case <-time.After(2 * time.Second):
		t.Fatal("a tick that lost its lease must be cancelled, not left running")
	}
	<-renewed
	require.Greater(t, handover.grantCount(), 1,
		"the lease must be renewed during the tick, not only acquired before it")
}

// TestTickKeepsRunningWhileTheLeaseHolds is the other half: renewal must not
// cancel a tick that still legitimately owns the lease.
func TestTickKeepsRunningWhileTheLeaseHolds(t *testing.T) {
	store, incidents := newTestStores(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	agg := newTestAggregator(store, incidents, nil, now)
	agg.renewInterval = 2 * time.Millisecond

	tickCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	renewed := agg.holdLease(tickCtx, cancel)

	select {
	case <-tickCtx.Done():
		t.Fatal("a tick still holding its lease must not be cancelled")
	case <-time.After(50 * time.Millisecond): // many renewal intervals
	}

	cancel()
	select {
	case <-renewed:
	case <-time.After(2 * time.Second):
		t.Fatal("the renewer must stop when the tick ends")
	}
}

// captureLogs collects slog records at info level so a test can assert on what an
// operator would actually see.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// messagesAtLeast returns the messages logged at or above level.
func (h *captureHandler) messagesAtLeast(level slog.Level) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.records {
		if r.Level >= level {
			out = append(out, r.Message)
		}
	}
	return out
}

func containsMessage(msgs []string, substr string) bool {
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// TestAnIdleTickStillReportsItself pins that an aggregator with nothing to do is
// distinguishable from one that is stuck.
//
// The completion line used to be emitted only when a tick touched something, so
// an install with no deployments yet logged nothing at all between startup and
// its first deployment. That is exactly the window in which someone goes looking
// for evidence the aggregator is running, and finding silence reads as a hang —
// it cost a real debugging session on a cluster that was working correctly.
func TestAnIdleTickStillReportsItself(t *testing.T) {
	ctx := context.Background()
	store, incidents := newTestStores(t)
	capture := &captureHandler{}

	agg := newTestAggregator(store, incidents, nil, time.Now().UTC())
	agg.logger = slog.New(capture)

	// No incidents, no events: a tick with nothing to fold.
	require.NoError(t, agg.RunOnce(ctx))

	msgs := capture.messagesAtLeast(slog.LevelInfo)
	require.True(t, containsMessage(msgs, "tick complete"),
		"an idle tick must still report at info level, or it cannot be told apart from a stuck one; got %v", msgs)
}

// TestASkippedTickIsVisible pins that a replica standing down is visible at info
// level. On a scaled Observer this is the normal state of every replica but one,
// and it used to log at Debug — invisible at the default level, so a replica that
// was deliberately idle looked identical to one that was broken.
func TestASkippedTickIsVisible(t *testing.T) {
	ctx := context.Background()
	store, incidents := newTestStores(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	leader := newTestAggregator(store, incidents, nil, now)
	leader.tick(ctx) // takes the lease

	capture := &captureHandler{}
	follower := newTestAggregator(store, incidents, nil, now)
	follower.logger = slog.New(capture)
	follower.tick(ctx) // must stand down, and say so

	msgs := capture.messagesAtLeast(slog.LevelInfo)
	require.True(t, containsMessage(msgs, "Another replica holds"),
		"a replica standing down must say so at info level; got %v", msgs)
}
