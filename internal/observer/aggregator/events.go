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

// EventsSource reads delivery lifecycle events from the observability event
// store. There is no implementation yet: the controllers do not emit delivery
// events, and reading them efficiently needs the logs-adapter `reasons` filter
// (implementation design §5.4). The aggregator skips the events path while nil.
type EventsSource interface {
	// FetchDeliveryEvents returns delivery lifecycle events in [fromMs, toMs),
	// ordered by timestamp ascending (phase merges assume chronological folding).
	FetchDeliveryEvents(ctx context.Context, fromMs, toMs int64) ([]DeliveryEvent, error)
}

// deliveryEventPayload is the JSON the emitting controller embeds in the event
// message — everything the aggregator needs, independent of collector enrichment
// (Kubernetes Events do not inherit the involved object's labels).
type deliveryEventPayload struct {
	RenderedReleaseUID   string `json:"renderedReleaseUid"`
	ComponentReleaseName string `json:"componentReleaseName"`
	ProjectUID           string `json:"projectUid"`
	ComponentUID         string `json:"componentUid"`
	EnvironmentUID       string `json:"environmentUid"`
	Commit               string `json:"commit"`
	CommitAuthoredAt     string `json:"commitAuthoredAt"`
	FailureReason        string `json:"failureReason"`
}

// processEvents reads delivery events since the events watermark and folds them
// into deployment/recovery facts. Returns the touched rollup moments.
func (a *Aggregator) processEvents(ctx context.Context, tickStart time.Time) ([]int64, error) {
	watermark, err := a.store.Watermark(ctx, watermarkSourceEvents)
	if err != nil {
		return nil, err
	}
	fromMs := watermark - a.cfg.Overlap.Milliseconds()
	if watermark == 0 {
		// First run: raw events only live for the retention window; reach back a
		// generous-but-bounded slice of it rather than asking for all time.
		fromMs = tickStart.AddDate(0, 0, -14).UnixMilli()
	}

	events, err := a.events.FetchDeliveryEvents(ctx, fromMs, tickStart.UnixMilli())
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
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
			return nil, err
		}
	}
	if len(recoveries) > 0 {
		if err := a.store.UpsertRecoveryFacts(ctx, recoveries); err != nil {
			return nil, err
		}
	}
	a.logger.Debug("Processed delivery events",
		"events", len(events), "facts", len(facts), "recoveries", len(recoveries))
	return touched, nil
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

	fact := deliveryinsights.DeploymentFact{
		ReleaseUID:       payload.RenderedReleaseUID,
		OrgNamespace:     event.Namespace,
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
		// folded (the merge keeps an earlier started_ms when one exists).
		fact.StartedMs = &eventMs
		// Open a health-sourced recovery episode; DeploymentRecovered closes it.
		return &fact, &deliveryinsights.RecoveryFact{
			ID:               healthRecoveryID(payload.RenderedReleaseUID),
			OrgNamespace:     event.Namespace,
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
			ID:             healthRecoveryID(payload.RenderedReleaseUID),
			OrgNamespace:   event.Namespace,
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

func healthRecoveryID(releaseUID string) string {
	return fmt.Sprintf("health-%s", releaseUID)
}
