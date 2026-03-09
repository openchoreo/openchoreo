// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ClusterDataPlane operations

func (h *MCPHandler) ListClusterDataPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterDataPlaneService.ListClusterDataPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_data_planes", result.Items, result.NextCursor, clusterDataPlaneSummary), nil
}

func (h *MCPHandler) GetClusterDataPlane(ctx context.Context, cdpName string) (any, error) {
	cdp, err := h.services.ClusterDataPlaneService.GetClusterDataPlane(ctx, cdpName)
	if err != nil {
		return nil, err
	}
	return clusterDataPlaneDetail(cdp), nil
}

// ClusterBuildPlane operations

func (h *MCPHandler) ListClusterBuildPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterBuildPlaneService.ListClusterBuildPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_build_planes", result.Items, result.NextCursor, clusterBuildPlaneSummary), nil
}

// ClusterObservabilityPlane operations

func (h *MCPHandler) ListClusterObservabilityPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_observability_planes", result.Items, result.NextCursor, clusterObservabilityPlaneSummary), nil
}
