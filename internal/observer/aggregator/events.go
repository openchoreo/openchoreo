// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
)

// Delivery lifecycle event reasons, as emitted (per the Delivery Insights design)
// by the rendered-release controller onto the data-plane workload object.
const (
	ReasonDeploymentStarted   = "DeploymentStarted"
	ReasonDeploymentSucceeded = "DeploymentSucceeded"
	ReasonDeploymentFailed    = "DeploymentFailed"
	ReasonDeploymentRecovered = "DeploymentRecovered"
)

// DeliveryEvent is one delivery lifecycle event read from the event store.
type DeliveryEvent struct {
	// Reason is one of the ReasonDeployment* constants.
	Reason string
	// TimestampMs is the event's occurrence time (epoch ms).
	TimestampMs int64
	// Namespace is the org namespace the event was enriched with.
	Namespace string
	// Names for display, from enrichment.
	ProjectName     string
	ComponentName   string
	EnvironmentName string
	// Message carries the emitter's JSON payload (deliveryEventPayload).
	Message string
}

// EventsSource reads delivery lifecycle events from the observability event store.
// service.LogsAdapter implements it via FetchDeliveryEvents, which sweeps the event
// index using the logs-adapter `reasons` filter and `searchAfter` cursor. A deployed
// adapter without those extensions cannot serve the sweep, so the source stays behind
// INSIGHTS_EVENTS_SOURCE_ENABLED; the aggregator skips the events path while nil.
type EventsSource interface {
	// FetchDeliveryEvents returns delivery lifecycle events in [fromMs, toMs),
	// ordered by timestamp ascending (phase merges assume chronological folding).
	// complete reports whether the sweep reached the end of the window; a false
	// value means the page cap cut it short and the caller must not treat toMs as
	// swept, or the unread remainder is skipped forever.
	FetchDeliveryEvents(ctx context.Context, fromMs, toMs int64) (events []DeliveryEvent, complete bool, err error)
}

// deliveryEventPayload is the JSON the emitting controller embeds in the event
// message — everything the aggregator needs, independent of collector enrichment
// (Kubernetes Events do not inherit the involved object's labels).
type deliveryEventPayload struct {
	RenderedReleaseUID   string `json:"renderedReleaseUid"`
	ComponentReleaseName string `json:"componentReleaseName"`
	OrgNamespace         string `json:"orgNamespace"`
	ProjectUID           string `json:"projectUid"`
	ComponentUID         string `json:"componentUid"`
	EnvironmentUID       string `json:"environmentUid"`
	Commit               string `json:"commit"`
	CommitAuthoredAt     string `json:"commitAuthoredAt"`
	FailureReason        string `json:"failureReason"`
	// FailureEpisode distinguishes successive failure->recovery cycles of one
	// rollout. Without it every episode shares a recovery-fact ID, so episode 2's
	// Recovered merges into episode 1's row and the duration spans the healthy
	// interval between them -- inflating MTTR and undercounting recoveries.
	FailureEpisode int32 `json:"failureEpisode,omitempty"`
}

// eventsProgress is where the next events sweep must start from, recorded after a tick.
type eventsProgress struct {
	// watermarkMs is how far the sweep covered. A completed sweep covers the whole
	// window up to the tick time.
	watermarkMs int64
	// resumeMs is the exact position a capped sweep stopped at, or 0 when the window
	// was fully swept. It exists because the overlap that absorbs ingest lag would
	// otherwise eat the progress a capped sweep made: resuming at
	// watermark - Overlap can refill the page cap before reaching the stop point, so
	// the sweep would re-read the same pages every tick and never move past it.
	resumeMs int64
}

