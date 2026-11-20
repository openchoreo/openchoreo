// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// fakeEnvLister serves a fixed environment list, or err.
type fakeEnvLister struct {
	envs  []service.Environment
	err   error
	calls int
}

func (f *fakeEnvLister) ListEnvironments(context.Context, string) ([]service.Environment, error) {
	f.calls++
	return f.envs, f.err
}

// perEnvDoraService answers DORA queries per environment: deploys[env] successful
// deployments, or errs[env]. The combined query (no environment) is keyed "".
type perEnvDoraService struct {
	deploys  map[string]int
	errs     map[string]error
	requests []string
}

func (s *perEnvDoraService) QueryDoraMetrics(
	_ context.Context, req gen.DoraMetricsQueryRequest,
) (*gen.DoraMetricsQueryResponse, error) {
	env := ""
	if req.SearchScope.Environment != nil {
		env = *req.SearchScope.Environment
	}
	s.requests = append(s.requests, env)
	if err := s.errs[env]; err != nil {
		return nil, err
	}

	// Each requested metric is present, classified, and counts total. Lead time,
	// as the service builds it, is present whenever asked for but has a median
	// only when there was a sample.
	total := s.deploys[env]
	wants := func(metric string) bool {
		if req.Metrics == nil {
			return true
		}
		for _, m := range *req.Metrics {
			if string(m) == metric {
				return true
			}
		}
		return false
	}
	summary := map[string]any{}
	if wants("deploymentFrequency") {
		summary["deploymentFrequency"] = map[string]any{"classification": "elite", "total": total}
	}
	if wants("changeFailureRate") {
		summary["changeFailureRate"] = map[string]any{"classification": "elite", "total": total}
	}
	if wants("mttr") {
		summary["mttr"] = map[string]any{"classification": "elite", "recoveries": total}
	}
	if wants("leadTime") {
		leadTime := map[string]any{"classification": "elite"}
		if total > 0 {
			leadTime["p50Ms"] = 3600000
		}
		summary["leadTime"] = leadTime
	}
	raw, err := json.Marshal(map[string]any{"summary": summary})
	if err != nil {
		return nil, err
	}
	resp := &gen.DoraMetricsQueryResponse{}
	if err := json.Unmarshal(raw, resp); err != nil {
		return nil, err
	}
	resp.Scope = &req.SearchScope
	return resp, nil
}

func (s *perEnvDoraService) QueryDoraDeployments(
	context.Context, gen.DoraDeploymentsQueryRequest,
) (*gen.DoraDeploymentsQueryResponse, error) {
	return &gen.DoraDeploymentsQueryResponse{}, nil
}

var threeEnvironments = []service.Environment{
	{Name: "development"},
	{Name: "staging"},
	{Name: "production", IsProduction: true},
}

func doraHandler(t *testing.T, svc *perEnvDoraService, lister EnvironmentLister) *MCPHandler {
	t.Helper()
	h := newTestMCPHandler(t, withDeliveryInsightsService(svc))
	if lister != nil {
		h.WithEnvironmentLister(lister)
	}
	return h
}

func queryDora(
	t *testing.T, h *MCPHandler, environment string, allEnvironments, byEnvironment bool,
) (any, map[string]any) {
	t.Helper()
	result, err := h.QueryDoraMetrics(context.Background(), testNamespace, "", "", environment, "", false,
		allEnvironments, byEnvironment, "", testStartTime, testEndTime, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return result, decoded
}

func notesOf(t *testing.T, decoded map[string]any) string {
	t.Helper()
	notes, _ := decoded["notes"].([]any)
	parts := make([]string, len(notes))
	for i, n := range notes {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, "\n")
}

// classificationsOf maps each of the four metrics to its classification, nil
// where it has none.
func classificationsOf(decoded map[string]any) map[string]any {
	summary, _ := decoded["summary"].(map[string]any)
	out := map[string]any{}
	for _, metric := range []string{"deploymentFrequency", "leadTime", "changeFailureRate", "mttr"} {
		m, _ := summary[metric].(map[string]any)
		out[metric] = m["classification"]
	}
	return out
}

var (
	allClassified = map[string]any{
		"deploymentFrequency": "elite", "leadTime": "elite", "changeFailureRate": "elite", "mttr": "elite",
	}
	noneClassified = map[string]any{
		"deploymentFrequency": nil, "leadTime": nil, "changeFailureRate": nil, "mttr": nil,
	}
)

// TestAnOmittedEnvironmentReportsProduction is the main change here:
// a caller who names no environment gets production, told so, with the
// classification intact because it is production's.
func TestAnOmittedEnvironmentReportsProduction(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"production": 12, "development": 300}}
	_, decoded := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments}), "", false, false)

	assert.Equal(t, []string{"production"}, svc.requests)
	assert.Contains(t, notesOf(t, decoded), `reports the production environment "production"`)
	assert.Equal(t, allClassified, classificationsOf(decoded), "production keeps every classification")
	scope, _ := decoded["scope"].(map[string]any)
	assert.Equal(t, "production", scope["environment"], "the scope echoes the environment reported")
}

