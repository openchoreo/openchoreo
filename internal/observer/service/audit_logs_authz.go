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

// auditLogsServiceWithAuthz wraps an AuditLogsQuerier and adds authorization
// checks. Both the HTTP handlers and any future MCP handler should use this via
// NewAuditLogsServiceWithAuthz.
type auditLogsServiceWithAuthz struct {
	internal AuditLogsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ AuditLogsQuerier = (*auditLogsServiceWithAuthz)(nil)

// NewAuditLogsServiceWithAuthz wraps the provided AuditLogsQuerier with
// authorization checks.
func NewAuditLogsServiceWithAuthz(
	s AuditLogsQuerier, pdp authzcore.PDP, logger *slog.Logger,
) AuditLogsQuerier {
	return &auditLogsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *auditLogsServiceWithAuthz) QueryAuditLogs(
	ctx context.Context,
	req *types.AuditLogsQueryRequest,
) (*types.AuditLogsResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	return s.internal.QueryAuditLogs(ctx, req)
}

// QueryAuditLogFilterValues carries the same check as QueryAuditLogs, not a
// weaker one: it enumerates the actors, resource names and source addresses in
// the trail, which discloses the trail whether or not the records come with it.
// Gating only the record read would leave this as a way around it.
func (s *auditLogsServiceWithAuthz) QueryAuditLogFilterValues(
	ctx context.Context,
	req *types.AuditLogFilterValuesRequest,
) (*types.AuditLogFilterValuesResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	return s.internal.QueryAuditLogFilterValues(ctx, req)
}

// authorize is shared by both reads, so neither can drift into a weaker check
// than the other.
//
// An empty hierarchy is the cluster scope: resourceHierarchyToPath maps it to
// "*", which only a cluster-scoped binding can satisfy. Deliberately not derived
// from the request — an audit query's actor and resource fields are filters, not
// a scope. Deriving a hierarchy from them would let a namespace-scoped binding
// read that namespace's audit trail, which AX-4 exists to prevent: working in a
// project does not confer the right to read its audit trail.
func (s *auditLogsServiceWithAuthz) authorize(ctx context.Context) error {
	return observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewAuditLogs,
		observerAuthz.ResourceTypeAudit, "", authzcore.ResourceHierarchy{},
		authzcore.Context{},
	)
}
