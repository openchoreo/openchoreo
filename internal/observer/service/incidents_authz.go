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

// incidentsServiceWithAuthz wraps an IncidentsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via NewIncidentsServiceWithAuthz.
// Other services that call IncidentsQuerier internally should use the unwrapped service directly.
type incidentsServiceWithAuthz struct {
	internal IncidentsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ IncidentsQuerier = (*incidentsServiceWithAuthz)(nil)

// NewIncidentsServiceWithAuthz wraps the provided IncidentsQuerier with authorization checks.
func NewIncidentsServiceWithAuthz(s IncidentsQuerier, pdp authzcore.PDP, logger *slog.Logger) IncidentsQuerier {
	return &incidentsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *incidentsServiceWithAuthz) QueryIncidents(ctx context.Context, req gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
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
		observerAuthz.ActionViewIncidents,
		resourceType, resourceName, hierarchy,
	); err != nil {
		return nil, err
	}
	return s.internal.QueryIncidents(ctx, req)
}
