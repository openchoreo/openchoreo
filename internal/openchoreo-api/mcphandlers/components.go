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
	WorkflowRuns []models.ComponentWorkflowResponse `json:"workflowRuns"`
}

type ListComponentWorkflowsResponse struct {
	Workflows []*models.WorkflowResponse `json:"workflows"`
}

func (h *MCPHandler) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (any, error) {
	return h.Services.ComponentService.CreateComponent(ctx, orgName, projectName, req)
}

func (h *MCPHandler) ListComponents(ctx context.Context, orgName, projectName string) (any, error) {
	components, err := h.Services.ComponentService.ListComponents(ctx, orgName, projectName)
	if err != nil {
		return ListComponentsResponse{}, err
	}
	return ListComponentsResponse{
		Components: components,
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
	return h.Services.ComponentService.GetComponentWorkloads(ctx, orgName, projectName, componentName)
}

func (h *MCPHandler) ListComponentReleases(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	releases, err := h.Services.ComponentService.ListComponentReleases(ctx, orgName, projectName, componentName)
	if err != nil {
		return ListComponentReleasesResponse{}, err
	}
	return ListComponentReleasesResponse{
		Releases: releases,
	}, nil
}

func (h *MCPHandler) CreateComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.CreateComponentRelease(ctx, orgName, projectName, componentName, releaseName)
}

func (h *MCPHandler) GetComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.GetComponentRelease(ctx, orgName, projectName, componentName, releaseName)
}

func (h *MCPHandler) ListReleaseBindings(ctx context.Context, orgName, projectName, componentName string, environments []string) (any, error) {
	bindings, err := h.Services.ComponentService.ListReleaseBindings(ctx, orgName, projectName, componentName, environments)
	if err != nil {
		return ListReleaseBindingsResponse{}, err
	}
	return ListReleaseBindingsResponse{
		Bindings: bindings,
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

func (h *MCPHandler) GetEnvironmentRenderedRelease(ctx context.Context, orgName, projectName, componentName, environmentName string) (any, error) {
	return h.Services.ComponentService.GetEnvironmentRenderedRelease(ctx, orgName, projectName, componentName, environmentName)
}

func (h *MCPHandler) PatchComponent(ctx context.Context, orgName, projectName, componentName string, req *models.PatchComponentRequest) (any, error) {
	return h.Services.ComponentService.PatchComponent(ctx, orgName, projectName, componentName, req)
}

func (h *MCPHandler) ListComponentWorkflows(ctx context.Context, orgName string) (any, error) {
	workflows, err := h.Services.ComponentWorkflowService.ListComponentWorkflows(ctx, orgName)
	if err != nil {
		return ListComponentWorkflowsResponse{}, err
	}
	return ListComponentWorkflowsResponse{
		Workflows: workflows,
	}, nil
}

func (h *MCPHandler) GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (any, error) {
	return h.Services.ComponentWorkflowService.GetComponentWorkflowSchema(ctx, orgName, cwName)
}

func (h *MCPHandler) TriggerComponentWorkflow(ctx context.Context, orgName, projectName, componentName, commit string) (any, error) {
	return h.Services.ComponentWorkflowService.TriggerWorkflow(ctx, orgName, projectName, componentName, commit)
}

func (h *MCPHandler) ListComponentWorkflowRuns(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	workflowRuns, err := h.Services.ComponentWorkflowService.ListComponentWorkflowRuns(ctx, orgName, projectName, componentName)
	if err != nil {
		return ListComponentWorkflowRunsResponse{}, err
	}
	return ListComponentWorkflowRunsResponse{
		WorkflowRuns: workflowRuns,
	}, nil
}

func (h *MCPHandler) UpdateComponentWorkflowSchema(ctx context.Context, orgName, projectName, componentName string, req *models.UpdateComponentWorkflowRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentWorkflowSchema(ctx, orgName, projectName, componentName, req)
}
