// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// metricsServiceWithAuthz wraps a MetricsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via NewMetricsServiceWithAuthz.
// Other services that call MetricsQuerier internally should use the unwrapped service directly.
type metricsServiceWithAuthz struct {
	internal MetricsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ MetricsQuerier = (*metricsServiceWithAuthz)(nil)

// NewMetricsServiceWithAuthz wraps the provided MetricsQuerier with authorization checks.
func NewMetricsServiceWithAuthz(s MetricsQuerier, pdp authzcore.PDP, logger *slog.Logger) MetricsQuerier {
	return &metricsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *metricsServiceWithAuthz) QueryMetrics(ctx context.Context, req *types.MetricsQueryRequest) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("metrics query request is required")
	}
	scope := req.SearchScope
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(scope.Namespace, scope.Project, scope.Component)
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewMetrics,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryMetrics(ctx, req)
}

func (s *metricsServiceWithAuthz) QueryRuntimeTopology(
	ctx context.Context,
	req *types.RuntimeTopologyRequest,
) (*types.RuntimeTopologyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("runtime topology request is required")
	}
	scope := req.SearchScope
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(
		scope.Namespace, scope.Project, scope.Component,
	)
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewMetrics,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryRuntimeTopology(ctx, req)
}
