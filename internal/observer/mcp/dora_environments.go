// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// The DORA tools have to decide what an omitted environment means, because the
// REST answer -- every environment of the scope combined -- is rarely what a
// caller asking about delivery performance wants. Combined, a commit promoted
// through three environments counts as three deployments, lead time mixes
// development's minutes with production's days, and the Elite/High/Medium/Low
// classification, which DORA defines for production, describes the mix. The
// portal never asks for it; an agent asked "how are we doing?" naturally would.
//
// So on MCP an omitted environment means production: the namespace's
// environment marked isProduction. The combined figure stays available, asked
// for explicitly, with its classification removed. Every answer that is not
// simply "the environment you named" says what it covers in notes, because an
// agent passes a note on and cannot see what the numbers leave out.
//
// The environment list is read with the Observer's own credentials, so the
// caller is authorized for the scope before it is read. The authz check covers
// namespace, project and component but not environment, so a caller allowed the
// scope sees every environment's name in a per-environment answer -- including,
// for a project or component viewer, environments their project does not use.

// EnvironmentLister lists the environments of a namespace. The UID resolver
// implements it; nil leaves the DORA tools combining environments as REST does.
type EnvironmentLister interface {
	ListEnvironments(ctx context.Context, namespace string) ([]service.Environment, error)
}

// WithEnvironmentLister lets the DORA tools default to the production
// environment and report per environment. The environments are listed with the
// Observer's own credentials, so this needs a delivery insights service that can
// authorize the caller first. Without one it fails closed: it logs why and leaves
// the tools combining environments, which never lists them.
func (h *MCPHandler) WithEnvironmentLister(l EnvironmentLister) *MCPHandler {
	a, ok := h.deliveryInsightsService.(scopeAuthorizer)
	if !ok {
		h.logger.Error("The delivery insights service can't authorize a scope on its own, so the DORA "+
			"tools will combine environments instead of defaulting to production",
			"service", fmt.Sprintf("%T", h.deliveryInsightsService))
		return h
	}
	h.environments, h.authorizer = l, a
	return h
}

// scopeAuthorizer checks deliveryinsights:view for a scope without querying. The
// authz-wrapped service implements it.
type scopeAuthorizer interface {
	AuthorizeScope(ctx context.Context, scope gen.ComponentSearchScope) error
}

// doraByEnvironmentMax bounds a per-environment answer: each row is its own
// authz check and store query, run one after another.
const doraByEnvironmentMax = 50

// doraMetricsResult is the REST response with notes alongside it. The response
// is embedded so its fields stay where existing callers read them.
type doraMetricsResult struct {
	*gen.DoraMetricsQueryResponse
	Notes []string `json:"notes,omitempty"`
}

// doraEnvironmentSummary is one environment's headline figures in a
// per-environment answer.
type doraEnvironmentSummary struct {
	Environment      string `json:"environment"`
	IsProduction     bool   `json:"isProduction"`
	HasData          bool   `json:"hasData"`
	Summary          any    `json:"summary,omitempty"`
	DataAvailability any    `json:"dataAvailability,omitempty"`
	// Error is set instead of the figures when this environment could not be
	// queried, so one environment the caller may not read does not fail the rest.
	Error string `json:"error,omitempty"`
}

// doraMetricsByEnvironmentResult answers with one summary per environment. Series
// are left out to keep the answer readable; one environment's series come from
// asking for that environment.
type doraMetricsByEnvironmentResult struct {
	Environments []doraEnvironmentSummary `json:"environments"`
	Notes        []string                 `json:"notes,omitempty"`
}

const (
	noteSeriesOmitted = "Series are omitted from a per-environment answer; pass environment " +
		"for one environment's series."
	noteCombined = "Combined across every environment this observer has data for: a commit " +
		"promoted through several environments counts once per environment, and lead time, change " +
		"failure rate and MTTR mix them. DORA classifications are defined for production, so they " +
		"are omitted here. Environments served by another observability plane are not included."
	noteNoData = "No delivery data on this observer for %s. It may be served by another " +
		"observability plane's observer, or nothing was deployed there in this window."
	noteProductionClassifications = "Classifications are given for production environments only: " +
		"DORA defines its Elite/High/Medium/Low bands for production."
	noteTruncated = "The namespace has %d environments; only %d are reported, production " +
		"environments first. Pass environment for any other."
)

