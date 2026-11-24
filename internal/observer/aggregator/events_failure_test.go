// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// failingEventsSource serves its events like fakeEventsSource while err is nil, and
// fails every sweep with err otherwise: an adapter that is down, absent, or too old
// to know the sweep, as opposed to one that answers 501.
type failingEventsSource struct {
	fakeEventsSource
	err   error
	calls int
}

func (f *failingEventsSource) FetchDeliveryEvents(
	ctx context.Context, fromMs, toMs int64,
) ([]DeliveryEvent, bool, error) {
	f.calls++
	if f.err != nil {
		return nil, false, f.err
	}
	return f.fakeEventsSource.FetchDeliveryEvents(ctx, fromMs, toMs)
}

var errAdapterDown = errors.New(
	`failed to call logs adapter delivery events query: dial tcp: connect: connection refused`)

// writeIncident records a resolved incident on the checkout-api production fixture.
func writeIncident(
	t *testing.T, incidents incidententry.IncidentEntryStore, alertID string, triggered, resolved time.Time,
) {
	t.Helper()
	_, err := incidents.WriteIncidentEntry(context.Background(), &incidententry.IncidentEntry{
		AlertID:         alertID,
		Timestamp:       triggered.Format(time.RFC3339Nano),
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
}

// failedBy maps each deployment fact around now to who failed it ("" when it
// succeeded).
func failedBy(t *testing.T, store deliveryinsights.Store, now time.Time) map[string]string {
	t.Helper()
	facts, _, err := store.QueryDeploymentFacts(context.Background(), deliveryinsights.FactQuery{
		Namespace: "default",
		StartMs:   now.Add(-48 * time.Hour).UnixMilli(),
		EndMs:     now.Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)
	out := make(map[string]string, len(facts))
	for _, f := range facts {
		out[f.ReleaseUID] = f.FailedBy
	}
	return out
}

func recoveryCount(t *testing.T, store deliveryinsights.Store, now time.Time) int {
	t.Helper()
	rollups, err := store.QueryRollups(context.Background(), deliveryinsights.RollupQuery{
		ScopeType:   deliveryinsights.ScopeTypeComponent,
		ScopeUID:    "checkout-api",
		Namespace:   "default",
		ProjectUID:  "checkout",
		Granularity: deliveryinsights.GranularityDaily,
		StartMs:     deliveryinsights.BucketStartMs(deliveryinsights.GranularityDaily, now.Add(-48*time.Hour).UnixMilli()),
		EndMs:       now.UnixMilli() + 1,
	})
	require.NoError(t, err)
	total := 0
	for _, r := range rollups {
		total += r.RecoveryCount
	}
	return total
}

// TestADeploymentAndIncidentInOneTickBlameOnlyTheNewDeployment pins the order the
// sources are folded in. An incident is attributed to the deployment live when it
// fired, and an attribution is never undone. Folding incidents before events meant
// a rollout and an incident that both arrived since the last tick were attributed
// without the rollout's fact: the incident landed on the previous deployment, and
// on the next tick -- the 30-day rescan finding the newer fact -- on the new one
// as well. One incident, two failed deployments.
func TestADeploymentAndIncidentInOneTickBlameOnlyTheNewDeployment(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// The previous rollout, already folded on an earlier tick.
	require.NoError(t, store.UpsertDeploymentFacts(ctx, []deliveryinsights.DeploymentFact{
		successFact("rel-old", now.Add(-3*time.Hour).UnixMilli()),
	}))

	// Since the last tick: a new rollout, then an incident it caused.
	deployed := now.Add(-4 * time.Minute)
	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentStarted, "rel-new", deployed.Add(-time.Minute), nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-new", deployed, nil),
	}}
	writeIncident(t, incidents, "alert-1", now.Add(-2*time.Minute), now.Add(-time.Minute))

	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, now)))
	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, now.Add(5*time.Minute))))

	assert.Equal(t, map[string]string{
		"rel-old": "",
		"rel-new": deliveryinsights.FailedByIncident,
	}, failedBy(t, store, now), "the incident must count against the rollout live when it fired, and only that one")
}

// TestAFailingEventsSourceDoesNotStopIncidents is the non-501 counterpart of
// TestUnservableEventsSourceDoesNotStopIncidents. An adapter that is unreachable,
// or too old to know the sweep and answers 404 or 400, used to fail the whole
// tick: incidents were folded and then thrown away with it, so Mean Time to
// Recovery -- which needs no adapter -- stayed empty for as long as the adapter did.
func TestAFailingEventsSourceDoesNotStopIncidents(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	writeIncident(t, incidents, "alert-1", now.Add(-time.Hour), now.Add(-30*time.Minute))

	source := &failingEventsSource{err: errAdapterDown}
	a := newTestAggregator(store, incidents, source, now)
	require.NoError(t, runOnce(t, a), "an events failure must not fail the tick")

	assert.Equal(t, 1, recoveryCount(t, store, now), "MTTR must be computed without the adapter")

	incidentsWm, err := store.Watermark(ctx, watermarkSourceIncidents)
	require.NoError(t, err)
	assert.Equal(t, now.UnixMilli(), incidentsWm, "the incidents watermark advances")

	eventsWm, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	assert.Zero(t, eventsWm, "the events watermark must hold, or the window the adapter missed is skipped")

	// Unlike a 501, a failure is not a statement about what the adapter can do, so
	// the sweep is not stood down: the next tick asks again.
	assert.True(t, a.EventsActive())
	require.NoError(t, runOnce(t, a))
	assert.Equal(t, 2, source.calls, "a failing sweep is retried every tick")
}

