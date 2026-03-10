// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacymcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterWorkflowPlanesResponse struct {
	ClusterWorkflowPlanes []models.ClusterWorkflowPlaneResponse `json:"cluster_workflow_planes"`
}

func (h *MCPHandler) ListClusterWorkflowPlanes(ctx context.Context) (any, error) {
	clusterWorkflowPlanes, err := h.Services.ClusterWorkflowPlaneService.ListClusterWorkflowPlanes(ctx)
	if err != nil {
		return ListClusterWorkflowPlanesResponse{}, fmt.Errorf("list cluster workflow planes failed: %w", err)
	}
	return ListClusterWorkflowPlanesResponse{
		ClusterWorkflowPlanes: clusterWorkflowPlanes,
	}, nil
}
