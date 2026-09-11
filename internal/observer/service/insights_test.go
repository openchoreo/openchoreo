// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
)

// The DORA math had no unit tests: the handler tests mock InsightsService
// wholesale, so they cover wiring and error mapping only. These drive the real
// service against a real SQLite store so the SQL is exercised too.

func newInsightsTestStore(t *testing.T) deliveryinsights.Store {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	store, err := deliveryinsights.New(deliveryinsights.BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.Initialize(context.Background()))
	return store
}

func newInsightsTestService(t *testing.T, store deliveryinsights.Store) InsightsService {
	t.Helper()
	return NewInsightsService(store, NewPassthroughUIDResolver(), slog.Default())
}

// TestLeadTimePercentilesUseTheWholeWindow is the regression test for the paging
// default leaking into the metrics path. FactQuery.Limit of 0 means "one page"
// (100 rows, ORDER BY ready_ms ASC), so the percentiles were computed over the
// *oldest* 100 deployments in the window while CountDeployments stayed exact.
// The sample was biased, not merely small.
func TestLeadTimePercentilesUseTheWholeWindow(t *testing.T) {
	ctx := context.Background()
	store := newInsightsTestStore(t)

	// 150 deployments, lead time rising with deploy time: truncating to the oldest
	// rows understates every percentile.
	const count = 150
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	facts := make([]deliveryinsights.DeploymentFact, 0, count)
	for i := 0; i < count; i++ {
		ready := start.Add(time.Duration(i) * time.Hour)
		readyMs := ready.UnixMilli()
		lead := int64(i+1) * 1000
		authored := readyMs - lead
		facts = append(facts, deliveryinsights.DeploymentFact{
			ReleaseUID:       fmt.Sprintf("rollout-%03d", i),
			OrgNamespace:     "acme",
			ProjectUID:       "shop",
			ComponentUID:     "checkout",
			EnvironmentUID:   "prod",
			ProjectName:      "shop",
			ComponentName:    "checkout",
			EnvironmentName:  "prod",
			CommitSHA:        fmt.Sprintf("%040d", i),
			CommitAuthoredMs: &authored,
			ReadyMs:          &readyMs,
			Outcome:          deliveryinsights.OutcomeSuccess,
			LeadTimeMs:       &lead,
			UpdatedAtMs:      readyMs,
		})
	}
	require.NoError(t, store.UpsertDeploymentFacts(ctx, facts))

	resp, err := newInsightsTestService(t, store).QueryDoraMetrics(ctx, gen.DoraMetricsQueryRequest{
		SearchScope: gen.ComponentSearchScope{
			Namespace: "acme",
			Project:   strPtr("shop"),
			Component: strPtr("checkout"),
		},
		StartTime: start.Add(-time.Hour),
		EndTime:   start.Add(count * time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	lt := resp.Summary.LeadTime
	require.NotNil(t, lt)
	require.NotNil(t, lt.P95Ms)

	// With all 150 rows, p95 lands in the top of the distribution. Truncated to the
	// oldest 100 it could not exceed 100_000.
	require.Greater(t, *lt.P95Ms, int64(100_000),
		"p95 must be computed over the whole window, not the oldest page of rows")

	// Coverage is lead-times-with-provenance over deployments counted; the two must
	// be drawn from the same sample or it reads below 1 with full provenance.
	require.NotNil(t, lt.Coverage)
	require.InDelta(t, 1.0, *lt.Coverage, 0.001,
		"every deployment has provenance, so coverage must be 1")
}

func TestClassifyDeploymentFrequency(t *testing.T) {
	for _, tc := range []struct {
		name   string
		total  int
		perDay float64
		want   gen.DoraClassification
	}{
		{"no deployments is unknown, not low", 0, 0, gen.DoraClassificationUnknown},
		{"daily or better is elite", 30, 1, gen.DoraClassificationElite},
		{"just under daily is high", 6, 0.99, gen.DoraClassificationHigh},
		{"weekly is high", 4, 1.0 / 7, gen.DoraClassificationHigh},
		{"monthly is medium", 1, 1.0 / 30, gen.DoraClassificationMedium},
		{"less than monthly is low", 1, 1.0 / 60, gen.DoraClassificationLow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, string(tc.want), classifyDeploymentFrequency(tc.total, tc.perDay))
		})
	}
}

func TestClassifyLeadTime(t *testing.T) {
	h := time.Hour.Milliseconds()
	d := 24 * h
	require.Equal(t, string(gen.DoraClassificationUnknown), classifyLeadTime(nil),
		"no provenance must read as unknown, not elite")
	for _, tc := range []struct {
		name string
		p50  int64
		want gen.DoraClassification
	}{
		{"under a day is elite", 23 * h, gen.DoraClassificationElite},
		{"exactly a day is high", d, gen.DoraClassificationHigh},
		{"under a week is high", 6 * d, gen.DoraClassificationHigh},
		{"exactly a week is medium", 7 * d, gen.DoraClassificationMedium},
		{"exactly a month is low", 30 * d, gen.DoraClassificationLow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.p50
			require.Equal(t, string(tc.want), classifyLeadTime(&p))
		})
	}
}

