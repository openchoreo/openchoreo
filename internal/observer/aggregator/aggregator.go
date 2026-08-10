// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package aggregator implements the DORA aggregator: a background worker in the
// observer that folds delivery signals into the durable delivery insights store.
// Each tick it reads incidents and delivery lifecycle events since its per-source
// watermark, normalizes them into deployment/recovery facts, attributes incidents to
// the deployment live at trigger time, and recomputes the metric rollups for every
// bucket it touched. The events source is opt-in
// (INSIGHTS_EVENTS_SOURCE_ENABLED) because it needs a logs adapter carrying the
// reasons filter and searchAfter cursor; without it only the incident path runs.
//
// Correctness rests on the store's semantics, not on tick bookkeeping: facts
// upsert on stable keys with sticky-failure merge rules, rollups are recomputed
// from facts and fully replaced (never incremented), and watermarks advance only
// after a tick commits — so re-processing any window, or a full backfill, is
// idempotent by construction.
package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

const (
	watermarkSourceIncidents = "incidents"
	watermarkSourceEvents    = "events"

	// incidentQueryLimit bounds a single incident read. The lookback window is paged
	// through at this size rather than truncated: the window start is derived from the
	// tick time, not from the watermark, so a truncated read would re-read the same
	// first page every tick and never attribute the remainder.
	incidentQueryLimit = 10000

	// incidentMaxPages bounds one tick's paging so a pathological window cannot spin
	// the tick forever. Hitting it is logged, and the remainder is picked up next tick.
	incidentMaxPages = 50
)

// Config tunes the aggregator loop.
type Config struct {
	// Interval between ticks.
	Interval time.Duration
	// Overlap re-read window absorbing source ingest lag (events arriving with
	// timestamps slightly before the previous watermark).
	Overlap time.Duration
	// AttributionWindow caps how long after a deployment an incident may trigger
	// and still be attributed to it.
	AttributionWindow time.Duration
	// IncidentLookback is the rolling window of incidents re-scanned every tick.
	// Incidents are rescanned (not watermark-incremental) because resolving one
	// does not bump its ingestion timestamp — a pure watermark would miss any
	// resolution that lands after the incident leaves the overlap window, leaving
	// its MTTR episode open forever. Volume is tiny, so the rescan is cheap;
	// rollups only recompute for incidents that actually changed.
	IncidentLookback time.Duration
}

// Aggregator folds incidents and delivery events into the insights store.
type Aggregator struct {
	store     deliveryinsights.Store
	incidents incidententry.IncidentEntryStore
	// events is nil when the deployed logs adapter lacks the reasons filter and
	// searchAfter cursor the sweep needs, which is why it stays behind
	// INSIGHTS_EVENTS_SOURCE_ENABLED. The events path is skipped when nil.
	events           EventsSource
	cfg              Config
	logger           *slog.Logger
	now              func() time.Time // injectable for tests
	incidentPageSize int              // overridable for tests
}

// New creates an aggregator. events may be nil, in which case only the incident path
// runs: MTTR stays live from incidents while deployment frequency, lead time and change
// failure rate wait on the delivery events source.
func New(
	store deliveryinsights.Store,
	incidents incidententry.IncidentEntryStore,
	events EventsSource,
	cfg Config,
	logger *slog.Logger,
) *Aggregator {
	return &Aggregator{
		store:            store,
		incidents:        incidents,
		events:           events,
		cfg:              cfg,
		logger:           logger,
		now:              time.Now,
		incidentPageSize: incidentQueryLimit,
	}
}

// Run ticks until ctx is cancelled. A failed tick logs and retries on the next
// interval — the watermark did not advance, so no data is skipped.
func (a *Aggregator) Run(ctx context.Context) {
	a.logger.Info("DORA aggregator started",
		"interval", a.cfg.Interval,
		"attributionWindow", a.cfg.AttributionWindow,
		"eventsSource", a.events != nil,
	)

	// First tick immediately so a restart doesn't wait a full interval.
	if err := a.RunOnce(ctx); err != nil && ctx.Err() == nil {
		a.logger.Error("DORA aggregation tick failed", "error", err)
	}

	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("DORA aggregator stopped")
			return
		case <-ticker.C:
			if err := a.RunOnce(ctx); err != nil && ctx.Err() == nil {
				a.logger.Error("DORA aggregation tick failed", "error", err)
			}
		}
	}
}