// TestANamedEnvironmentIsUnchanged pins that naming an environment answers
// exactly as before: the REST response, no notes, no environment lookup.
func TestANamedEnvironmentIsUnchanged(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"staging": 5}}
	lister := &fakeEnvLister{envs: threeEnvironments}
	result, decoded := queryDora(t, doraHandler(t, svc, lister), "staging", false, false)

	assert.IsType(t, &gen.DoraMetricsQueryResponse{}, result)
	assert.NotContains(t, decoded, "notes")
	assert.Zero(t, lister.calls)
}

func TestAllEnvironmentsCombinesWithoutAClassification(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"": 312}}
	lister := &fakeEnvLister{envs: threeEnvironments}
	_, decoded := queryDora(t, doraHandler(t, svc, lister), "", true, false)

	assert.Equal(t, []string{""}, svc.requests)
	assert.Zero(t, lister.calls, "a combined answer needs no environment list")
	assert.Equal(t, noneClassified, classificationsOf(decoded),
		"DORA classifications describe production, not a mix")
	assert.Contains(t, notesOf(t, decoded), "counts once per environment")
}

func TestByEnvironmentReportsEachAndNamesTheEmptyOnes(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"development": 300, "production": 12}}
	result, decoded := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments}), "", false, true)

	byEnv, ok := result.(doraMetricsByEnvironmentResult)
	require.True(t, ok, "got %T", result)
	require.Len(t, byEnv.Environments, 3)
	assert.Equal(t, "production", byEnv.Environments[2].Environment)
	assert.True(t, byEnv.Environments[2].IsProduction)
	assert.False(t, byEnv.Environments[1].HasData, "staging deployed nothing")
	notes := notesOf(t, decoded)
	assert.Contains(t, notes, `No delivery data on this observer for "staging"`)
	assert.Contains(t, notes, "another observability plane")
}

// TestSeveralProductionEnvironmentsAreReportedSeparately pins that two
// production environments are not combined: deployment counts would add up, but
// lead time and MTTR are percentiles, which do not.
func TestSeveralProductionEnvironmentsAreReportedSeparately(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"prod-us": 10, "prod-eu": 4}}
	lister := &fakeEnvLister{envs: []service.Environment{
		{Name: "development"}, {Name: "prod-us", IsProduction: true}, {Name: "prod-eu", IsProduction: true},
	}}
	result, decoded := queryDora(t, doraHandler(t, svc, lister), "", false, false)

	byEnv, ok := result.(doraMetricsByEnvironmentResult)
	require.True(t, ok, "got %T", result)
	assert.Equal(t, []string{"prod-us", "prod-eu"}, svc.requests, "only the production environments")
	assert.Len(t, byEnv.Environments, 2)
	assert.Contains(t, notesOf(t, decoded), "2 environments are marked as production (prod-us, prod-eu)")
}

func TestNoProductionEnvironmentCombinesAndSaysWhy(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"": 40}}
	lister := &fakeEnvLister{envs: []service.Environment{{Name: "development"}, {Name: "staging"}}}
	_, decoded := queryDora(t, doraHandler(t, svc, lister), "", false, false)

	assert.Equal(t, []string{""}, svc.requests)
	assert.Equal(t, noneClassified, classificationsOf(decoded))
	assert.Contains(t, notesOf(t, decoded), "No environment in this namespace is marked as production")
}

// TestAnEmptyProductionEnvironmentSaysItMayBeElsewhere covers the
// multi-plane case: production exists, but this observer has nothing for it.
func TestAnEmptyProductionEnvironmentSaysItMayBeElsewhere(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"development": 300}}
	_, decoded := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments}), "", false, false)

	assert.Contains(t, notesOf(t, decoded), `No delivery data on this observer for "production"`)
}

