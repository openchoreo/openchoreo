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
	Workflows any `json:"component-component-workflows"`
}

type ListTraitsResponse struct {
	Traits any `json:"traits"`
}

func (h *MCPHandler) ListComponentTypes(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items
	opts := &models.ListOptions{
		Limit:    models.MaxPageLimit,
		Continue: "",
	}
	result, err := h.Services.ComponentTypeService.ListComponentTypes(ctx, orgName, opts)
	if err != nil {
		return ListComponentTypesResponse{}, err
	}

	// Warn if result may be truncated
	h.warnIfTruncated("component_types", len(result.Items))

	return ListComponentTypesResponse{
		ComponentTypes: result.Items,
	}, nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (any, error) {
	return h.Services.ComponentTypeService.GetComponentTypeSchema(ctx, orgName, ctName)
}

func (h *MCPHandler) ListWorkflows(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items
	opts := &models.ListOptions{
		Limit:    models.MaxPageLimit,
		Continue: "",
	}
	result, err := h.Services.WorkflowService.ListWorkflows(ctx, orgName, opts)
	if err != nil {
		return ListWorkflowsResponse{}, err
	}

	// Warn if result may be truncated
	h.warnIfTruncated("workflows", len(result.Items))

	return ListWorkflowsResponse{
		Workflows: result.Items,
	}, nil
}

func (h *MCPHandler) GetWorkflowSchema(ctx context.Context, orgName, workflowName string) (any, error) {
	return h.Services.WorkflowService.GetWorkflowSchema(ctx, orgName, workflowName)
}

func (h *MCPHandler) ListTraits(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items
	opts := &models.ListOptions{
		Limit:    models.MaxPageLimit,
		Continue: "",
	}
	result, err := h.Services.TraitService.ListTraits(ctx, orgName, opts)
	if err != nil {
		return ListTraitsResponse{}, err
	}

	// Warn if result may be truncated
	h.warnIfTruncated("traits", len(result.Items))

	return ListTraitsResponse{
		Traits: result.Items,
	}, nil
}

func (h *MCPHandler) GetTraitSchema(ctx context.Context, orgName, traitName string) (any, error) {
	return h.Services.TraitService.GetTraitSchema(ctx, orgName, traitName)
}
