// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package aggregator implements the DORA aggregator: a background worker in the
// observer that folds delivery signals into the durable delivery insights store.
// Each tick it reads incidents and delivery lifecycle events since its per-source
// watermark, normalizes them into deployment/recovery facts, attributes incidents to
// the deployment live at trigger time, and recomputes the metric rollups for every
// bucket it touched. The events source is opt-in
// (DELIVERY_INSIGHTS_EVENTS_SOURCE_ENABLED) because it needs a logs adapter carrying the
// reasons filter, and able to return them across every namespace in one query
// rather than a scope at a time; without it only the incident path runs.
//
// Correctness rests on the store's semantics, not on tick bookkeeping: facts
// upsert on stable keys with sticky-failure merge rules, rollups are recomputed
// from facts and fully replaced (never incremented), and watermarks advance only
// after a tick commits — so re-processing any window, or a full backfill, is
// idempotent by construction.
//
// Exactly one replica aggregates at a time. Every replica runs this loop, but a
// tick only proceeds while the replica holds the aggregation lease in the store,
// so an Observer can be scaled for read traffic without the sweeps colliding.
package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

const (
	watermarkSourceIncidents = "incidents"
	watermarkSourceEvents    = "events"
	// watermarkSourceEventsResume holds the exact position a page-capped event sweep
	// stopped at, so the next tick resumes there instead of re-reading from
	// watermark - Overlap. Zero means the last sweep covered its whole window.
	watermarkSourceEventsResume = "events_resume"

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
// aggregationLease is the single lease name every replica contends for.
const aggregationLease = "dora-aggregation"

type Aggregator struct {
	store     deliveryinsights.Store
	incidents incidententry.IncidentEntryStore
	// events is nil when the deployed logs adapter cannot filter events by reason, or
	// cannot return them across every namespace in one query -- the sweep covers the
	// whole install on a timer, so asking scope by scope is not an option. That is why
	// it stays behind DELIVERY_INSIGHTS_EVENTS_SOURCE_ENABLED, and the events path is
	// skipped when nil.
	events           EventsSource
	cfg              Config
	logger           *slog.Logger
	now              func() time.Time // injectable for tests
	incidentPageSize int              // overridable for tests
	// holder identifies this replica in the aggregation lease. It has to be unique
	// per process, not per pod: a restarted pod reusing its name must not be able to
	// renew the lease its predecessor held.
	holder string
	// renewInterval overrides the lease renewal cadence in tests, where waiting a
	// third of a real TTL is not an option.
	renewInterval time.Duration
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
		holder:           newHolderID(),
	}
}

// newHolderID identifies this process in the aggregation lease. The hostname
// makes a held lease traceable to a pod; the UUID keeps it unique across restarts
// of a pod that keeps its name.
func newHolderID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "observer"
	}
	return host + "-" + uuid.NewString()
}

// leaseTTL is how long a won tick keeps the lease. It outlasts the interval so the
// holder keeps aggregating across ticks rather than re-contending each time, and a
// holder that dies hard is replaced within it. A graceful stop releases early, so
// this only bounds the crash case.
func (a *Aggregator) leaseTTL() time.Duration {
	ttl := 2 * a.cfg.Interval
	if ttl < time.Minute {
		ttl = time.Minute
	}
	return ttl
}

// Run ticks until ctx is cancelled. A failed tick logs and retries on the next
// interval — the watermark did not advance, so no data is skipped.
//
// Every replica runs this loop, but only the lease holder aggregates. The others
// idle at the same interval, taking over within one TTL if the holder stops.
func (a *Aggregator) Run(ctx context.Context) {
	a.logger.Info("DORA aggregator started",
		"interval", a.cfg.Interval,
		"attributionWindow", a.cfg.AttributionWindow,
		"eventsSource", a.events != nil,
		"holder", a.holder,
	)
	defer a.releaseLease()

	// First tick immediately so a restart doesn't wait a full interval.
	a.tick(ctx)

	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("DORA aggregator stopped")
			return
		case <-ticker.C:
			a.tick(ctx)
		}
	}
}

