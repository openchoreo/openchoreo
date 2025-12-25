// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListBuildPlanesResponse struct {
	BuildPlanes any `json:"build_planes"`
}

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items
	opts := &models.ListOptions{
		Limit:    models.MaxPageLimit,
		Continue: "",
	}
	result, err := h.Services.BuildPlaneService.ListBuildPlanes(ctx, orgName, opts)
	if err != nil {
		return ListBuildPlanesResponse{}, err
	}

	// Warn if result may be truncated
	h.warnIfTruncated("build_planes", len(result.Items))

	return ListBuildPlanesResponse{
		BuildPlanes: result.Items,
	}, nil
}