// doraMetricsForEnvironments answers a query that named no environment.
func (h *MCPHandler) doraMetricsForEnvironments(
	ctx context.Context, req gen.DoraMetricsQueryRequest, allEnvironments, byEnvironment bool,
) (any, error) {
	if allEnvironments {
		return h.combinedDoraMetrics(ctx, req)
	}
	if h.environments == nil {
		return h.combinedDoraMetrics(ctx, req)
	}
	if err := h.authorizer.AuthorizeScope(ctx, req.SearchScope); err != nil {
		return nil, err
	}
	envs, err := h.environments.ListEnvironments(ctx, req.SearchScope.Namespace)
	if err != nil {
		h.logger.Warn("Could not list environments for a DORA query; combining them instead",
			"namespace", req.SearchScope.Namespace, "error", err)
		result, combineErr := h.combinedDoraMetrics(ctx, req)
		if combineErr != nil {
			return nil, combineErr
		}
		why := "so no production environment could be chosen."
		if byEnvironment {
			why = "so this is every environment combined, not one summary per environment."
		}
		result.Notes = append([]string{"The namespace's environments could not be listed, " + why},
			result.Notes...)
		return result, nil
	}
	if len(envs) == 0 {
		return h.combinedDoraMetrics(ctx, req)
	}

	if byEnvironment {
		return h.doraMetricsByEnvironment(ctx, req, envs, nil)
	}

	var production []service.Environment
	for _, e := range envs {
		if e.IsProduction {
			production = append(production, e)
		}
	}
	switch len(production) {
	case 0:
		result, err := h.combinedDoraMetrics(ctx, req)
		if err != nil {
			return nil, err
		}
		result.Notes = append([]string{"No environment in this namespace is marked as production, " +
			"so there is no production environment to default to. Pass environment to report on one."},
			result.Notes...)
		return result, nil
	case 1:
		return h.productionDoraMetrics(ctx, req, production[0].Name)
	default:
		names := make([]string, len(production))
		for i, e := range production {
			names[i] = e.Name
		}
		return h.doraMetricsByEnvironment(ctx, req, production, []string{fmt.Sprintf(
			"No environment was given, and %d environments are marked as production (%s). They are "+
				"reported separately: lead time and MTTR are percentiles, which cannot be combined "+
				"exactly across environments.", len(production), strings.Join(names, ", "))})
	}
}

// productionDoraMetrics reports on the one production environment, saying it chose it.
func (h *MCPHandler) productionDoraMetrics(
	ctx context.Context, req gen.DoraMetricsQueryRequest, environment string,
) (any, error) {
	req.SearchScope.Environment = &environment
	resp, err := h.deliveryInsightsService.QueryDoraMetrics(ctx, req)
	if err != nil {
		return nil, err
	}
	notes := []string{fmt.Sprintf("No environment was given, so this reports the production "+
		"environment %q. Pass environment for another one, by_environment for one summary per "+
		"environment, or all_environments for every environment combined.", environment)}
	if !doraHasData(resp) {
		notes = append(notes, fmt.Sprintf(noteNoData, fmt.Sprintf("%q", environment)))
	}
	return doraMetricsResult{DoraMetricsQueryResponse: resp, Notes: notes}, nil
}

// combinedDoraMetrics is the REST answer for an omitted environment, without the
// classifications that do not apply to it.
func (h *MCPHandler) combinedDoraMetrics(
	ctx context.Context, req gen.DoraMetricsQueryRequest,
) (doraMetricsResult, error) {
	req.SearchScope.Environment = nil
	resp, err := h.deliveryInsightsService.QueryDoraMetrics(ctx, req)
	if err != nil {
		return doraMetricsResult{}, err
	}
	stripClassifications(resp)
	return doraMetricsResult{DoraMetricsQueryResponse: resp, Notes: []string{noteCombined}}, nil
}

