// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterComponentTypesResponse struct {
	ClusterComponentTypes []*models.ComponentTypeResponse `json:"cluster_component_types"`
}

func (h *MCPHandler) ListClusterComponentTypes(ctx context.Context) (any, error) {
	clusterComponentTypes, err := h.Services.ClusterComponentTypeService.ListClusterComponentTypes(ctx)
	if err != nil {
		return ListClusterComponentTypesResponse{}, fmt.Errorf("list cluster component types failed: %w", err)
	}
	return ListClusterComponentTypesResponse{
		ClusterComponentTypes: clusterComponentTypes,
	}, nil
}

func (h *MCPHandler) GetClusterComponentType(ctx context.Context, cctName string) (any, error) {
	result, err := h.Services.ClusterComponentTypeService.GetClusterComponentType(ctx, cctName)
	if err != nil {
		return nil, fmt.Errorf("get cluster component type %q failed: %w", cctName, err)
	}
	return result, nil
}

func (h *MCPHandler) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error) {
	result, err := h.Services.ClusterComponentTypeService.GetClusterComponentTypeSchema(ctx, cctName)
	if err != nil {
		return nil, fmt.Errorf("get cluster component type schema %q failed: %w", cctName, err)
	}
	return result, nil
}
