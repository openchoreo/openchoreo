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

// alertsServiceWithAuthz wraps an AlertsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via NewAlertsServiceWithAuthz.
// Other services that call AlertsQuerier internally should use the unwrapped service directly.
type alertsServiceWithAuthz struct {
	internal AlertsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ AlertsQuerier = (*alertsServiceWithAuthz)(nil)

// NewAlertsServiceWithAuthz wraps the provided AlertsQuerier with authorization checks.
func NewAlertsServiceWithAuthz(s AlertsQuerier, pdp authzcore.PDP, logger *slog.Logger) AlertsQuerier {
	return &alertsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *alertsServiceWithAuthz) QueryAlerts(ctx context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	scope := req.SearchScope
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
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewAlerts,
		resourceType, resourceName, hierarchy,
	); err != nil {
		return nil, err
	}
	return s.internal.QueryAlerts(ctx, req)
}
