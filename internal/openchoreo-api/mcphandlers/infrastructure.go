// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListComponentTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ComponentTypeService.ListComponentTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("component_types", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error) {
	return h.services.ComponentTypeService.GetComponentTypeSchema(ctx, namespaceName, ctName)
}

func (h *MCPHandler) ListTraits(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.TraitService.ListTraits(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("traits", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error) {
	return h.services.TraitService.GetTraitSchema(ctx, namespaceName, traitName)
}

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

func (h *MCPHandler) CreateWorkflowRun(ctx context.Context, namespaceName, workflowName string, parameters map[string]any) (any, error) {
	wfRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: workflowName + "-run-",
			Namespace:    namespaceName,
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name: workflowName,
			},
		},
	}

	if parameters != nil {
		rawParams, err := json.Marshal(parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal workflow parameters: %w", err)
		}
		wfRun.Spec.Workflow.Parameters = &runtime.RawExtension{Raw: rawParams}
	}

	return h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, wfRun)
}

func (h *MCPHandler) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, namespaceName, projectName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("workflow_runs", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error) {
	return h.services.WorkflowRunService.GetWorkflowRun(ctx, namespaceName, runName)
}