// TestEventsMissedDuringAnOutageAreReadWhenItEnds pins that holding the events
// watermark through an outage loses nothing: the first sweep after it reads the
// whole window the failed ticks did not.
func TestEventsMissedDuringAnOutageAreReadWhenItEnds(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// One good tick establishes a watermark.
	source := &failingEventsSource{}
	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, start)))

	// The adapter goes down for an hour, during which a rollout happens.
	source.err = errAdapterDown
	deployed := start.Add(20 * time.Minute)
	source.events = []DeliveryEvent{
		deliveryEvent(ReasonDeploymentStarted, "rel-1", deployed.Add(-time.Minute), nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-1", deployed, nil),
	}
	for tick := 1; tick <= 12; tick++ {
		require.NoError(t, runOnce(t, newTestAggregator(
			store, incidents, source, start.Add(time.Duration(tick)*5*time.Minute))))
	}
	assert.Empty(t, failedBy(t, store, start), "nothing is folded while the adapter is down")

	// It comes back.
	source.err = nil
	end := start.Add(65 * time.Minute)
	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, end)))

	assert.Equal(t, map[string]string{"rel-1": ""}, failedBy(t, store, start),
		"the rollout from the outage must be folded once the adapter is back")
	eventsWm, err := store.Watermark(ctx, watermarkSourceEvents)
	require.NoError(t, err)
	assert.Equal(t, end.UnixMilli(), eventsWm)
}

// TestAnIncidentDuringAnEventsOutageBlamesOnlyItsOwnDeployment pins why
// attribution waits for the events path. While the adapter is down, the rollout
// that caused an incident has no fact yet, so attributing then would blame the
// rollout before it -- permanently, since attribution is never undone -- and the
// rescan after the outage would blame the right one as well.
func TestAnIncidentDuringAnEventsOutageBlamesOnlyItsOwnDeployment(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	ctx := context.Background()
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.UpsertDeploymentFacts(ctx, []deliveryinsights.DeploymentFact{
		successFact("rel-old", start.Add(-3*time.Hour).UnixMilli()),
	}))

	source := &failingEventsSource{err: errAdapterDown}
	deployed := start.Add(10 * time.Minute)
	source.events = []DeliveryEvent{
		deliveryEvent(ReasonDeploymentStarted, "rel-new", deployed.Add(-time.Minute), nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-new", deployed, nil),
	}
	writeIncident(t, incidents, "alert-1", deployed.Add(5*time.Minute), deployed.Add(20*time.Minute))

	during := start.Add(30 * time.Minute)
	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, during)))
	assert.Equal(t, map[string]string{"rel-old": ""}, failedBy(t, store, start),
		"no attribution while the rollout that caused it may still be unread")
	assert.Equal(t, 1, recoveryCount(t, store, during), "MTTR does not wait for the adapter")

	source.err = nil
	after := start.Add(35 * time.Minute)
	require.NoError(t, runOnce(t, newTestAggregator(store, incidents, source, after)))

	assert.Equal(t, map[string]string{
		"rel-old": "",
		"rel-new": deliveryinsights.FailedByIncident,
	}, failedBy(t, store, start))

	recoveries, err := store.QueryRecoveryFacts(ctx, deliveryinsights.FactQuery{
		Namespace: "default", StartMs: start.UnixMilli(), EndMs: after.UnixMilli(),
	})
	require.NoError(t, err)
	require.Len(t, recoveries, 1)
	assert.Equal(t, "rel-new", recoveries[0].ReleaseUID,
		"the recovery fact picks up its rollout once attribution runs")
}

// TestAnEventsOutageIsLoggedOnceAndItsEndAnnounced pins the log shape of an
// outage. An install with no adapter at all fails every sweep, every five
// minutes, indefinitely; a warning each time buries everything else in the log.
func TestAnEventsOutageIsLoggedOnceAndItsEndAnnounced(t *testing.T) {
	t.Parallel()

	store, incidents := newTestStores(t)
	capture := &captureHandler{}
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	source := &failingEventsSource{err: errAdapterDown}
	a := newTestAggregator(store, incidents, source, start)
	a.logger = slog.New(capture)

	for tick := 0; tick < 3; tick++ {
		a.now = func() time.Time { return start.Add(time.Duration(tick) * 5 * time.Minute) }
		require.NoError(t, runOnce(t, a))
	}
	warnings := capture.messagesAtLeast(slog.LevelWarn)
	require.Len(t, warnings, 1, "one warning for the outage, not one per tick; got %v", warnings)
	assert.Contains(t, warnings[0], "Delivery events sweep failed")

	source.err = nil
	a.now = func() time.Time { return start.Add(15 * time.Minute) }
	require.NoError(t, runOnce(t, a))
	assert.True(t, containsMessage(capture.messagesAtLeast(slog.LevelInfo), "Delivery events sweep recovered"))
}
