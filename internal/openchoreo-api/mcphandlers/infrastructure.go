// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListObservabilityPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("observability_planes", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetDeploymentPipeline(ctx context.Context, namespaceName, pipelineName string) (any, error) {
	return h.services.DeploymentPipelineService.GetDeploymentPipeline(ctx, namespaceName, pipelineName)
}

func (h *MCPHandler) ListDeploymentPipelines(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.DeploymentPipelineService.ListDeploymentPipelines(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("deployment_pipelines", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("build_planes", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetObserverURL(ctx context.Context, namespaceName, envName string) (any, error) {
	return h.services.EnvironmentService.GetObserverURL(ctx, namespaceName, envName)
}

