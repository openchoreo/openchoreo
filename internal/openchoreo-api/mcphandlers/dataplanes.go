// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (h *MCPHandler) ListDataPlanes(ctx context.Context, orgName string) (string, error) {
	dataplanes, err := h.Services.DataPlaneService.ListDataPlanes(ctx, orgName)
	if err != nil {
		return "", err
	}

	return marshalResponse(dataplanes)
}

func (h *MCPHandler) GetDataPlane(ctx context.Context, orgName, dpName string) (string, error) {
	dataplane, err := h.Services.DataPlaneService.GetDataPlane(ctx, orgName, dpName)
	if err != nil {
		return "", err
	}

	return marshalResponse(dataplane)
}

func (h *MCPHandler) CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (string, error) {
	dataplane, err := h.Services.DataPlaneService.CreateDataPlane(ctx, orgName, req)
	if err != nil {
		return "", err
	}

	return marshalResponse(dataplane)
}