// RunOnce executes a single aggregation tick.
func (a *Aggregator) RunOnce(ctx context.Context) error {
	tickStart := a.now().UTC()
	var touched []int64

	incidentTouched, err := a.processIncidents(ctx, tickStart)
	if err != nil {
		return fmt.Errorf("incidents: %w", err)
	}
	touched = append(touched, incidentTouched...)

	// eventsSweptToMs is how far the events sweep actually got, which is short of
	// tickStart when the page cap cut it off.
	eventsSweptToMs := tickStart.UnixMilli()
	if a.events != nil {
		eventTouched, sweptToMs, eventsErr := a.processEvents(ctx, tickStart)
		if eventsErr != nil {
			return fmt.Errorf("events: %w", eventsErr)
		}
		touched = append(touched, eventTouched...)
		eventsSweptToMs = sweptToMs
	}

	if len(touched) > 0 {
		if err := a.recomputeRollups(ctx, touched, tickStart); err != nil {
			return fmt.Errorf("rollups: %w", err)
		}
	}

	// Watermarks advance last: a failure above re-processes the window next tick.
	if err := a.store.SetWatermark(ctx, watermarkSourceIncidents, tickStart.UnixMilli()); err != nil {
		return err
	}
	if a.events != nil {
		// Advance only as far as the sweep reached, so a capped sweep resumes from
		// where it stopped rather than jumping the unread remainder.
		if err := a.store.SetWatermark(ctx, watermarkSourceEvents, eventsSweptToMs); err != nil {
			return err
		}
	}

	if len(touched) > 0 {
		a.logger.Info("DORA aggregation tick complete",
			"touchedMoments", len(touched), "tookMs", a.now().UTC().Sub(tickStart).Milliseconds())
	}
	return nil
}

// processIncidents scans the incident lookback window, attributes incidents to
// the deployment live at trigger time, and upserts incident-sourced recovery
// facts. Returns the epoch-ms moments whose rollup buckets were touched — only
// for incidents that are new or changed since the last tick, so an unchanged
// window recomputes nothing.
func (a *Aggregator) processIncidents(ctx context.Context, tickStart time.Time) ([]int64, error) {
	watermark, err := a.store.Watermark(ctx, watermarkSourceIncidents)
	if err != nil {
		return nil, err
	}
	// First run backfills all history (incidents are durable — no retention
	// constraint); afterwards a rolling lookback window is rescanned every tick.
	fromMs := int64(0)
	if watermark > 0 {
		fromMs = tickStart.Add(-a.cfg.IncidentLookback).UnixMilli()
		if fromMs < 0 {
			fromMs = 0
		}
	}
	// Moments newer than this are considered changed since the last tick.
	changedSinceMs := watermark - a.cfg.Overlap.Milliseconds()

	// Page forward through the window. seen guards against re-processing an entry that
	// straddles a page boundary, since the cursor is inclusive of its own timestamp.
	var touched []int64
	seen := make(map[string]struct{})
	cursorMs := fromMs
	processed := 0

	for page := 0; page < incidentMaxPages; page++ {
		entries, _, queryErr := a.incidents.QueryIncidentEntries(ctx, incidententry.QueryParams{
			StartTime: time.UnixMilli(cursorMs).UTC().Format(time.RFC3339Nano),
			EndTime:   tickStart.Format(time.RFC3339Nano),
			Limit:     a.incidentPageSize,
			SortOrder: "ASC",
		})
		if queryErr != nil {
			return nil, fmt.Errorf("query incident entries: %w", queryErr)
		}
		if len(entries) == 0 {
			break
		}

		pageTouched, count, lastTriggeredMs, foldErr := a.foldIncidentPage(
			ctx, entries, seen, changedSinceMs, tickStart)
		if foldErr != nil {
			return nil, foldErr
		}
		touched = append(touched, pageTouched...)
		processed += count

		if len(entries) < a.incidentPageSize {
			break // short page: the window is exhausted
		}
		if lastTriggeredMs <= cursorMs {
			// A full page sharing one trigger timestamp cannot be paged past without
			// skipping entries. Practically unreachable at this page size; bail out
			// rather than spin.
			a.logger.Warn("Incident page did not advance the cursor; stopping this tick",
				"cursorMs", cursorMs, "pageSize", len(entries))
			break
		}
		cursorMs = lastTriggeredMs

		if page == incidentMaxPages-1 {
			a.logger.Warn("Incident window paging hit its page cap; remainder processed on later ticks",
				"pages", incidentMaxPages, "processed", processed)
		}
	}

	a.logger.Debug("Processed incidents", "incidents", processed)
	return touched, nil
}

