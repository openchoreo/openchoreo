// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

func (h *MCPHandler) GetProjectDeploymentPipeline(ctx context.Context, orgName, projectName string) (any, error) {
	return h.Services.DeploymentPipelineService.GetProjectDeploymentPipeline(ctx, orgName, projectName)
}