// tick aggregates if this replica holds the lease, and does nothing if it does
// not. A lease store that errors is treated as not-held: skipping a tick costs a
// delay, whereas aggregating alongside another replica corrupts the watermarks.
func (a *Aggregator) tick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	held, err := a.store.AcquireLease(
		ctx, aggregationLease, a.holder,
		a.now().UTC().UnixMilli(), a.leaseTTL().Milliseconds(),
	)
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Error("Failed to acquire DORA aggregation lease, skipping tick", "error", err)
		}
		return
	}
	if !held {
		// Info, not Debug: on a scaled Observer this is the normal state of every
		// replica but one, and "am I the one aggregating" is the first question
		// asked when the metrics look stale.
		a.logger.Info("Another replica holds the DORA aggregation lease, skipping tick",
			"holder", a.holder)
		return
	}

	// Acquiring once is not enough. A sweep can outlast the TTL -- the first tick
	// after enabling reads a 30-day incident window and whatever event backlog
	// exists -- and an expired lease is takeable, so another replica could start
	// writing facts and watermarks while this one is still going. The lease is
	// renewed for as long as the tick runs, and losing it cancels the tick instead
	// of letting it write on.
	tickCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	renewed := a.holdLease(tickCtx, cancel)

	err = a.RunOnce(tickCtx)
	cancel()
	<-renewed

	switch {
	case err == nil:
	case ctx.Err() != nil:
		// The aggregator is shutting down; not a tick failure.
	case tickCtx.Err() != nil:
		a.logger.Warn("DORA aggregation tick abandoned after losing the lease", "holder", a.holder)
	default:
		a.logger.Error("DORA aggregation tick failed", "error", err)
	}
}

// holdLease renews the lease while a tick runs and calls lost if it ever cannot,
// so the tick is cancelled rather than continuing to write without the lease. It
// returns a channel closed once the renewer has stopped, so the caller can be
// sure no renewal outlives the tick.
//
// This narrows the window rather than closing it: like any lease-based election,
// a renewal that succeeds moments before the holder stalls still leaves a gap. It
// is bounded by the renewal interval, and a store unreachable for long enough to
// lose the lease is one this replica's writes are failing against anyway.
func (a *Aggregator) holdLease(ctx context.Context, lost func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(a.leaseRenewInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				held, err := a.store.AcquireLease(
					ctx, aggregationLease, a.holder,
					a.now().UTC().UnixMilli(), a.leaseTTL().Milliseconds(),
				)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					a.logger.Error("Failed to renew DORA aggregation lease, abandoning tick",
						"holder", a.holder, "error", err)
					lost()
					return
				}
				if !held {
					a.logger.Warn("Lost the DORA aggregation lease to another replica, abandoning tick",
						"holder", a.holder)
					lost()
					return
				}
			}
		}
	}()
	return done
}

// leaseRenewInterval renews comfortably inside the TTL, so a single slow or failed
// renewal does not forfeit the lease.
func (a *Aggregator) leaseRenewInterval() time.Duration {
	if a.renewInterval > 0 {
		return a.renewInterval
	}
	return a.leaseTTL() / 3
}

// releaseLease hands the lease back on shutdown so a surviving replica starts on
// its next tick instead of waiting out the TTL. Best-effort by nature: the process
// is stopping, and an expired lease reaches the same place.
func (a *Aggregator) releaseLease() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 5*time.Second)
	defer cancel()
	if err := a.store.ReleaseLease(ctx, aggregationLease, a.holder); err != nil {
		a.logger.Warn("Failed to release DORA aggregation lease", "error", err)
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

	// eventsProgress records how far the events sweep got, which is short of tickStart
	// when the page cap cut it off.
	eventsProg := eventsProgress{watermarkMs: tickStart.UnixMilli()}
	if a.events != nil {
		eventTouched, progress, eventsErr := a.processEvents(ctx, tickStart)
		if eventsErr != nil {
			return fmt.Errorf("events: %w", eventsErr)
		}
		touched = append(touched, eventTouched...)
		eventsProg = progress
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
		if err := a.store.SetWatermark(ctx, watermarkSourceEvents, eventsProg.watermarkMs); err != nil {
			return err
		}
		// Zero when the window was fully swept, which re-arms the ingest-lag overlap.
		if err := a.store.SetWatermark(ctx, watermarkSourceEventsResume, eventsProg.resumeMs); err != nil {
			return err
		}
	}

	// Reported on every tick, including the ones that touched nothing. Logging
	// only when there was work leaves an idle aggregator looking exactly like a
	// stuck one -- no line either way -- and an install with no deployments yet is
	// precisely when someone goes looking for proof it is running.
	a.logger.Info("DORA aggregation tick complete",
		"touchedMoments", len(touched), "tookMs", a.now().UTC().Sub(tickStart).Milliseconds())
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

		pageTouched, count, lastIngestedMs, foldErr := a.foldIncidentPage(
			ctx, entries, seen, changedSinceMs, tickStart)
		if foldErr != nil {
			return nil, foldErr
		}
		touched = append(touched, pageTouched...)
		processed += count

		if len(entries) < a.incidentPageSize {
			break // short page: the window is exhausted
		}
		if lastIngestedMs <= cursorMs {
			// A full page sharing one ingestion timestamp cannot be paged past
			// without skipping entries. Practically unreachable at this page size;
			// bail out rather than spin.
			a.logger.Warn("Incident page did not advance the cursor; stopping this tick",
				"cursorMs", cursorMs, "pageSize", len(entries))
			break
		}
		cursorMs = lastIngestedMs

		if page == incidentMaxPages-1 {
			a.logger.Warn("Incident window paging hit its page cap; remainder processed on later ticks",
				"pages", incidentMaxPages, "processed", processed)
		}
	}

	a.logger.Debug("Processed incidents", "incidents", processed)
	return touched, nil
}

