// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

type ListComponentTypesResponse struct {
	ComponentTypes any `json:"component_types"`
}

type ListWorkflowsResponse struct {
	Workflows any `json:"component-component-workflows"`
}

type ListTraitsResponse struct {
	Traits any `json:"traits"`
}

func (h *MCPHandler) ListComponentTypes(ctx context.Context, orgName string) (any, error) {
	componentTypes, err := h.Services.ComponentTypeService.ListComponentTypes(ctx, orgName)
	if err != nil {
		return ListComponentTypesResponse{}, err
	}
	return ListComponentTypesResponse{
		ComponentTypes: componentTypes,
	}, nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (any, error) {
	return h.Services.ComponentTypeService.GetComponentTypeSchema(ctx, orgName, ctName)
}

func (h *MCPHandler) ListWorkflows(ctx context.Context, orgName string) (any, error) {
	workflows, err := h.Services.WorkflowService.ListWorkflows(ctx, orgName)
	if err != nil {
		return ListWorkflowsResponse{}, err
	}
	return ListWorkflowsResponse{
		Workflows: workflows,
	}, nil
}

func (h *MCPHandler) GetWorkflowSchema(ctx context.Context, orgName, workflowName string) (any, error) {
	return h.Services.WorkflowService.GetWorkflowSchema(ctx, orgName, workflowName)
}

func (h *MCPHandler) ListTraits(ctx context.Context, orgName string) (any, error) {
	traits, err := h.Services.TraitService.ListTraits(ctx, orgName)
	if err != nil {
		return ListTraitsResponse{}, err
	}
	return ListTraitsResponse{
		Traits: traits,
	}, nil
}

func (h *MCPHandler) GetTraitSchema(ctx context.Context, orgName, traitName string) (any, error) {
	return h.Services.TraitService.GetTraitSchema(ctx, orgName, traitName)
}

func (h *MCPHandler) ListObservabilityPlanes(ctx context.Context, orgName string) (any, error) {
	return h.Services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, orgName)
}
