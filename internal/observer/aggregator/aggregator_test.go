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
}

func (f *fakeEventsSource) FetchDeliveryEvents(_ context.Context, fromMs, toMs int64) ([]DeliveryEvent, error) {
	var out []DeliveryEvent
	for _, e := range f.events {
		if e.TimestampMs >= fromMs && e.TimestampMs < toMs {
			out = append(out, e)
		}
	}
	return out, nil
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
