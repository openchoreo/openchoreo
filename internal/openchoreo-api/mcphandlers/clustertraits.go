// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListClusterTraitsResponse struct {
	ClusterTraits []*models.TraitResponse `json:"cluster_traits"`
}

func (h *MCPHandler) ListClusterTraits(ctx context.Context) (any, error) {
	clusterTraits, err := h.Services.ClusterTraitService.ListClusterTraits(ctx)
	if err != nil {
		return ListClusterTraitsResponse{}, fmt.Errorf("list cluster traits failed: %w", err)
	}
	return ListClusterTraitsResponse{
		ClusterTraits: clusterTraits,
	}, nil
}

func (h *MCPHandler) GetClusterTrait(ctx context.Context, ctName string) (any, error) {
	result, err := h.Services.ClusterTraitService.GetClusterTrait(ctx, ctName)
	if err != nil {
		return nil, fmt.Errorf("get cluster trait %q failed: %w", ctName, err)
	}
	return result, nil
}

func (h *MCPHandler) GetClusterTraitSchema(ctx context.Context, ctName string) (any, error) {
	result, err := h.Services.ClusterTraitService.GetClusterTraitSchema(ctx, ctName)
	if err != nil {
		return nil, fmt.Errorf("get cluster trait schema %q failed: %w", ctName, err)
	}
	return result, nil
}