// doraMetricsByEnvironment queries each environment in turn. An environment that
// fails is reported in its row rather than failing the answer. A failure every
// row would share fails the call: not signed in, authz unreachable, or refused
// for every environment -- an answer of nothing but "not permitted" rows would
// still list the environments to a caller allowed none of them.
func (h *MCPHandler) doraMetricsByEnvironment(
	ctx context.Context, req gen.DoraMetricsQueryRequest, envs []service.Environment, notes []string,
) (any, error) {
	if len(envs) > doraByEnvironmentMax {
		notes = append(notes, fmt.Sprintf(noteTruncated, len(envs), doraByEnvironmentMax))
		envs = capEnvironments(envs, doraByEnvironmentMax)
	}
	rows := make([]doraEnvironmentSummary, 0, len(envs))
	var empty []string
	forbidden, stripped := 0, false
	var forbiddenErr error
	for _, e := range envs {
		name := e.Name
		envReq := req
		envReq.SearchScope.Environment = &name
		row := doraEnvironmentSummary{Environment: name, IsProduction: e.IsProduction}

		resp, err := h.deliveryInsightsService.QueryDoraMetrics(ctx, envReq)
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized),
			errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return nil, err
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			forbidden++
			forbiddenErr = err
			row.Error = "not permitted"
		case err != nil:
			// The error itself stays in the observer's log: what it says is
			// internal, and the caller can do nothing with it but retry.
			h.logger.Warn("Per-environment DORA query failed", "environment", name, "error", err)
			row.Error = "could not be queried"
		default:
			if !e.IsProduction {
				stripClassifications(resp)
				stripped = true
			}
			row.HasData = doraHasData(resp)
			row.Summary = resp.Summary
			row.DataAvailability = resp.DataAvailability
			if !row.HasData {
				empty = append(empty, fmt.Sprintf("%q", name))
			}
		}
		rows = append(rows, row)
	}

	if forbidden == len(rows) && forbidden > 0 {
		return nil, forbiddenErr
	}

	notes = append(notes, noteSeriesOmitted)
	if stripped {
		notes = append(notes, noteProductionClassifications)
	}
	if len(empty) > 0 {
		notes = append(notes, fmt.Sprintf(noteNoData, strings.Join(empty, ", ")))
	}
	return doraMetricsByEnvironmentResult{Environments: rows, Notes: notes}, nil
}

// capEnvironments keeps max environments, production ones first so a cut never
// drops production for a development environment that sorted earlier. Those kept
// stay in listing order.
func capEnvironments(envs []service.Environment, maxEnvs int) []service.Environment {
	keep := make([]bool, len(envs))
	kept := 0
	for _, production := range []bool{true, false} {
		for i, e := range envs {
			if kept < maxEnvs && !keep[i] && e.IsProduction == production {
				keep[i] = true
				kept++
			}
		}
	}
	out := make([]service.Environment, 0, kept)
	for i, e := range envs {
		if keep[i] {
			out = append(out, e)
		}
	}
	return out
}

// doraHasData reports whether any requested metric counted anything. Lead time
// has no count, so it is judged by its median, which is set only when there was
// a sample.
func doraHasData(resp *gen.DoraMetricsQueryResponse) bool {
	if resp == nil || resp.Summary == nil {
		return false
	}
	s := resp.Summary
	if s.DeploymentFrequency != nil && s.DeploymentFrequency.Total != nil && *s.DeploymentFrequency.Total > 0 {
		return true
	}
	if s.ChangeFailureRate != nil && s.ChangeFailureRate.Total != nil && *s.ChangeFailureRate.Total > 0 {
		return true
	}
	if s.Mttr != nil && s.Mttr.Recoveries != nil && *s.Mttr.Recoveries > 0 {
		return true
	}
	if s.LeadTime != nil && s.LeadTime.P50Ms != nil {
		return true
	}
	return false
}

// stripClassifications removes the DORA classification from every summary metric.
func stripClassifications(resp *gen.DoraMetricsQueryResponse) {
	if resp == nil || resp.Summary == nil {
		return
	}
	s := resp.Summary
	if s.DeploymentFrequency != nil {
		s.DeploymentFrequency.Classification = nil
	}
	if s.LeadTime != nil {
		s.LeadTime.Classification = nil
	}
	if s.ChangeFailureRate != nil {
		s.ChangeFailureRate.Classification = nil
	}
	if s.Mttr != nil {
		s.Mttr.Classification = nil
	}
}
