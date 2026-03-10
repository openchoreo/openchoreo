// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacymcphandlers

import (
	"context"
)

type ListWorkflowPlanesResponse struct {
	WorkflowPlanes any `json:"workflow_planes"`
}

func (h *MCPHandler) ListWorkflowPlanes(ctx context.Context, namespaceName string) (any, error) {
	workflowplanes, err := h.Services.WorkflowPlaneService.ListWorkflowPlanes(ctx, namespaceName)
	if err != nil {
		return ListWorkflowPlanesResponse{}, err
	}
	return ListWorkflowPlanesResponse{
		WorkflowPlanes: workflowplanes,
	}, nil
}
