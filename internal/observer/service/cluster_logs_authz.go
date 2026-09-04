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

// clusterLogsServiceWithAuthz wraps a ClusterLogsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via
// NewClusterLogsServiceWithAuthz.
type clusterLogsServiceWithAuthz struct {
	internal ClusterLogsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ ClusterLogsQuerier = (*clusterLogsServiceWithAuthz)(nil)

// NewClusterLogsServiceWithAuthz wraps the provided ClusterLogsQuerier with
// authorization checks.
func NewClusterLogsServiceWithAuthz(
	s ClusterLogsQuerier, pdp authzcore.PDP, logger *slog.Logger,
) ClusterLogsQuerier {
	return &clusterLogsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *clusterLogsServiceWithAuthz) QueryClusterLogs(
	ctx context.Context,
	req *types.ClusterLogsQueryRequest,
) (*types.ClusterLogsResponse, error) {
	// An empty hierarchy is the cluster scope: resourceHierarchyToPath maps it to "*",
	// which only a cluster-scoped binding can satisfy. Deliberately not derived from
	// anything in the request - the query's namespaces are Kubernetes namespaces of
	// the pods it names, not OpenChoreo namespaces, and treating them as a hierarchy
	// would hand out access on a name collision.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewClusterLogs,
		observerAuthz.ResourceTypeCluster, "", authzcore.ResourceHierarchy{},
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryClusterLogs(ctx, req)
}
