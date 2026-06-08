// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// eventsServiceWithAuthz wraps an EventsQuerier and adds authorization checks.
type eventsServiceWithAuthz struct {
	internal EventsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ EventsQuerier = (*eventsServiceWithAuthz)(nil)

// NewEventsServiceWithAuthz wraps the provided EventsQuerier with authorization checks.
func NewEventsServiceWithAuthz(s EventsQuerier, pdp authzcore.PDP, logger *slog.Logger) EventsQuerier {
	return &eventsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *eventsServiceWithAuthz) QueryEvents(
	ctx context.Context,
	req *types.EventsQueryRequest,
) (*types.EventsQueryResponse, error) {
	resourceType, resourceName, hierarchy, err := observerAuthz.EventsScopeAuthz(req)
	if err != nil {
		return nil, err
	}
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	authzCtx := authzcore.Context{}
	if req.SearchScope != nil && req.SearchScope.Component != nil {
		scope := req.SearchScope.Component
		authzCtx.Resource = authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}
	}
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewEvents,
		resourceType, resourceName, hierarchy,
		authzCtx,
	); err != nil {
		return nil, err
	}
	return s.internal.QueryEvents(ctx, req)
}
