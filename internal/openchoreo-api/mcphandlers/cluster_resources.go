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

// ClusterWorkflowPlane operations

func (h *MCPHandler) ListClusterWorkflowPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterWorkflowPlaneService.ListClusterWorkflowPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_workflow_planes", result.Items, result.NextCursor, clusterWorkflowPlaneSummary), nil
}

func (h *MCPHandler) GetClusterWorkflowPlane(ctx context.Context, cbpName string) (any, error) {
	cbp, err := h.services.ClusterWorkflowPlaneService.GetClusterWorkflowPlane(ctx, cbpName)
	if err != nil {
		return nil, err
	}
	return clusterWorkflowPlaneDetail(cbp), nil
}

// ClusterObservabilityPlane operations

func (h *MCPHandler) ListClusterObservabilityPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_observability_planes", result.Items, result.NextCursor, clusterObservabilityPlaneSummary), nil
}

func (h *MCPHandler) GetClusterObservabilityPlane(ctx context.Context, copName string) (any, error) {
	cop, err := h.services.ClusterObservabilityPlaneService.GetClusterObservabilityPlane(ctx, copName)
	if err != nil {
		return nil, err
	}
	return clusterObservabilityPlaneDetail(cop), nil
}
