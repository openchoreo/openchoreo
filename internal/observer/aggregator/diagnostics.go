// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

// Reasons a delivery event was read but folded into nothing. They are the keys of
// eventsSkipped on the tick-complete line.
const (
	skipReasonInvalidPayload = "invalidPayload"
	skipReasonNoNamespace    = "noNamespace"
	skipReasonUnknownReason  = "unknownReason"
)

// tickStats counts what one aggregation tick read and what it could not use.
//
// Every stage of the pipeline discards input it cannot attribute -- an event
// with no namespace, a deployment with no commit, an incident with no component
// -- and each discard is correct on its own. Together they mean a dashboard can
// show a confident number over a subset of what happened, with nothing erroring.
// These counts are what an operator needs when the numbers look wrong: they say
// how much of the input each metric actually covers, without anyone having to
// reproduce the pipeline by hand.
//
// The two sweeps count differently, because they read differently. The events
// sweep reads each tick's new window plus an ingest-lag overlap, so its counts
// can include events re-read from the previous tick. The incident sweep rescans
// its whole lookback window every tick, so its counts describe the window: an
// unscoped incident is counted on every tick until it ages out, which is what
// makes the figure comparable from one tick to the next.
type tickStats struct {
	eventsRead   int
	eventsFolded int
	// eventsSkipped counts events folded into nothing, by skipReason*.
	eventsSkipped map[string]int
	// Succeeded deployments that carried no commit authoring time. They count
	// toward deployment frequency but have no lead time -- the coverage the
	// metrics API reports is the other side of this number.
	leadTimeNoCommit int
	// Succeeded deployments whose commit authoring time did not parse. Unlike a
	// missing commit this is a defect in the event, so each is also logged.
	leadTimeUnparseable int

	incidentsInWindow int
	// incidentsAttributed counts incidents this tick newly marked a deployment
	// failed for. An incident attributed on an earlier tick is not counted again.
	incidentsAttributed int
	// Incidents with no component or environment UID. They cannot be attributed
	// to a deployment at all, so they never count toward change failure rate.
	incidentsUnscoped int
	// Scoped incidents with no deployment of that component and environment
	// within the attribution window before they triggered.
	incidentsNoDeployment int
	// Incidents with no namespace. They are still attributed, but their recovery
	// episode cannot be scoped, so they contribute nothing to MTTR.
	recoveriesNoNamespace int
}

func newTickStats() *tickStats {
	return &tickStats{eventsSkipped: map[string]int{}}
}

func (s *tickStats) skipEvent(reason string) {
	s.eventsSkipped[reason]++
}

// logAttrs renders the counts as the tick-complete line's attributes. Every count
// is always present, zero included, so a query over the logs sees the same shape
// on every tick and a zero reads as "none" rather than "not reported".
func (s *tickStats) logAttrs() []any {
	skipped := 0
	for _, n := range s.eventsSkipped {
		skipped += n
	}
	return []any{
		"eventsRead", s.eventsRead,
		"eventsFolded", s.eventsFolded,
		"eventsSkipped", skipped,
		"eventsSkippedByReason", s.eventsSkipped,
		"leadTimeNoCommit", s.leadTimeNoCommit,
		"leadTimeUnparseable", s.leadTimeUnparseable,
		"incidentsInWindow", s.incidentsInWindow,
		"incidentsAttributed", s.incidentsAttributed,
		"incidentsUnscoped", s.incidentsUnscoped,
		"incidentsNoDeployment", s.incidentsNoDeployment,
		"recoveriesNoNamespace", s.recoveriesNoNamespace,
	}
}
