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

// platformLogsServiceWithAuthz wraps a PlatformLogsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via
// NewPlatformLogsServiceWithAuthz.
type platformLogsServiceWithAuthz struct {
	internal PlatformLogsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ PlatformLogsQuerier = (*platformLogsServiceWithAuthz)(nil)

// NewPlatformLogsServiceWithAuthz wraps the provided PlatformLogsQuerier with
// authorization checks.
func NewPlatformLogsServiceWithAuthz(
	s PlatformLogsQuerier, pdp authzcore.PDP, logger *slog.Logger,
) PlatformLogsQuerier {
	return &platformLogsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *platformLogsServiceWithAuthz) QueryPlatformLogs(
	ctx context.Context,
	req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	// An empty hierarchy is the cluster scope: resourceHierarchyToPath maps it to "*",
	// which only a cluster-scoped binding can satisfy. Deliberately not derived from
	// anything in the request - the query's namespaces are Kubernetes namespaces of
	// the pods it names, not OpenChoreo namespaces, and treating them as a hierarchy
	// would hand out access on a name collision.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewPlatformLogs,
		observerAuthz.ResourceTypePlatform, "", authzcore.ResourceHierarchy{},
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryPlatformLogs(ctx, req)
}

// --- filter values ---

func (s *platformLogsServiceWithAuthz) QueryPlatformLogFilterValues(
	ctx context.Context,
	req *types.PlatformLogFilterValuesRequest,
) (*types.PlatformLogFilterValuesResponse, error) {
	// The same action and the same cluster scope as the platform logs themselves.
	// An empty hierarchy maps to "*", which only a cluster-scoped binding satisfies.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewPlatformLogs,
		observerAuthz.ResourceTypePlatform, "", authzcore.ResourceHierarchy{},
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryPlatformLogFilterValues(ctx, req)
}