// processEvents reads delivery events since the events watermark and folds them into
// deployment/recovery facts. Returns the touched rollup moments and the progress to
// persist.
func (a *Aggregator) processEvents(
	ctx context.Context, tickStart time.Time,
) (touchedMs []int64, progress eventsProgress, err error) {
	watermark, err := a.store.Watermark(ctx, watermarkSourceEvents)
	if err != nil {
		return nil, eventsProgress{}, err
	}
	priorResumeMs, err := a.store.Watermark(ctx, watermarkSourceEventsResume)
	if err != nil {
		return nil, eventsProgress{}, err
	}

	var fromMs int64
	switch {
	case priorResumeMs > 0:
		// Resuming a capped sweep: we know exactly where it stopped, and we are behind
		// rather than caught up, so the ingest-lag overlap does not apply.
		fromMs = priorResumeMs
	case watermark == 0:
		// First run: raw events only live for the retention window; reach back a
		// generous-but-bounded slice of it rather than asking for all time.
		fromMs = tickStart.AddDate(0, 0, -14).UnixMilli()
	default:
		fromMs = watermark - a.cfg.Overlap.Milliseconds()
	}

	events, complete, err := a.events.FetchDeliveryEvents(ctx, fromMs, tickStart.UnixMilli())
	if err != nil {
		return nil, eventsProgress{}, err
	}
	progress = eventsProgress{watermarkMs: tickStart.UnixMilli()}
	// The !complete branch runs before the empty-result return on purpose. An
	// incomplete sweep that yielded no events must not advance the watermark to
	// tickStart and clear resumeMs, which would skip the remainder it stopped
	// short of. Unreachable with the current adapter (20 pages x 1000), but the
	// ordering is what makes it safe rather than the adapter's shape.
	if !complete && len(events) == 0 {
		progress.watermarkMs = watermark
		progress.resumeMs = priorResumeMs
		a.logger.Error("Delivery event sweep reported incomplete with no events; "+
			"holding position rather than advancing past the unread remainder",
			"fromMs", fromMs, "windowEndMs", tickStart.UnixMilli())
		return nil, progress, nil
	}
	if len(events) == 0 {
		return nil, progress, nil
	}
	if !complete {
		stoppedAtMs := events[len(events)-1].TimestampMs
		progress.watermarkMs = stoppedAtMs
		progress.resumeMs = stoppedAtMs
		if stoppedAtMs <= priorResumeMs {
			// A full sweep's worth of events sharing one millisecond cannot be paged
			// past on timestamp alone. Nothing is skipped, but the sweep is stuck and
			// needs a bigger cap, so say so rather than looping quietly.
			a.logger.Error("Delivery event sweep cannot advance past its stop position",
				"stoppedAtMs", stoppedAtMs, "events", len(events))
		}
		a.logger.Warn("Delivery event sweep hit its page cap; remainder resumes next tick",
			"events", len(events), "resumeFromMs", stoppedAtMs, "windowEndMs", tickStart.UnixMilli())
	}

	var touched []int64
	var facts []deliveryinsights.DeploymentFact
	var recoveries []deliveryinsights.RecoveryFact
	for i := range events {
		event := &events[i]
		fact, recovery, ok := a.foldEvent(event, tickStart)
		if !ok {
			continue
		}
		if fact != nil {
			facts = append(facts, *fact)
			touched = append(touched, fact.OccurredMs())
		}
		if recovery != nil {
			recoveries = append(recoveries, *recovery)
			touched = append(touched, recovery.FailureStartedMs)
		}
	}

	if len(facts) > 0 {
		if err := a.store.UpsertDeploymentFacts(ctx, facts); err != nil {
			return nil, eventsProgress{}, err
		}
	}
	if len(recoveries) > 0 {
		if err := a.store.UpsertRecoveryFacts(ctx, recoveries); err != nil {
			return nil, eventsProgress{}, err
		}
	}
	a.logger.Debug("Processed delivery events",
		"events", len(events), "facts", len(facts), "recoveries", len(recoveries))
	return touched, progress, nil
}

