// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListDataPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.DataPlaneService.ListDataPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("data_planes", result.Items, result.NextCursor, dataplaneSummary), nil
}

func (h *MCPHandler) GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error) {
	dp, err := h.services.DataPlaneService.GetDataPlane(ctx, namespaceName, dpName)
	if err != nil {
		return nil, err
	}
	return dataplaneDetail(dp), nil
}
