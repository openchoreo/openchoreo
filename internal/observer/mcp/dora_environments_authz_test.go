// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// TestTheAuthzWrapperCanBeAskedFirst pins that the service main.go hands the MCP
// handler can authorize a scope on its own. Without it the environment list
// would be read before the caller was checked.
func TestTheAuthzWrapperCanBeAskedFirst(t *testing.T) {
	svc := service.NewDeliveryInsightsServiceWithAuthz(&perEnvDoraService{}, nil, slog.Default())
	_, ok := svc.(scopeAuthorizer)
	assert.True(t, ok, "%T does not implement AuthorizeScope", svc)
}

// TestACallerRefusedTheScopeIsRefusedBeforeListing pins that a caller without
// deliveryinsights:view is refused before the Observer lists the namespace's
// environments with its own credentials, so neither the names nor the
// control-plane calls are spent on them.
func TestACallerRefusedTheScopeIsRefusedBeforeListing(t *testing.T) {
	for _, tc := range []struct {
		name          string
		byEnvironment bool
	}{{"production default", false}, {"by_environment", true}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &perEnvDoraService{authzErr: fmt.Errorf("denied: %w", observerAuthz.ErrAuthzForbidden)}
			lister := &fakeEnvLister{envs: threeEnvironments}
			result, err := doraHandler(t, svc, lister).QueryDoraMetrics(context.Background(), testNamespace,
				"", "", "", "", false, false, tc.byEnvironment, "", testStartTime, testEndTime, nil)

			require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
			assert.Nil(t, result)
			assert.Equal(t, 0, lister.calls, "the environments were listed for a refused caller")
			assert.Empty(t, svc.requests)
		})
	}
}

// TestEveryEnvironmentForbiddenFailsTheCall pins the second line of defense:
// rows that are all "not permitted" are a refusal, not an answer listing every
// environment. Today's authz gives every environment the same verdict, so this
// is what a refused caller would get if nothing was asked first.
func TestEveryEnvironmentForbiddenFailsTheCall(t *testing.T) {
	forbidden := fmt.Errorf("denied: %w", observerAuthz.ErrAuthzForbidden)
	for _, tc := range []struct {
		name          string
		envs          []service.Environment
		byEnvironment bool
	}{
		{"by_environment", threeEnvironments, true},
		{"several production environments", []service.Environment{
			{Name: "prod-us", IsProduction: true}, {Name: "prod-eu", IsProduction: true},
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := map[string]error{}
			for _, e := range tc.envs {
				errs[e.Name] = forbidden
			}
			svc := &perEnvDoraService{errs: errs}
			result, err := doraHandler(t, svc, &fakeEnvLister{envs: tc.envs}).QueryDoraMetrics(
				context.Background(), testNamespace, "", "", "", "", false, false, tc.byEnvironment, "",
				testStartTime, testEndTime, nil)

			require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
			assert.Nil(t, result)
		})
	}
}

// TestNonProductionRowsCarryNoClassification pins that a per-environment answer
// classifies only production, as the combined answer classifies nothing: the
// DORA bands describe production.
func TestNonProductionRowsCarryNoClassification(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"development": 300, "staging": 30, "production": 12}}
	_, decoded := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: threeEnvironments}), "", false, true)

	rows, _ := decoded["environments"].([]any)
	require.Len(t, rows, 3)
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row["isProduction"] == true {
			assert.Equal(t, allClassified, classificationsOf(row), "%v", row["environment"])
		} else {
			assert.Equal(t, noneClassified, classificationsOf(row), "%v", row["environment"])
		}
	}
	assert.Contains(t, notesOf(t, decoded), "production environments only")
}

// TestAPerEnvironmentAnswerIsCapped pins that a namespace with very many
// environments does not turn one tool call into as many queries, and that the
// answer says what it left out.
func TestAPerEnvironmentAnswerIsCapped(t *testing.T) {
	envs := make([]service.Environment, doraByEnvironmentMax+10)
	for i := range envs {
		envs[i] = service.Environment{Name: fmt.Sprintf("env-%02d", i)}
	}
	svc := &perEnvDoraService{}
	result, decoded := queryDora(t, doraHandler(t, svc, &fakeEnvLister{envs: envs}), "", false, true)

	assert.Len(t, result.(doraMetricsByEnvironmentResult).Environments, doraByEnvironmentMax)
	assert.Len(t, svc.requests, doraByEnvironmentMax)
	assert.Contains(t, notesOf(t, decoded),
		fmt.Sprintf("has %d environments; only the first %d", len(envs), doraByEnvironmentMax))
}

// TestAFailedListingSaysWhatByEnvironmentGotInstead pins that a caller who asked
// for one summary per environment is told the answer is combined instead, not
// that a production environment could not be chosen, which they did not ask for.
func TestAFailedListingSaysWhatByEnvironmentGotInstead(t *testing.T) {
	svc := &perEnvDoraService{deploys: map[string]int{"": 40}}
	lister := &fakeEnvLister{err: errors.New("connection refused")}
	_, decoded := queryDora(t, doraHandler(t, svc, lister), "", false, true)

	notes := notesOf(t, decoded)
	assert.Contains(t, notes, "not one summary per environment")
	assert.NotContains(t, notes, "no production environment could be chosen")
	assert.Equal(t, noneClassified, classificationsOf(decoded))
}
