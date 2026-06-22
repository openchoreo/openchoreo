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

// runsServiceWithAuthz wraps a RunsQuerier and adds authorization checks. Runs/retries
// reuse the events authz pathway since they are derived from the same Kubernetes events
// stream (an event-view permission grants visibility into run/retry derivations of those
// events).
type runsServiceWithAuthz struct {
	internal RunsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ RunsQuerier = (*runsServiceWithAuthz)(nil)

// NewRunsServiceWithAuthz wraps the provided RunsQuerier with authorization checks.
func NewRunsServiceWithAuthz(s RunsQuerier, pdp authzcore.PDP, logger *slog.Logger) RunsQuerier {
	return &runsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *runsServiceWithAuthz) QueryRuns(
	ctx context.Context,
	req *types.RunsQueryRequest,
) (*types.RunsQueryResponse, error) {
	var scope *types.ComponentSearchScope
	if req != nil {
		scope = req.SearchScope
	}
	if err := s.authorize(ctx, scope); err != nil {
		return nil, err
	}
	return s.internal.QueryRuns(ctx, req)
}

func (s *runsServiceWithAuthz) QueryRetries(
	ctx context.Context,
	jobName string,
	req *types.RetriesQueryRequest,
) (*types.RetriesQueryResponse, error) {
	var scope *types.ComponentSearchScope
	if req != nil {
		scope = req.SearchScope
	}
	if err := s.authorize(ctx, scope); err != nil {
		return nil, err
	}
	return s.internal.QueryRetries(ctx, jobName, req)
}

func (s *runsServiceWithAuthz) authorize(ctx context.Context, scope *types.ComponentSearchScope) error {
	resourceType, resourceName, hierarchy, err := observerAuthz.RunsScopeAuthz(scope)
	if err != nil {
		return err
	}
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	authzCtx := authzcore.Context{}
	if scope != nil {
		authzCtx.Resource = authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}
	}
	return observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewEvents,
		resourceType, resourceName, hierarchy,
		authzCtx,
	)
}
