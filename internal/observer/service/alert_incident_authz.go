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

// alertIncidentServiceWithAuthz wraps an AlertIncidentService and adds authorization
// checks for all three operations. Both the HTTP handlers and the MCP handler should
// use this via NewAlertIncidentServiceWithAuthz rather than the individual wrappers.
type alertIncidentServiceWithAuthz struct {
	internal AlertIncidentService
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ AlertIncidentService = (*alertIncidentServiceWithAuthz)(nil)

// NewAlertIncidentServiceWithAuthz wraps the provided AlertIncidentService with
// authorization checks for QueryAlerts, QueryIncidents, and UpdateIncident.
func NewAlertIncidentServiceWithAuthz(s AlertIncidentService, pdp authzcore.PDP, logger *slog.Logger) AlertIncidentService {
	return &alertIncidentServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *alertIncidentServiceWithAuthz) QueryAlerts(ctx context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
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

func (s *alertIncidentServiceWithAuthz) QueryIncidents(ctx context.Context, req gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
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

// UpdateIncident performs a generic permission check (no scope/namespace lookup —
// just verifies the caller has the incidents:update permission) before delegating.
func (s *alertIncidentServiceWithAuthz) UpdateIncident(ctx context.Context, incidentID string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionUpdateIncidents,
		observerAuthz.ResourceTypeNamespace, "", authzcore.ResourceHierarchy{},
	); err != nil {
		return nil, err
	}
	return s.internal.UpdateIncident(ctx, incidentID, req)
}
