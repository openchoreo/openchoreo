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
		authzcore.Context{},
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
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryIncidents(ctx, req)
}

// IncidentScope is a pass-through: it is the read this wrapper uses to
// authorize UpdateIncident, so authorizing it here would be circular. No
// handler calls it.
func (s *alertIncidentServiceWithAuthz) IncidentScope(
	ctx context.Context, incidentID string,
) (string, string, string, error) {
	return s.internal.IncidentScope(ctx, incidentID)
}

// UpdateIncident authorizes against the incident's own
// namespace/project/component before delegating. The hierarchy is read from
// the stored incident, because IncidentPutRequest names no scope, and must be
// named precisely: an empty ResourceHierarchy{} resolves to the "*" wildcard
// path, which under resourceMatch's prefix semantics only a cluster-wide
// grant would match — silently denying a namespace- or project-scoped
// incidents:update grant. It also gives the audit event its hierarchy, since
// CheckAuthorization records what it authorized on.
func (s *alertIncidentServiceWithAuthz) UpdateIncident(ctx context.Context, incidentID string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	namespace, project, component, err := s.internal.IncidentScope(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	resourceType, resourceName, hierarchy := observerAuthz.ComponentScopeAuthz(namespace, project, component)
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionUpdateIncidents,
		resourceType, resourceName, hierarchy,
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.UpdateIncident(ctx, incidentID, req)
}
