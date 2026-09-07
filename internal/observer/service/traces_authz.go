// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/labels"
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
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewTraces,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryTraces(ctx, req)
}

func (s *tracesServiceWithAuthz) QuerySpans(ctx context.Context, traceID string, req *types.TracesQueryRequest) (*types.SpansQueryResponse, error) {
	scope := req.SearchScope
	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(scope.Namespace, scope.Project, scope.Component)
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewTraces,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(scope.Namespace, scope.Environment, false),
		}},
	); err != nil {
		return nil, err
	}
	return s.internal.QuerySpans(ctx, traceID, req)
}

// GetSpanDetails authorizes on the return path: the request carries only traceID+spanID,
// so the scope is derived from the fetched span's resource attributes and the span is
// discarded if the caller is not authorized for it.
func (s *tracesServiceWithAuthz) GetSpanDetails(ctx context.Context, traceID string, spanID string) (*types.SpanInfo, error) {
	span, err := s.internal.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		return nil, err
	}

	namespace := resourceAttrString(span.ResourceAttributes, labels.NamespaceName)
	project := resourceAttrString(span.ResourceAttributes, labels.ProjectName)
	component := resourceAttrString(span.ResourceAttributes, labels.ComponentName)
	environment := resourceAttrString(span.ResourceAttributes, labels.EnvironmentName)

	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(namespace, project, component)
	// TODO: currently the obs API is not equipped to provide cluster level environments,
	// once that is done update false to proper isClusterScoped value.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewTraces,
		resourceType, resourceName, hierarchy,
		authzcore.Context{Resource: authzcore.ResourceAttribute{
			Environment: observerAuthz.FormatDualScopedResourceName(namespace, environment, false),
		}},
	); err != nil {
		return nil, err
	}
	return span, nil
}

// resourceAttrString returns the string value for key, or "" if absent or not a string.
func resourceAttrString(attrs map[string]interface{}, key string) string {
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}
