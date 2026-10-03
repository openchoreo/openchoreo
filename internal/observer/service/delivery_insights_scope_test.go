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
)

// scopeResolverStub answers every level with the name as its UID, except the one
// level it is told to fail.
type scopeResolverStub struct {
	failLevel string
	failWith  error
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
