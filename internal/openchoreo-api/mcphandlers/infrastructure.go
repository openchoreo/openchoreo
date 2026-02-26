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
	return wrapTransformedList("observability_planes", result.Items, result.NextCursor, observabilityPlaneSummary), nil
}

func (h *MCPHandler) GetDeploymentPipeline(ctx context.Context, namespaceName, pipelineName string) (any, error) {
	dp, err := h.services.DeploymentPipelineService.GetDeploymentPipeline(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, err
	}
	return deploymentPipelineDetail(dp), nil
}

func (h *MCPHandler) ListDeploymentPipelines(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.DeploymentPipelineService.ListDeploymentPipelines(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("deployment_pipelines", result.Items, result.NextCursor, deploymentPipelineSummary), nil
}

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("build_planes", result.Items, result.NextCursor, buildPlaneSummary), nil
}

func (h *MCPHandler) GetObserverURL(ctx context.Context, namespaceName, envName string) (any, error) {
	return h.services.EnvironmentService.GetObserverURL(ctx, namespaceName, envName)
}