// foldEvent turns one delivery event into its fact writes. The store's upsert
// merge rules (phase COALESCE, sticky failure) do the heavy lifting: this only
// decides which columns each phase is authoritative for.
func (a *Aggregator) foldEvent(
	event *DeliveryEvent, tickStart time.Time,
) (*deliveryinsights.DeploymentFact, *deliveryinsights.RecoveryFact, bool) {
	var payload deliveryEventPayload
	if err := json.Unmarshal([]byte(event.Message), &payload); err != nil ||
		payload.RenderedReleaseUID == "" {
		a.logger.Warn("Skipping delivery event with invalid payload",
			"reason", event.Reason, "namespace", event.Namespace)
		return nil, nil, false
	}

	// The payload is authoritative; collector enrichment is the fallback for events
	// emitted before orgNamespace was carried in the message.
	//
	// Skipping here rather than letting the store reject the fact is the point:
	// UpsertDeploymentFacts validates the whole slice before writing any of it and
	// returns on the first error, so one event missing this field wrote none of the
	// batch, failed the tick, and left the watermark unmoved -- re-reading the same
	// bad event on every tick, forever.
	orgNamespace := payload.OrgNamespace
	if orgNamespace == "" {
		orgNamespace = event.Namespace
	}
	if orgNamespace == "" {
		a.logger.Warn("Skipping delivery event with no org namespace; it cannot be attributed",
			"reason", event.Reason, "renderedReleaseUid", payload.RenderedReleaseUID)
		return nil, nil, false
	}

	fact := deliveryinsights.DeploymentFact{
		ReleaseUID:       payload.RenderedReleaseUID,
		OrgNamespace:     orgNamespace,
		ProjectUID:       payload.ProjectUID,
		ComponentUID:     payload.ComponentUID,
		EnvironmentUID:   payload.EnvironmentUID,
		ProjectName:      event.ProjectName,
		ComponentName:    event.ComponentName,
		EnvironmentName:  event.EnvironmentName,
		ComponentRelease: payload.ComponentReleaseName,
		CommitSHA:        payload.Commit,
		UpdatedAtMs:      tickStart.UnixMilli(),
	}
	eventMs := event.TimestampMs

	switch event.Reason {
	case ReasonDeploymentStarted:
		fact.StartedMs = &eventMs
		fact.Outcome = deliveryinsights.OutcomeInProgress
	case ReasonDeploymentSucceeded:
		fact.ReadyMs = &eventMs
		fact.Outcome = deliveryinsights.OutcomeSuccess
		if authoredMs, err := parseEntryTime(payload.CommitAuthoredAt); err == nil {
			lead := eventMs - authoredMs
			fact.CommitAuthoredMs = &authoredMs
			fact.LeadTimeMs = &lead
		}
	case ReasonDeploymentFailed:
		fact.Outcome = deliveryinsights.OutcomeFailed
		fact.FailedBy = deliveryinsights.FailedByRollout
		fact.FailureReason = payload.FailureReason
		// The failure moment anchors the fact in time when no Started event was
		// folded. The store's merge keeps whichever started_ms is earliest, so this
		// never moves a fact that already has its true start.
		fact.StartedMs = &eventMs
		// Open a health-sourced recovery episode; DeploymentRecovered closes it.
		return &fact, &deliveryinsights.RecoveryFact{
			ID:               healthRecoveryID(payload.RenderedReleaseUID, payload.FailureEpisode),
			OrgNamespace:     orgNamespace,
			ProjectUID:       payload.ProjectUID,
			ComponentUID:     payload.ComponentUID,
			EnvironmentUID:   payload.EnvironmentUID,
			ReleaseUID:       payload.RenderedReleaseUID,
			Source:           deliveryinsights.RecoverySourceHealth,
			FailureStartedMs: eventMs,
			UpdatedAtMs:      tickStart.UnixMilli(),
		}, true
	case ReasonDeploymentRecovered:
		// Only closes the episode — the deployment fact keeps its failure.
		return nil, &deliveryinsights.RecoveryFact{
			ID:             healthRecoveryID(payload.RenderedReleaseUID, payload.FailureEpisode),
			OrgNamespace:   orgNamespace,
			ProjectUID:     payload.ProjectUID,
			ComponentUID:   payload.ComponentUID,
			EnvironmentUID: payload.EnvironmentUID,
			ReleaseUID:     payload.RenderedReleaseUID,
			Source:         deliveryinsights.RecoverySourceHealth,
			// On merge the store keeps the existing row's failure start (from the
			// Failed event) and derives duration from it; this value only lands
			// when no Failed event was ever folded (degenerate zero-length episode).
			FailureStartedMs: eventMs,
			RecoveredMs:      &eventMs,
			UpdatedAtMs:      tickStart.UnixMilli(),
		}, true
	default:
		a.logger.Warn("Skipping delivery event with unknown reason", "reason", event.Reason)
		return nil, nil, false
	}

	return &fact, nil, true
}

// healthRecoveryID identifies one failure->recovery episode of one rollout.
//
// The episode is part of the key: a rollout can fail, recover, and fail again, and
// each cycle is its own MTTR sample. Keying on the rollout alone merged them, and
// because the store preserves the original failure_started_ms on merge, the
// resulting duration ran from episode 1's failure to episode 2's recovery --
// spanning the healthy interval in between.
//
// Episode 0 means the emitter sent no episode (an event from before the field
// existed). Those keep the original ID so they stay idempotent rather than
// creating a duplicate alongside the row they already wrote.
func healthRecoveryID(releaseUID string, episode int32) string {
	if episode <= 0 {
		return fmt.Sprintf("health-%s", releaseUID)
	}
	return fmt.Sprintf("health-%s-e%d", releaseUID, episode)
}
