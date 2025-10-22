// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, orgName string) (string, error) {
	buildplanes, err := h.Services.BuildPlaneService.ListBuildPlanes(ctx, orgName)
	if err != nil {
		return "", err
	}

	return marshalResponse(buildplanes)
}
