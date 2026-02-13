// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterObservabilityPlanesResponse struct {
	ClusterObservabilityPlanes []models.ClusterObservabilityPlaneResponse `json:"cluster_observability_planes"`
}

func (h *MCPHandler) ListClusterObservabilityPlanes(ctx context.Context) (any, error) {
	clusterObservabilityPlanes, err := h.Services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx)
	if err != nil {
		return ListClusterObservabilityPlanesResponse{}, fmt.Errorf("list cluster observability planes failed: %w", err)
	}
	return ListClusterObservabilityPlanesResponse{
		ClusterObservabilityPlanes: clusterObservabilityPlanes,
	}, nil
}