// foldIncidentPage attributes one page of incidents and upserts their recovery facts.
// It returns the moments whose rollup buckets were touched, how many entries it
// folded, and the newest ingestion time in the page, which becomes the next
// page's cursor.
func (a *Aggregator) foldIncidentPage(
	ctx context.Context,
	entries []incidententry.IncidentEntry,
	seen map[string]struct{},
	changedSinceMs int64,
	tickStart time.Time,
) (touched []int64, processed int, lastIngestedMs int64, err error) {
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
		// The cursor advances on Timestamp, the ingestion time the incident query
		// filters and orders by -- not on TriggeredAt, which is when the alert
		// fired. The two are equal for every entry the alert path writes today, but
		// paging on a column the query does not order by would skip whatever sorts
		// between them the moment they diverge. TriggeredAt stays what attribution
		// is measured from.
		if ingestedMs, tsErr := parseEntryTime(entry.Timestamp); tsErr != nil {
			a.logger.Warn("Incident has an unparseable ingestion time; not advancing the cursor past it",
				"incident", entry.ID, "timestamp", entry.Timestamp)
		} else if ingestedMs > lastIngestedMs {
			lastIngestedMs = ingestedMs
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
	return touched, processed, lastIngestedMs, nil
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
	// BuildRollups emits daily, weekly and monthly buckets, so the read has to cover
	// every bucket it will rebuild. Snapping to the monthly boundary is not enough: a
	// week that starts in the previous month (any month whose 1st is not a Monday)
	// would be rebuilt from a partial fact set, and UpsertRollups replaces rather
	// than increments, so the undercount persists.
	//
	// Read from the weekly boundary of the month start -- the earliest start of any
	// bucket that can contain minTouched -- then discard the buckets that boundary
	// still leaves partially covered.
	monthStartMs := deliveryinsights.BucketStartMs(deliveryinsights.GranularityMonthly, minTouched)
	readStartMs := deliveryinsights.BucketStartMs(deliveryinsights.GranularityWeekly, monthStartMs)
	endMs := tickStart.UnixMilli() + 1

	// All, not a Limit: a rollup is a count and a set of percentiles over its
	// bucket, so a capped read produces undercounted buckets -- and because
	// UpsertRollups replaces, the undercount persists. Skipping the write on
	// truncation was the previous guard, but a window that stays above the cap then
	// never recomputes at all, so the rollups go stale indefinitely instead. Reading
	// every row removes both failure modes.
	factQuery := deliveryinsights.FactQuery{
		StartMs: readStartMs,
		EndMs:   endMs,
		All:     true,
		// Deployment moment ascending keeps the read deterministic.
		SortOrder: "ASC",
	}
	facts, _, err := a.store.QueryDeploymentFacts(ctx, factQuery)
	if err != nil {
		return err
	}
	recoveries, err := a.store.QueryRecoveryFacts(ctx, factQuery)
	if err != nil {
		return err
	}

	rollups := deliveryinsights.BuildRollups(facts, recoveries, tickStart.UnixMilli())

	// A bucket starting at or after readStartMs is fully covered by the read, which
	// runs to tickStart. One starting before it is not -- the monthly bucket of the
	// previous month, reached only because a straddling week pulled the read back --
	// so it must not be written.
	covered := rollups[:0]
	for _, r := range rollups {
		if r.BucketStartMs >= readStartMs {
			covered = append(covered, r)
		}
	}
	return a.store.UpsertRollups(ctx, covered)
}

func parseEntryTime(value string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}
