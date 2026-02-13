// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterBuildPlanesResponse struct {
	ClusterBuildPlanes []models.ClusterBuildPlaneResponse `json:"cluster_build_planes"`
}

func (h *MCPHandler) ListClusterBuildPlanes(ctx context.Context) (any, error) {
	clusterBuildPlanes, err := h.Services.ClusterBuildPlaneService.ListClusterBuildPlanes(ctx)
	if err != nil {
		return ListClusterBuildPlanesResponse{}, fmt.Errorf("list cluster build planes failed: %w", err)
	}
	return ListClusterBuildPlanesResponse{
		ClusterBuildPlanes: clusterBuildPlanes,
	}, nil
}
