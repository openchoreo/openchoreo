// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
)

// scopeResolverStub answers every level with the name as its UID, except the one
// level it is told to fail. Components have release lines when withLines is set.
type scopeResolverStub struct {
	failLevel string
	failWith  error
	withLines bool
}

func (r scopeResolverStub) resolve(level, name string) (string, error) {
	if level == r.failLevel {
		return "", r.failWith
	}
	return name, nil
}

func (r scopeResolverStub) GetNamespaceUID(_ context.Context, namespace string) (string, error) {
	return r.resolve("namespace", namespace)
}

func (r scopeResolverStub) GetProjectUID(_ context.Context, _, project string) (string, error) {
	return r.resolve("project", project)
}

func (r scopeResolverStub) GetComponentUID(_ context.Context, _, _, component string) (string, error) {
	return r.resolve("component", component)
}

func (r scopeResolverStub) GetEnvironmentUID(_ context.Context, _, environment string) (string, error) {
	return r.resolve("environment", environment)
}

func (r scopeResolverStub) GetReleaseLineUID(_ context.Context, _, _, releaseLine string) (string, error) {
	return r.resolve("release line", releaseLine)
}

func (r scopeResolverStub) HasReleaseLines(_ context.Context, _, component string) (bool, error) {
	if _, err := r.resolve("release lines", component); err != nil {
		return false, err
	}
	return r.withLines, nil
}