// foldIncidentPage attributes one page of incidents and upserts their recovery facts.
// It returns the moments whose rollup buckets were touched, how many entries it folded,
// and the newest trigger time in the page, which becomes the next page's cursor.
func (a *Aggregator) foldIncidentPage(
	ctx context.Context,
	entries []incidententry.IncidentEntry,
	seen map[string]struct{},
	changedSinceMs int64,
	tickStart time.Time,
) (touched []int64, processed int, lastTriggeredMs int64, err error) {
	var recoveries []deliveryinsights.RecoveryFact
	attributedCount := 0
	for i := range entries {
		entry := &entries[i]
		triggeredMs, parseErr := parseEntryTime(entry.TriggeredAt)
		if parseErr != nil {
			a.logger.Warn("Skipping incident with unparseable trigger time",
				"incident", entry.ID, "triggeredAt", entry.TriggeredAt)
			continue
		}
		if triggeredMs > lastTriggeredMs {
			lastTriggeredMs = triggeredMs
		}
		if _, done := seen[entry.ID]; done {
			continue // already folded earlier in this tick
		}
		seen[entry.ID] = struct{}{}
		processed++

		attribution, attrErr := a.store.AttributeIncident(ctx,
			entry.ComponentID, entry.EnvironmentID, entry.ID,
			triggeredMs, a.cfg.AttributionWindow.Milliseconds())
		if attrErr != nil {
			return nil, 0, 0, attrErr
		}
		if attribution.Attributed {
			attributedCount++
			touched = append(touched, attribution.OccurredMs)
		}

		fact := deliveryinsights.RecoveryFact{
			ID:               "incident-" + entry.ID,
			OrgNamespace:     entry.NamespaceName,
			ProjectUID:       entry.ProjectID,
			ComponentUID:     entry.ComponentID,
			EnvironmentUID:   entry.EnvironmentID,
			ReleaseUID:       attribution.ReleaseUID,
			IncidentID:       entry.ID,
			Source:           deliveryinsights.RecoverySourceIncident,
			FailureStartedMs: triggeredMs,
			UpdatedAtMs:      tickStart.UnixMilli(),
		}
		changed := triggeredMs >= changedSinceMs // new incident since last tick
		if entry.ResolvedAt != "" {
			if resolvedMs, resolveErr := parseEntryTime(entry.ResolvedAt); resolveErr == nil {
				fact.RecoveredMs = &resolvedMs
				if resolvedMs >= changedSinceMs {
					changed = true // resolution landed since last tick
				}
			}
		}
		recoveries = append(recoveries, fact)
		if changed {
			touched = append(touched, triggeredMs)
		}
	}

	if len(recoveries) > 0 {
		if err := a.store.UpsertRecoveryFacts(ctx, recoveries); err != nil {
			return nil, 0, 0, err
		}
	}
	a.logger.Debug("Folded incident page",
		"incidents", len(recoveries), "attributedDeployments", attributedCount)
	return touched, processed, lastTriggeredMs, nil
}

// recomputeRollups rebuilds every rollup bucket containing a touched moment.
// Buckets are always derived from the full fact set in range, so late updates
// to old buckets (e.g. an incident flipping last week's deployment to failed)
// replace stale rollups rather than drifting from them.
func (a *Aggregator) recomputeRollups(ctx context.Context, touchedMs []int64, tickStart time.Time) error {
	minTouched := touchedMs[0]
	for _, ms := range touchedMs[1:] {
		if ms < minTouched {
			minTouched = ms
		}
	}
	// Snap to the widest bucket boundary containing the earliest touched moment,
	// so the monthly bucket it falls in is fully recomputed.
	startMs := deliveryinsights.BucketStartMs(deliveryinsights.GranularityMonthly, minTouched)
	endMs := tickStart.UnixMilli() + 1

	factQuery := deliveryinsights.FactQuery{
		StartMs: startMs,
		EndMs:   endMs,
		Limit:   rollupFactLimit,
		// Deployment moment ascending keeps the read deterministic.
		SortOrder: "ASC",
	}
	facts, total, err := a.store.QueryDeploymentFacts(ctx, factQuery)
	if err != nil {
		return err
	}
	if total > len(facts) {
		a.logger.Warn("Rollup recompute fact read truncated — rollups may lag until volume drops",
			"total", total, "read", len(facts))
	}
	recoveries, err := a.store.QueryRecoveryFacts(ctx, factQuery)
	if err != nil {
		return err
	}

	rollups := deliveryinsights.BuildRollups(facts, recoveries, tickStart.UnixMilli())
	return a.store.UpsertRollups(ctx, rollups)
}

// rollupFactLimit bounds one recompute read; matches the store's max query limit.
const rollupFactLimit = 10000

func parseEntryTime(value string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}
