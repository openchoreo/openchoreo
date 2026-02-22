// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListComponentTypesResponse struct {
	ComponentTypes any `json:"component_types"`
}

type ListWorkflowsResponse struct {
	Workflows any `json:"workflows"`
}

type ListTraitsResponse struct {
	Traits any `json:"traits"`
}

type ListWorkflowRunsResponse struct {
	WorkflowRuns any `json:"workflow_runs"`
}

func (h *MCPHandler) ListComponentTypes(ctx context.Context, namespaceName string) (any, error) {
	componentTypes, err := h.Services.ComponentTypeService.ListComponentTypes(ctx, namespaceName)
	if err != nil {
		return ListComponentTypesResponse{}, err
	}
	return ListComponentTypesResponse{
		ComponentTypes: componentTypes,
	}, nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error) {
	return h.Services.ComponentTypeService.GetComponentTypeSchema(ctx, namespaceName, ctName)
}

func (h *MCPHandler) ListWorkflows(ctx context.Context, namespaceName string) (any, error) {
	workflows, err := h.Services.WorkflowService.ListWorkflows(ctx, namespaceName)
	if err != nil {
		return ListWorkflowsResponse{}, err
	}
	return ListWorkflowsResponse{
		Workflows: workflows,
	}, nil
}

func (h *MCPHandler) GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error) {
	return h.Services.WorkflowService.GetWorkflowSchema(ctx, namespaceName, workflowName)
}

func (h *MCPHandler) CreateWorkflowRun(ctx context.Context, namespaceName string, req *models.CreateWorkflowRunRequest) (any, error) {
	return h.Services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, req)
}

func (h *MCPHandler) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	workflowRuns, err := h.Services.WorkflowRunService.ListWorkflowRuns(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return ListWorkflowRunsResponse{}, err
	}
	return ListWorkflowRunsResponse{
		WorkflowRuns: workflowRuns,
	}, nil
}

func (h *MCPHandler) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error) {
	return h.Services.WorkflowRunService.GetWorkflowRun(ctx, namespaceName, runName)
}

func (h *MCPHandler) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, stepName string, sinceSeconds *int64) (any, error) {
	return h.Services.WorkflowRunService.GetWorkflowRunLogs(
		ctx, namespaceName, runName, stepName, h.GatewayURL, sinceSeconds)
}

func (h *MCPHandler) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, stepName string) (any, error) {
	return h.Services.WorkflowRunService.GetWorkflowRunEvents(
		ctx, namespaceName, runName, stepName, h.GatewayURL)
}

func (h *MCPHandler) ListTraits(ctx context.Context, namespaceName string) (any, error) {
	traits, err := h.Services.TraitService.ListTraits(ctx, namespaceName)
	if err != nil {
		return ListTraitsResponse{}, err
	}
	return ListTraitsResponse{
		Traits: traits,
	}, nil
}

func (h *MCPHandler) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error) {
	return h.Services.TraitService.GetTraitSchema(ctx, namespaceName, traitName)
}

func (h *MCPHandler) ListObservabilityPlanes(ctx context.Context, namespaceName string) (any, error) {
	return h.Services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, namespaceName)
}