// TestAFailedEnvironmentListFallsBackWithoutLeaking pins that a failed lookup
// still answers, and that what it failed with stays in the observer's log.
func TestAFailedEnvironmentListFallsBackWithoutLeaking(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"": 40}}
	lister := &fakeEnvLister{err: errors.New("dial tcp 10.0.0.7:8080: connection refused")}
	_, decoded := queryDora(t, doraHandler(t, svc, lister), "", false, false)

	raw, err := json.Marshal(decoded)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "10.0.0.7")
	assert.Contains(t, notesOf(t, decoded), "could not be listed")
	assert.Equal(t, noneClassified, classificationsOf(decoded))
}

func TestWithoutAListerTheAnswerIsCombined(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"": 40}}
	_, decoded := queryDora(t, doraHandler(t, svc, nil), "", false, false)

	assert.Equal(t, []string{""}, svc.requests)
	assert.Equal(t, noneClassified, classificationsOf(decoded))
}

// TestPerEnvironmentFailuresStayInTheirRow pins that one environment the caller
// may not read, or that fails, does not sink the others -- and that a failure's
// text does not reach the caller.
func TestPerEnvironmentFailuresStayInTheirRow(t *testing.T) {
	svc := &perEnvDoraService{
		deploys: map[string]int{"production": 12},
		errs: map[string]error{
			"development": fmt.Errorf("denied: %w", observerAuthz.ErrAuthzForbidden),
			"staging":     errors.New("sql: database is locked at /data/alerts.db"),
		},
	}
	result, _ := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments}), "", false, true)

	byEnv := result.(doraMetricsByEnvironmentResult)
	assert.Equal(t, "not permitted", byEnv.Environments[0].Error)
	assert.Equal(t, "could not be queried", byEnv.Environments[1].Error)
	assert.True(t, byEnv.Environments[2].HasData)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "alerts.db")
}

// TestAnUnauthenticatedCallerIsNotAnsweredPerEnvironment pins that a failure
// every row would share fails the call, as it does for any other tool.
func TestAnUnauthenticatedCallerIsNotAnsweredPerEnvironment(t *testing.T) {
	svc := &perEnvDoraService{errs: map[string]error{"development": observerAuthz.ErrAuthzUnauthorized}}
	h := doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments})
	_, err := h.QueryDoraMetrics(context.Background(), testNamespace, "", "", "", "", false,
		false, true, "", testStartTime, testEndTime, nil)
	require.ErrorIs(t, err, observerAuthz.ErrAuthzUnauthorized)
}

func TestConflictingEnvironmentArgumentsAreRejected(t *testing.T) {
	h := doraHandler(t, &perEnvDoraService{}, &fakeEnvLister{envs: threeEnvironments})
	for _, tc := range []struct {
		name          string
		environment   string
		all, perEnvir bool
	}{
		{"environment with all_environments", "production", true, false},
		{"environment with by_environment", "production", false, true},
		{"all_environments with by_environment", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.QueryDoraMetrics(context.Background(), testNamespace, "", "", tc.environment, "", false,
				tc.all, tc.perEnvir, "", testStartTime, testEndTime, nil)
			require.Error(t, err)
			var invalid *invalidArgumentError
			assert.True(t, errors.As(err, &invalid), "got %T: %v", err, err)
		})
	}
}

// TestALeadTimeOnlyQueryStillFindsItsData pins that a query for one metric is
// judged on that metric. Asked for lead time alone, the answer carries no
// deployment, failure or recovery counts, and reading only those reported an
// environment with lead time data as having none.
func TestALeadTimeOnlyQueryStillFindsItsData(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"production": 12}}
	h := doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments})

	result, err := h.QueryDoraMetrics(context.Background(), testNamespace, "", "", "", "", false,
		false, false, "", testStartTime, testEndTime, []string{"leadTime"})
	require.NoError(t, err)
	got := result.(doraMetricsResult)
	assert.NotContains(t, strings.Join(got.Notes, "\n"), "No delivery data")

	result, err = h.QueryDoraMetrics(context.Background(), testNamespace, "", "", "", "", false,
		false, true, "", testStartTime, testEndTime, []string{"leadTime"})
	require.NoError(t, err)
	byEnv := result.(doraMetricsByEnvironmentResult)
	assert.False(t, byEnv.Environments[0].HasData, "development has no lead time samples")
	assert.True(t, byEnv.Environments[2].HasData, "production does")
}
