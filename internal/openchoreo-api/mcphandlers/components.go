// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

type ListComponentsResponse struct {
	Components []*models.ComponentResponse `json:"components"`
}

type ListComponentReleasesResponse struct {
	Releases []*models.ComponentReleaseResponse `json:"releases"`
}

type ListReleaseBindingsResponse struct {
	Bindings []*models.ReleaseBindingResponse `json:"bindings"`
}

type ListComponentWorkflowRunsResponse struct {
	WorkflowRuns []*models.ComponentWorkflowResponse `json:"workflowRuns"`
}

type ListComponentWorkflowsResponse struct {
	Workflows []*models.WorkflowResponse `json:"workflows"`
}

func (h *MCPHandler) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (any, error) {
	return h.Services.ComponentService.CreateComponent(ctx, orgName, projectName, req)
}

func (h *MCPHandler) ListComponents(ctx context.Context, orgName, projectName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.ComponentResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentService.ListComponents(ctx, orgName, projectName, opts)
		if err != nil {
			return ListComponentsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("components", len(allItems))

	return ListComponentsResponse{
		Components: allItems,
	}, nil
}

func (h *MCPHandler) GetComponent(ctx context.Context, orgName, projectName, componentName string, additionalResources []string) (any, error) {
	return h.Services.ComponentService.GetComponent(ctx, orgName, projectName, componentName, additionalResources)
}

func (h *MCPHandler) UpdateComponentBinding(ctx context.Context, orgName, projectName, componentName, bindingName string, req *models.UpdateBindingRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentBinding(ctx, orgName, projectName, componentName, bindingName, req)
}

func (h *MCPHandler) GetComponentObserverURL(ctx context.Context, orgName, projectName, componentName, environmentName string) (any, error) {
	return h.Services.ComponentService.GetComponentObserverURL(ctx, orgName, projectName, componentName, environmentName)
}

func (h *MCPHandler) GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.GetBuildObserverURL(ctx, orgName, projectName, componentName)
}

func (h *MCPHandler) GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*openchoreov1alpha1.WorkloadSpec
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentService.GetComponentWorkloads(ctx, orgName, projectName, componentName, opts)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	return allItems, nil
}

func (h *MCPHandler) ListComponentReleases(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.ComponentReleaseResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentService.ListComponentReleases(ctx, orgName, projectName, componentName, opts)
		if err != nil {
			return ListComponentReleasesResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("component_releases", len(allItems))

	return ListComponentReleasesResponse{
		Releases: allItems,
	}, nil
}

func (h *MCPHandler) CreateComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.CreateComponentRelease(ctx, orgName, projectName, componentName, releaseName)
}

func (h *MCPHandler) GetComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.GetComponentRelease(ctx, orgName, projectName, componentName, releaseName)
}

func (h *MCPHandler) ListReleaseBindings(ctx context.Context, orgName, projectName, componentName string, environments []string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.ReleaseBindingResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentService.ListReleaseBindings(ctx, orgName, projectName, componentName, environments, opts)
		if err != nil {
			return ListReleaseBindingsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("release_bindings", len(allItems))

	return ListReleaseBindingsResponse{
		Bindings: allItems,
	}, nil
}

func (h *MCPHandler) PatchReleaseBinding(ctx context.Context, orgName, projectName, componentName, bindingName string, req *models.PatchReleaseBindingRequest) (any, error) {
	return h.Services.ComponentService.PatchReleaseBinding(ctx, orgName, projectName, componentName, bindingName, req)
}

func (h *MCPHandler) DeployRelease(ctx context.Context, orgName, projectName, componentName string, req *models.DeployReleaseRequest) (any, error) {
	return h.Services.ComponentService.DeployRelease(ctx, orgName, projectName, componentName, req)
}

func (h *MCPHandler) PromoteComponent(ctx context.Context, orgName, projectName, componentName string, req *models.PromoteComponentRequest) (any, error) {
	binding, err := h.Services.ComponentService.PromoteComponent(ctx, &services.PromoteComponentPayload{
		PromoteComponentRequest: *req,
		ComponentName:           componentName,
		ProjectName:             projectName,
		OrgName:                 orgName,
	})
	return binding, err
}

func (h *MCPHandler) CreateWorkload(ctx context.Context, orgName, projectName, componentName string, workloadSpec interface{}) (any, error) {
	// Convert interface{} to WorkloadSpec
	workloadSpecBytes, err := json.Marshal(workloadSpec)
	if err != nil {
		return nil, err
	}

	var spec openchoreov1alpha1.WorkloadSpec
	if err := json.Unmarshal(workloadSpecBytes, &spec); err != nil {
		return nil, err
	}

	return h.Services.ComponentService.CreateComponentWorkload(ctx, orgName, projectName, componentName, &spec)
}

func (h *MCPHandler) GetComponentSchema(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.GetComponentSchema(ctx, orgName, projectName, componentName)
}

func (h *MCPHandler) GetComponentReleaseSchema(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.GetComponentReleaseSchema(ctx, orgName, projectName, componentName, releaseName)
}

func (h *MCPHandler) ListComponentTraits(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.ListComponentTraits(ctx, orgName, projectName, componentName)
}

func (h *MCPHandler) UpdateComponentTraits(ctx context.Context, orgName, projectName, componentName string, req *models.UpdateComponentTraitsRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentTraits(ctx, orgName, projectName, componentName, req)
}

func (h *MCPHandler) GetEnvironmentRelease(ctx context.Context, orgName, projectName, componentName, environmentName string) (any, error) {
	return h.Services.ComponentService.GetEnvironmentRelease(ctx, orgName, projectName, componentName, environmentName)
}

func (h *MCPHandler) PatchComponent(ctx context.Context, orgName, projectName, componentName string, req *models.PatchComponentRequest) (any, error) {
	return h.Services.ComponentService.PatchComponent(ctx, orgName, projectName, componentName, req)
}

func (h *MCPHandler) ListComponentWorkflows(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.WorkflowResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentWorkflowService.ListComponentWorkflows(ctx, orgName, opts)
		if err != nil {
			return ListComponentWorkflowsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("component_workflows", len(allItems))

	return ListComponentWorkflowsResponse{
		Workflows: allItems,
	}, nil
}

func (h *MCPHandler) GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (any, error) {
	return h.Services.ComponentWorkflowService.GetComponentWorkflowSchema(ctx, orgName, cwName)
}

func (h *MCPHandler) TriggerComponentWorkflow(ctx context.Context, orgName, projectName, componentName, commit string) (any, error) {
	return h.Services.ComponentWorkflowService.TriggerWorkflow(ctx, orgName, projectName, componentName, commit)
}

func (h *MCPHandler) ListComponentWorkflowRuns(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.ComponentWorkflowResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ComponentWorkflowService.ListComponentWorkflowRuns(ctx, orgName, projectName, componentName, opts)
		if err != nil {
			return ListComponentWorkflowRunsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("component_workflow_runs", len(allItems))

	return ListComponentWorkflowRunsResponse{
		WorkflowRuns: allItems,
	}, nil
}

func (h *MCPHandler) UpdateComponentWorkflowSchema(ctx context.Context, orgName, projectName, componentName string, req *models.UpdateComponentWorkflowSchemaRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentWorkflowSchema(ctx, orgName, projectName, componentName, req)
}
