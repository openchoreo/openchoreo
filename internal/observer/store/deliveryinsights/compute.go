// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deliveryinsights

import (
	"sort"
	"time"
)

// BucketStartMs returns the UTC bucket boundary containing the given epoch-ms instant:
// midnight for daily, Monday midnight for weekly, first of the month for monthly.
// Bucketing lives in Go (not SQL) so it is identical across database backends.
func BucketStartMs(granularity string, ms int64) int64 {
	t := time.UnixMilli(ms).UTC()
	switch granularity {
	case GranularityWeekly:
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		// time.Weekday: Sunday==0; shift so Monday starts the week.
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset).UnixMilli()
	case GranularityMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	default: // GranularityDaily
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	}
}

// NextBucketStartMs returns the start of the bucket following the one that starts at
// bucketStartMs. Used to iterate (and zero-fill) consecutive buckets over a window.
func NextBucketStartMs(granularity string, bucketStartMs int64) int64 {
	t := time.UnixMilli(bucketStartMs).UTC()
	switch granularity {
	case GranularityWeekly:
		return t.AddDate(0, 0, 7).UnixMilli()
	case GranularityMonthly:
		return t.AddDate(0, 1, 0).UnixMilli()
	default: // GranularityDaily
		return t.AddDate(0, 0, 1).UnixMilli()
	}
}

// Percentile returns the p-th percentile (0 < p <= 1) of values using the
// nearest-rank method. Returns nil for an empty input. The input is not mutated.
func Percentile(values []int64, p float64) *int64 {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	rank := int(float64(len(sorted))*p+0.999999) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	v := sorted[rank]
	return &v
}

// Mean returns the arithmetic mean of values, or nil for an empty input.
func Mean(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	var sum int64
	for _, v := range values {
		sum += v
	}
	m := sum / int64(len(values))
	return &m
}

// rollupScope identifies one (scope, environment slice) pair a fact contributes to.
type rollupScope struct {
	scopeType      string
	scopeUID       string
	environmentUID string
}

// scopesForFact returns every rollup a fact contributes to: org, project, and component
// scope, each both unsliced (environmentUID "") and sliced by the fact's environment.
func scopesForFact(orgNamespace, projectUID, componentUID, environmentUID string) []rollupScope {
	scopes := make([]rollupScope, 0, 6)
	for _, s := range []rollupScope{
		{ScopeTypeOrg, orgNamespace, ""},
		{ScopeTypeProject, projectUID, ""},
		{ScopeTypeComponent, componentUID, ""},
	} {
		if s.scopeUID == "" {
			continue
		}
		scopes = append(scopes, s)
		if environmentUID != "" {
			scopes = append(scopes, rollupScope{s.scopeType, s.scopeUID, environmentUID})
		}
	}
	return scopes
}

type rollupAccumulator struct {
	deployTotal   int
	deploySuccess int
	deployFailed  int
	leadTimes     []int64
	recoveries    []int64
	recoveryCount int
}

type rollupKey struct {
	scope       rollupScope
	granularity string
	bucketStart int64
}

// BuildRollups derives the complete set of metric rollups (all scopes × environment
// slices × granularities) from deployment and recovery facts. It is a pure function used
// by the aggregator after each tick — callers persist the result with UpsertRollups.
// Deployment counts bucket by the deployment moment; recovery stats by failure start.
func BuildRollups(facts []DeploymentFact, recoveries []RecoveryFact, computedAtMs int64) []MetricRollup {
	acc := make(map[rollupKey]*rollupAccumulator)
	granularities := []string{GranularityDaily, GranularityWeekly, GranularityMonthly}

	accumulate := func(scopes []rollupScope, occurredMs int64, fn func(a *rollupAccumulator)) {
		for _, scope := range scopes {
			for _, g := range granularities {
				key := rollupKey{scope, g, BucketStartMs(g, occurredMs)}
				a := acc[key]
				if a == nil {
					a = &rollupAccumulator{}
					acc[key] = a
				}
				fn(a)
			}
		}
	}

	for i := range facts {
		f := &facts[i]
		if f.Outcome == OutcomeInProgress {
			continue
		}
		scopes := scopesForFact(f.OrgNamespace, f.ProjectUID, f.ComponentUID, f.EnvironmentUID)
		occurred := f.OccurredMs()
		accumulate(scopes, occurred, func(a *rollupAccumulator) {
			a.deployTotal++
			if f.Outcome == OutcomeSuccess {
				a.deploySuccess++
			} else {
				a.deployFailed++
			}
			if f.LeadTimeMs != nil && *f.LeadTimeMs >= 0 {
				a.leadTimes = append(a.leadTimes, *f.LeadTimeMs)
			}
		})
	}

	for i := range recoveries {
		r := &recoveries[i]
		scopes := scopesForFact(r.OrgNamespace, r.ProjectUID, r.ComponentUID, r.EnvironmentUID)
		accumulate(scopes, r.FailureStartedMs, func(a *rollupAccumulator) {
			a.recoveryCount++
			if r.DurationMs != nil && *r.DurationMs >= 0 {
				a.recoveries = append(a.recoveries, *r.DurationMs)
			}
		})
	}

	rollups := make([]MetricRollup, 0, len(acc))
	for key, a := range acc {
		rollups = append(rollups, MetricRollup{
			ScopeType:      key.scope.scopeType,
			ScopeUID:       key.scope.scopeUID,
			EnvironmentUID: key.scope.environmentUID,
			Granularity:    key.granularity,
			BucketStartMs:  key.bucketStart,
			DeployTotal:    a.deployTotal,
			DeploySuccess:  a.deploySuccess,
			DeployFailed:   a.deployFailed,
			LeadTimeP50Ms:  Percentile(a.leadTimes, 0.50),
			LeadTimeP75Ms:  Percentile(a.leadTimes, 0.75),
			LeadTimeP95Ms:  Percentile(a.leadTimes, 0.95),
			MTTRMeanMs:     Mean(a.recoveries),
			MTTRP50Ms:      Percentile(a.recoveries, 0.50),
			RecoveryCount:  a.recoveryCount,
			ComputedAtMs:   computedAtMs,
		})
	}

	sort.Slice(rollups, func(i, j int) bool {
		a, b := &rollups[i], &rollups[j]
		if a.ScopeType != b.ScopeType {
			return a.ScopeType < b.ScopeType
		}
		if a.ScopeUID != b.ScopeUID {
			return a.ScopeUID < b.ScopeUID
		}
		if a.EnvironmentUID != b.EnvironmentUID {
			return a.EnvironmentUID < b.EnvironmentUID
		}
		if a.Granularity != b.Granularity {
			return a.Granularity < b.Granularity
		}
		return a.BucketStartMs < b.BucketStartMs
	})
	return rollups
}
