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
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.DataPlaneResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.DataPlaneService.ListDataPlanes(ctx, orgName, opts)
		if err != nil {
			return ListDataPlanesResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("dataplanes", len(allItems))

	return ListDataPlanesResponse{
		DataPlanes: allItems,
	}, nil
}

func (h *MCPHandler) GetDataPlane(ctx context.Context, orgName, dpName string) (any, error) {
	return h.Services.DataPlaneService.GetDataPlane(ctx, orgName, dpName)
}

func (h *MCPHandler) CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (any, error) {
	return h.Services.DataPlaneService.CreateDataPlane(ctx, orgName, req)
}
