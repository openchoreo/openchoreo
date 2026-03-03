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

// tracesServiceWithAuthz wraps a TracesQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via NewTracesServiceWithAuthz.
// Other services that call TracesQuerier internally should use the unwrapped service directly.
type tracesServiceWithAuthz struct {
	internal TracesQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ TracesQuerier = (*tracesServiceWithAuthz)(nil)

// NewTracesServiceWithAuthz wraps the provided TracesQuerier with authorization checks.
func NewTracesServiceWithAuthz(s TracesQuerier, pdp authzcore.PDP, logger *slog.Logger) TracesQuerier {
	return &tracesServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *tracesServiceWithAuthz) QueryTraces(ctx context.Context, req *types.TracesQueryRequest) (*types.TracesQueryResponse, error) {
	scope := req.SearchScope
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(scope.Namespace, scope.Project, scope.Component)
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewTraces,
		resourceType, resourceName, hierarchy,
	); err != nil {
		return nil, err
	}
	return s.internal.QueryTraces(ctx, req)
}

func (s *tracesServiceWithAuthz) QuerySpans(ctx context.Context, traceID string, req *types.TracesQueryRequest) (*types.SpansQueryResponse, error) {
	scope := req.SearchScope
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(scope.Namespace, scope.Project, scope.Component)
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewTraces,
		resourceType, resourceName, hierarchy,
	); err != nil {
		return nil, err
	}
	return s.internal.QuerySpans(ctx, traceID, req)
}

// GetSpanDetails passes through without an authz check — traceID+spanID alone do not
// carry enough scope context (namespace/project/component) to evaluate authorization.
func (s *tracesServiceWithAuthz) GetSpanDetails(ctx context.Context, traceID string, spanID string) (*types.SpanInfo, error) {
	return s.internal.GetSpanDetails(ctx, traceID, spanID)
}