// TestDeliveryInsightsScopeResolution covers both query operations against a scope
// that fails to resolve at each level. An unknown namespace used to be the exception:
// it was never checked, so it matched no rows and came back as an empty success (#4806).
// Every failure is reported under delivery insights' own sentinel, where they used to
// borrow the alerts one (#4805).
func TestDeliveryInsightsScopeResolution(t *testing.T) {
	ctx := context.Background()
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -7)
	scope := gen.ComponentSearchScope{
		Namespace:   "acme",
		Project:     strPtr("shop"),
		Component:   strPtr("checkout"),
		Environment: strPtr("prod"),
	}

	notFound := fmt.Errorf("%w: /api/v1/namespaces/nope", ErrResourceNotFound)
	outage := errors.New("dial tcp 10.0.0.5:8080: connection refused")

	tests := []struct {
		name      string
		level     string
		failWith  error
		wantIs    error
		wantInMsg string
	}{
		{"unknown namespace", "namespace", notFound, ErrScopeNotFound, `namespace "acme" not found`},
		{"unknown project", "project", notFound, ErrScopeNotFound, `project "shop" not found`},
		{"unknown component", "component", notFound, ErrScopeNotFound, `component "checkout" not found`},
		{"unknown environment", "environment", notFound, ErrScopeNotFound, `environment "prod" not found`},
		{"namespace lookup unavailable", "namespace", outage, ErrScopeResolutionFailed, `failed to resolve namespace "acme"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewDeliveryInsightsService(
				newDeliveryInsightsTestStore(t), scopeResolverStub{failLevel: tt.level, failWith: tt.failWith},
				slog.Default(), true, func() bool { return true },
			)

			_, metricsErr := svc.QueryDoraMetrics(ctx, gen.DoraMetricsQueryRequest{
				StartTime: start, EndTime: end, SearchScope: scope,
			})
			_, deploymentsErr := svc.QueryDoraDeployments(ctx, gen.DoraDeploymentsQueryRequest{
				StartTime: start, EndTime: end, SearchScope: scope,
			})

			for op, err := range map[string]error{"metrics": metricsErr, "deployments": deploymentsErr} {
				require.Error(t, err, "%s: a scope that does not resolve must fail the query", op)
				assert.ErrorIs(t, err, tt.wantIs, op)
				assert.ErrorIs(t, err, ErrDeliveryInsightsResolveSearchScope, op)
				assert.ErrorIs(t, err, tt.failWith, "%s: the resolver's error must stay in the chain", op)
				assert.NotErrorIs(t, err, ErrAlertsResolveSearchScope,
					"%s: delivery insights must not report its failures as the alerts subsystem's", op)
				assert.Contains(t, err.Error(), tt.wantInMsg, op)
				assert.NotContains(t, err.Error(), "alerts", op)
			}
		})
	}

	t.Run("a namespace that resolves is still queried by name", func(t *testing.T) {
		svc := NewDeliveryInsightsService(
			newDeliveryInsightsTestStore(t), scopeResolverStub{}, slog.Default(),
			true, func() bool { return true },
		)
		resp, err := svc.QueryDoraMetrics(ctx, gen.DoraMetricsQueryRequest{
			StartTime: start, EndTime: end, SearchScope: gen.ComponentSearchScope{Namespace: "acme"},
		})
		require.NoError(t, err)
		assert.Equal(t, "acme", resp.Scope.Namespace)
	})
}

// TestWrapScopeErrorKeepsTheAlertsSentinel pins that the alerts and incidents paths,
// which share the helper, still report under their own sentinel.
func TestWrapScopeErrorKeepsTheAlertsSentinel(t *testing.T) {
	err := wrapScopeError(fmt.Errorf("%w: x", ErrResourceNotFound), "project", "shop")
	assert.ErrorIs(t, err, ErrAlertsResolveSearchScope)
	assert.ErrorIs(t, err, ErrScopeNotFound)
	assert.NotErrorIs(t, err, ErrDeliveryInsightsResolveSearchScope)
}

// TestDeliveryInsightsReleaseLineScope pins that a release line narrows both the
// fact-backed summary and the rollup-backed series to that line.
func TestDeliveryInsightsReleaseLineScope(t *testing.T) {
	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Hour)
	start := end.AddDate(0, 0, -7)
	deployed := end.Add(-24 * time.Hour).UnixMilli()

	fact := func(releaseUID, lineUID string, offsetMs int64) deliveryinsights.DeploymentFact {
		ready := deployed + offsetMs
		return deliveryinsights.DeploymentFact{
			ReleaseUID: releaseUID, Namespace: "acme", ProjectUID: "shop", ComponentUID: "checkout",
			EnvironmentUID: "prod", ReleaseLineUID: lineUID, ReadyMs: &ready,
			Outcome: deliveryinsights.OutcomeSuccess, UpdatedAtMs: ready,
		}
	}
	facts := []deliveryinsights.DeploymentFact{
		fact("rel-stable", "stable", 0), fact("rel-next-1", "next", 1000), fact("rel-next-2", "next", 2000),
	}
	store := newDeliveryInsightsTestStore(t)
	require.NoError(t, store.UpsertDeploymentFacts(ctx, facts))
	require.NoError(t, store.UpsertRollups(ctx, deliveryinsights.BuildRollups(facts, nil, deployed)))

	svc := NewDeliveryInsightsService(store, scopeResolverStub{}, slog.Default(), true, func() bool { return true })
	scope := gen.ComponentSearchScope{Namespace: "acme", Project: strPtr("shop"), Component: strPtr("checkout")}

	total := func(scope gen.ComponentSearchScope) (int, int) {
		t.Helper()
		resp, err := svc.QueryDoraMetrics(ctx, gen.DoraMetricsQueryRequest{StartTime: start, EndTime: end, SearchScope: scope})
		require.NoError(t, err)
		seriesTotal := 0
		for _, p := range *resp.Series.DeploymentFrequency {
			if p.Count != nil {
				seriesTotal += *p.Count
			}
		}
		require.NotNil(t, resp.Summary.DeploymentFrequency.Total)
		return *resp.Summary.DeploymentFrequency.Total, seriesTotal
	}

	summary, series := total(scope)
	assert.Equal(t, 3, summary, "the component counts every line")
	assert.Equal(t, 3, series)

	scope.ReleaseLine = strPtr("next")
	summary, series = total(scope)
	assert.Equal(t, 2, summary)
	assert.Equal(t, 2, series)

	deployments, err := svc.QueryDoraDeployments(ctx, gen.DoraDeploymentsQueryRequest{StartTime: start, EndTime: end, SearchScope: scope})
	require.NoError(t, err)
	require.NotNil(t, deployments.TotalCount)
	assert.Equal(t, 2, *deployments.TotalCount)
}

// A component scope without a line covers the main line. A component without release lines keeps
// its component rollups and every fact, which hold its history from before release lines.
func TestDeliveryInsightsMainLineScope(t *testing.T) {
	ctx := context.Background()
	all := true
	tests := []struct {
		name      string
		withLines bool
		line      string
		all       *bool
		wantType  string
		wantAll   bool
	}{
		{"no line, component without lines", false, "", nil, deliveryinsights.ScopeTypeComponent, true},
		{"no line, component with lines", true, "", nil, deliveryinsights.ScopeTypeReleaseLine, false},
		{"every line", true, "", &all, deliveryinsights.ScopeTypeComponent, true},
		{"main keyword, component with lines", true, "main", nil, deliveryinsights.ScopeTypeReleaseLine, false},
		{"main keyword, component without lines", false, "main", nil, deliveryinsights.ScopeTypeComponent, true},
	}
	t.Run("failed release line check keeps the component scope", func(t *testing.T) {
		svc := &DoraMetricsService{logger: slog.Default(), resolver: scopeResolverStub{withLines: true,
			failLevel: "release lines", failWith: errors.New("unexpected status 403")}}
		rs, err := svc.resolveScope(ctx, gen.ComponentSearchScope{
			Namespace: "acme", Project: strPtr("shop"), Component: strPtr("checkout")})
		require.NoError(t, err)
		assert.Equal(t, deliveryinsights.ScopeTypeComponent, rs.scopeType)
		assert.True(t, rs.allReleaseLines)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &DoraMetricsService{resolver: scopeResolverStub{withLines: tt.withLines}, logger: slog.Default()}
			rs, err := svc.resolveScope(ctx, gen.ComponentSearchScope{
				Namespace: "acme", Project: strPtr("shop"), Component: strPtr("checkout"),
				ReleaseLine: strPtr(tt.line), AllReleaseLines: tt.all,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, rs.scopeType)
			assert.Equal(t, "checkout", rs.scopeUID)
			assert.Equal(t, tt.wantAll, rs.allReleaseLines)
		})
	}
}