func TestClassifyChangeFailureRate(t *testing.T) {
	require.Equal(t, string(gen.DoraClassificationUnknown), classifyChangeFailureRate(0, 0),
		"an empty window must not report a 0%% failure rate as elite")
	for _, tc := range []struct {
		rate float64
		want gen.DoraClassification
	}{
		{0.05, gen.DoraClassificationElite},
		{0.10, gen.DoraClassificationHigh},
		{0.15, gen.DoraClassificationMedium},
		{0.16, gen.DoraClassificationLow},
	} {
		t.Run(fmt.Sprintf("rate %.2f", tc.rate), func(t *testing.T) {
			require.Equal(t, string(tc.want), classifyChangeFailureRate(10, tc.rate))
		})
	}
}

func TestDeltaPct(t *testing.T) {
	require.Nil(t, deltaPct(5, 0), "no previous window means no delta, not +Inf")
	require.NotNil(t, deltaPct(0, 4))
	require.InDelta(t, -100.0, *deltaPct(0, 4), 0.001)
	require.InDelta(t, 50.0, *deltaPct(6, 4), 0.001)
	require.InDelta(t, -25.0, *deltaPct(3, 4), 0.001)
}

func strPtr(s string) *string { return &s }

// TestDistributionReadsAreNotCapped covers the window that exceeds any fixed cap.
//
// Raising the row limit from 100 to 10,000 only moved the threshold at which the
// numbers start lying: the reads are ORDER BY ... ASC, so a cap keeps the oldest
// rows and drops the newest, and percentiles computed from that are biased while
// CountDeployments stays exact. With 10,001 facts the old cap would have dropped
// exactly one -- the newest, and by construction the largest -- so p95 and the
// deployment count would disagree about which population they describe.
func TestDistributionReadsAreNotCapped(t *testing.T) {
	ctx := context.Background()
	store := newInsightsTestStore(t)

	const count = 10_001
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	facts := make([]deliveryinsights.DeploymentFact, 0, count)
	for i := 0; i < count; i++ {
		readyMs := start.Add(time.Duration(i) * time.Minute).UnixMilli()
		lead := int64(i+1) * 1000
		authored := readyMs - lead
		facts = append(facts, deliveryinsights.DeploymentFact{
			ReleaseUID:       fmt.Sprintf("rollout-%05d", i),
			OrgNamespace:     "acme",
			ProjectUID:       "shop",
			ComponentUID:     "checkout",
			EnvironmentUID:   "prod",
			CommitSHA:        fmt.Sprintf("%040d", i),
			CommitAuthoredMs: &authored,
			ReadyMs:          &readyMs,
			Outcome:          deliveryinsights.OutcomeSuccess,
			LeadTimeMs:       &lead,
			UpdatedAtMs:      readyMs,
		})
	}
	require.NoError(t, store.UpsertDeploymentFacts(ctx, facts))

	resp, err := newInsightsTestService(t, store).QueryDoraMetrics(ctx, gen.DoraMetricsQueryRequest{
		SearchScope: gen.ComponentSearchScope{
			Namespace: "acme",
			Project:   strPtr("shop"),
			Component: strPtr("checkout"),
		},
		StartTime: start.Add(-time.Minute),
		EndTime:   start.Add(count * time.Minute),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The count is exact, and the distribution has to describe the same population.
	require.NotNil(t, resp.Summary.DeploymentFrequency)
	require.NotNil(t, resp.Summary.DeploymentFrequency.Total)
	require.Equal(t, count, *resp.Summary.DeploymentFrequency.Total)

	lt := resp.Summary.LeadTime
	require.NotNil(t, lt)
	require.NotNil(t, lt.P95Ms)

	// Exact, because the difference a cap makes here is one row. Percentile ranks as
	// int(n*p + 0.999999) - 1, so over all 10,001 values the rank is 9500 and p95 is
	// 9,501,000ms; over the oldest 10,000 the rank is 9499 and p95 is 9,500,000ms. A
	// loose bound passes either way -- checked, it does -- so it has to be equality.
	require.Equal(t, int64(9_501_000), *lt.P95Ms,
		"p95 must come from all %d rows, not the oldest page", count)

	// Coverage cannot discriminate here: it is round3(len(leadTimes)/success), and
	// 10000/10001 rounds to 1.000 just as 10001/10001 does. Asserted as a sanity
	// check on the happy path, not as the guard against truncation.
	require.NotNil(t, lt.Coverage)
	require.InDelta(t, 1.0, *lt.Coverage, 0.001)
}
