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

// deliveryInsightsServiceWithAuthz wraps an DeliveryInsightsService and checks the deliveryinsights:view
// permission for the requested scope before delegating. Both the HTTP handlers and any
// MCP handler should use this via NewDeliveryInsightsServiceWithAuthz.
type deliveryInsightsServiceWithAuthz struct {
	internal DeliveryInsightsService
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ DeliveryInsightsService = (*deliveryInsightsServiceWithAuthz)(nil)

// NewDeliveryInsightsServiceWithAuthz wraps the provided DeliveryInsightsService with authorization
// checks for both query operations.
func NewDeliveryInsightsServiceWithAuthz(s DeliveryInsightsService, pdp authzcore.PDP, logger *slog.Logger) DeliveryInsightsService {
	return &deliveryInsightsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *deliveryInsightsServiceWithAuthz) QueryDoraMetrics(
	ctx context.Context, req gen.DoraMetricsQueryRequest,
) (*gen.DoraMetricsQueryResponse, error) {
	if err := s.checkScope(ctx, req.SearchScope); err != nil {
		return nil, err
	}
	return s.internal.QueryDoraMetrics(ctx, req)
}

func (s *deliveryInsightsServiceWithAuthz) QueryDoraDeployments(
	ctx context.Context, req gen.DoraDeploymentsQueryRequest,
) (*gen.DoraDeploymentsQueryResponse, error) {
	if err := s.checkScope(ctx, req.SearchScope); err != nil {
		return nil, err
	}
	return s.internal.QueryDoraDeployments(ctx, req)
}

func (s *deliveryInsightsServiceWithAuthz) checkScope(ctx context.Context, scope gen.ComponentSearchScope) error {
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
		observerAuthz.ActionViewDeliveryInsights,
		resourceType, resourceName, hierarchy,
		authzcore.Context{},
	)
}
