// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

type ListBuildPlanesResponse struct {
	BuildPlanes any `json:"build_planes"`
}

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, orgName string) (any, error) {
	buildplanes, err := h.Services.BuildPlaneService.ListBuildPlanes(ctx, orgName)
	if err != nil {
		return ListBuildPlanesResponse{}, err
	}
	return ListBuildPlanesResponse{
		BuildPlanes: buildplanes,
	}, nil
}
