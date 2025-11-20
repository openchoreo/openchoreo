// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListDataPlanesResponse struct {
	DataPlanes []*models.DataPlaneResponse `json:"data_planes"`
}

func (h *MCPHandler) ListDataPlanes(ctx context.Context, orgName string) (any, error) {
	dataplanes, err := h.Services.DataPlaneService.ListDataPlanes(ctx, orgName)
	if err != nil {
		return ListDataPlanesResponse{}, err
	}
	return ListDataPlanesResponse{
		DataPlanes: dataplanes,
	}, nil
}

func (h *MCPHandler) GetDataPlane(ctx context.Context, orgName, dpName string) (any, error) {
	return h.Services.DataPlaneService.GetDataPlane(ctx, orgName, dpName)
}

func (h *MCPHandler) CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (any, error) {
	return h.Services.DataPlaneService.CreateDataPlane(ctx, orgName, req)
}
