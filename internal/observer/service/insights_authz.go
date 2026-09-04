// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"
	"strings"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
)

// insightsServiceWithAuthz wraps an InsightsService and checks the insights:view
// permission for the requested scope before delegating. Both the HTTP handlers and any
// MCP handler should use this via NewInsightsServiceWithAuthz.
type insightsServiceWithAuthz struct {
	internal InsightsService
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ InsightsService = (*insightsServiceWithAuthz)(nil)

// NewInsightsServiceWithAuthz wraps the provided InsightsService with authorization
// checks for both query operations.
func NewInsightsServiceWithAuthz(s InsightsService, pdp authzcore.PDP, logger *slog.Logger) InsightsService {
	return &insightsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *insightsServiceWithAuthz) QueryDoraMetrics(
	ctx context.Context, req gen.DoraMetricsQueryRequest,
) (*gen.DoraMetricsQueryResponse, error) {
	if err := s.checkScope(ctx, req.SearchScope); err != nil {
		return nil, err
	}
	return s.internal.QueryDoraMetrics(ctx, req)
}

func (s *insightsServiceWithAuthz) QueryDoraDeployments(
	ctx context.Context, req gen.DoraDeploymentsQueryRequest,
) (*gen.DoraDeploymentsQueryResponse, error) {
	if err := s.checkScope(ctx, req.SearchScope); err != nil {
		return nil, err
	}
	return s.internal.QueryDoraDeployments(ctx, req)
}

func (s *insightsServiceWithAuthz) checkScope(ctx context.Context, scope gen.ComponentSearchScope) error {
	project := ""
	if scope.Project != nil {
		project = strings.TrimSpace(*scope.Project)
	}
	component := ""
	if scope.Component != nil {
		component = strings.TrimSpace(*scope.Component)
	}
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(
		scope.Namespace, project, component,
	)
	return observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewInsights,
		resourceType, resourceName, hierarchy,
		authzcore.Context{},
	)
}
