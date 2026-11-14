// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// tickCompleteAttrs returns the attributes of the last tick-complete line.
func (h *captureHandler) tickCompleteAttrs(t *testing.T) map[string]slog.Value {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.records) - 1; i >= 0; i-- {
		if h.records[i].Message != "DORA aggregation tick complete" {
			continue
		}
		attrs := map[string]slog.Value{}
		h.records[i].Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value
			return true
		})
		return attrs
	}
	t.Fatal("no tick-complete line was logged")
	return nil
}

// TestTickReportsWhatItCouldNotUse pins the counts on the tick-complete line.
//
// Each input below is dropped, or only partly used, for a reason that is correct
// on its own -- and until these counts, none of them left a trace. A dashboard
// built from what remained looked healthy while covering a fraction of what
// happened (#4843).
func TestTickReportsWhatItCouldNotUse(t *testing.T) {
	store, incidents := newTestStores(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	at := now.Add(-20 * time.Minute)
	authored := at.Add(-3 * time.Hour).Format(time.RFC3339)

	noNamespace := deliveryEvent(ReasonDeploymentSucceeded, "rel-no-ns", at, nil)
	noNamespace.Namespace = ""
	invalid := deliveryEvent(ReasonDeploymentSucceeded, "rel-invalid", at, nil)
	invalid.Message = "{not json"

	source := &fakeEventsSource{events: []DeliveryEvent{
		deliveryEvent(ReasonDeploymentSucceeded, "rel-commit", at, map[string]string{"commitAuthoredAt": authored}),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-no-commit", at, nil),
		deliveryEvent(ReasonDeploymentSucceeded, "rel-bad-commit", at, map[string]string{"commitAuthoredAt": "last tuesday"}),
		deliveryEvent("DeploymentExploded", "rel-unknown", at, nil),
		noNamespace,
		invalid,
	}}

	// A deployment for the scoped incident to land on, from before the window above.
	deployedAt := now.Add(-2 * time.Hour)
	require.NoError(t, store.UpsertDeploymentFacts(ctx,
		[]deliveryinsights.DeploymentFact{successFact("rel-live", deployedAt.UnixMilli())}))

	triggered := deployedAt.Add(30 * time.Minute).Format(time.RFC3339Nano)
	for _, entry := range []*incidententry.IncidentEntry{
		// Attributed to rel-live.
		{AlertID: "a-attributed", NamespaceName: "default", ComponentID: "checkout-api", EnvironmentID: "production"},
		// No component: nothing to blame.
		{AlertID: "a-unscoped", NamespaceName: "default", EnvironmentID: "production"},
		// Scoped, but that component never deployed to that environment.
		{AlertID: "a-no-deploy", NamespaceName: "default", ComponentID: "other-api", EnvironmentID: "production"},
		// Attributed too, but with no namespace its recovery episode is dropped.
		{AlertID: "a-no-ns", ComponentID: "checkout-api", EnvironmentID: "production"},
	} {
		entry.Timestamp = triggered
		entry.TriggeredAt = triggered
		entry.Status = incidententry.StatusActive
		_, err := incidents.WriteIncidentEntry(ctx, entry)
		require.NoError(t, err)
	}
	capture := &captureHandler{}
	agg := newTestAggregator(store, incidents, source, now)
	agg.logger = slog.New(capture)
	require.NoError(t, runOnce(t, agg))

	attrs := capture.tickCompleteAttrs(t)
	count := func(key string) int64 {
		t.Helper()
		v, ok := attrs[key]
		require.True(t, ok, "tick-complete line is missing %q", key)
		return v.Int64()
	}

	assert.EqualValues(t, 6, count("eventsRead"))
	assert.EqualValues(t, 3, count("eventsFolded"), "the three succeeded events with a namespace and a valid payload")
	assert.EqualValues(t, 3, count("eventsSkipped"))
	assert.Equal(t, map[string]int{
		skipReasonInvalidPayload: 1,
		skipReasonNoNamespace:    1,
		skipReasonUnknownReason:  1,
	}, attrs["eventsSkippedByReason"].Any())
	assert.EqualValues(t, 1, count("leadTimeNoCommit"))
	assert.EqualValues(t, 1, count("leadTimeUnparseable"))

	assert.EqualValues(t, 4, count("incidentsInWindow"))
	// a-attributed marks rel-live failed; a-no-ns finds it already attributed.
	assert.EqualValues(t, 1, count("incidentsAttributed"))
	assert.EqualValues(t, 1, count("incidentsUnscoped"))
	assert.EqualValues(t, 1, count("incidentsNoDeployment"))
	assert.EqualValues(t, 1, count("recoveriesNoNamespace"))

	assert.True(t, containsMessage(capture.messagesAtLeast(slog.LevelWarn), "unparseable commit authoring time"),
		"a malformed commit time is a defect in the event and must be logged on its own")
}

// TestAnIdleTickReportsZeroCounts pins that every count is present on every tick,
// zero included, so a log query sees the same shape whether or not there was work.
func TestAnIdleTickReportsZeroCounts(t *testing.T) {
	store, incidents := newTestStores(t)
	capture := &captureHandler{}
	agg := newTestAggregator(store, incidents, &fakeEventsSource{}, time.Now().UTC())
	agg.logger = slog.New(capture)
	require.NoError(t, runOnce(t, agg))

	attrs := capture.tickCompleteAttrs(t)
	for _, key := range []string{
		"eventsRead", "eventsFolded", "eventsSkipped", "leadTimeNoCommit", "leadTimeUnparseable",
		"incidentsInWindow", "incidentsAttributed", "incidentsUnscoped", "incidentsNoDeployment",
		"recoveriesNoNamespace",
	} {
		v, ok := attrs[key]
		if assert.True(t, ok, "missing %q", key) {
			assert.EqualValues(t, 0, v.Int64(), key)
		}
	}
}
