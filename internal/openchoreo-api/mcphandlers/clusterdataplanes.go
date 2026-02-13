// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterDataPlanesResponse struct {
	ClusterDataPlanes []*models.ClusterDataPlaneResponse `json:"cluster_data_planes"`
}

func (h *MCPHandler) ListClusterDataPlanes(ctx context.Context) (any, error) {
	clusterDataPlanes, err := h.Services.ClusterDataPlaneService.ListClusterDataPlanes(ctx)
	if err != nil {
		return ListClusterDataPlanesResponse{}, fmt.Errorf("list cluster dataplanes failed: %w", err)
	}
	return ListClusterDataPlanesResponse{
		ClusterDataPlanes: clusterDataPlanes,
	}, nil
}

func (h *MCPHandler) GetClusterDataPlane(ctx context.Context, cdpName string) (any, error) {
	result, err := h.Services.ClusterDataPlaneService.GetClusterDataPlane(ctx, cdpName)
	if err != nil {
		return nil, fmt.Errorf("get cluster dataplane %q failed: %w", cdpName, err)
	}
	return result, nil
}

func (h *MCPHandler) CreateClusterDataPlane(ctx context.Context, req *models.CreateClusterDataPlaneRequest) (any, error) {
	result, err := h.Services.ClusterDataPlaneService.CreateClusterDataPlane(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create cluster dataplane failed: %w", err)
	}
	return result, nil
}
