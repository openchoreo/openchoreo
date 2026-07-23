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

// finopsServiceWithAuthz wraps a FinOpsQuerier and adds authorization checks.
// Both the HTTP handlers and any other entrypoint should use this via
// NewFinOpsServiceWithAuthz rather than the bare adapter.
type finopsServiceWithAuthz struct {
	internal FinOpsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ FinOpsQuerier = (*finopsServiceWithAuthz)(nil)

// NewFinOpsServiceWithAuthz wraps the provided FinOpsQuerier with authorization checks.
func NewFinOpsServiceWithAuthz(s FinOpsQuerier, pdp authzcore.PDP, logger *slog.Logger) FinOpsQuerier {
	return &finopsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *finopsServiceWithAuthz) GetComponentCosts(ctx context.Context, req *types.CostQueryRequest) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("FinOps cost query request is required")
	}
	if err := s.authorize(ctx, req.Namespace, req.Project, req.Component, req.Environment); err != nil {
		return nil, err
	}
	return s.internal.GetComponentCosts(ctx, req)
}

func (s *finopsServiceWithAuthz) GetRecommendations(
	ctx context.Context,
	req *types.RecommendationQueryRequest,
) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("FinOps recommendation query request is required")
	}
	if err := s.authorize(ctx, req.Namespace, req.Project, req.Component, req.Environment); err != nil {
		return nil, err
	}
	return s.internal.GetRecommendations(ctx, req)
}

func (s *finopsServiceWithAuthz) authorize(ctx context.Context, namespace, project, component, environment string) error {
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(namespace, project, component)
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	return observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewFinOps,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(namespace, environment, false),
		}},
	)
}
