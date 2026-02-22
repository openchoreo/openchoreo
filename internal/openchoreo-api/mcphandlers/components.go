// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

func (h *MCPHandler) CreateComponent(ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest) (any, error) {
	return h.Services.ComponentService.CreateComponent(ctx, namespaceName, projectName, req)
}

func (h *MCPHandler) ListComponents(ctx context.Context, namespaceName, projectName string) (any, error) {
	components, err := h.Services.ComponentService.ListComponents(ctx, namespaceName, projectName)
	if err != nil {
		return ListComponentsResponse{}, err
	}
	return ListComponentsResponse{
		Components: components,
	}, nil
}

func (h *MCPHandler) GetComponent(ctx context.Context, namespaceName, projectName, componentName string, additionalResources []string) (any, error) {
	return h.Services.ComponentService.GetComponent(ctx, namespaceName, projectName, componentName, additionalResources)
}

func (h *MCPHandler) UpdateComponentBinding(ctx context.Context, namespaceName, projectName, componentName, bindingName string, req *models.UpdateBindingRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentBinding(ctx, namespaceName, projectName, componentName, bindingName, req)
}

func (h *MCPHandler) GetComponentObserverURL(ctx context.Context, namespaceName, projectName, componentName, environmentName string) (any, error) {
	return h.Services.ComponentService.GetComponentObserverURL(ctx, namespaceName, projectName, componentName, environmentName)
}

func (h *MCPHandler) GetBuildObserverURL(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.GetBuildObserverURL(ctx, namespaceName, projectName, componentName)
}

func (h *MCPHandler) GetComponentWorkloads(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.GetComponentWorkloads(ctx, namespaceName, projectName, componentName)
}

func (h *MCPHandler) ListComponentReleases(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	releases, err := h.Services.ComponentService.ListComponentReleases(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return ListComponentReleasesResponse{}, err
	}
	return ListComponentReleasesResponse{
		Releases: releases,
	}, nil
}

func (h *MCPHandler) CreateComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.CreateComponentRelease(ctx, namespaceName, projectName, componentName, releaseName)
}

func (h *MCPHandler) GetComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.GetComponentRelease(ctx, namespaceName, projectName, componentName, releaseName)
}

func (h *MCPHandler) ListReleaseBindings(ctx context.Context, namespaceName, projectName, componentName string, environments []string) (any, error) {
	bindings, err := h.Services.ComponentService.ListReleaseBindings(ctx, namespaceName, projectName, componentName, environments)
	if err != nil {
		return ListReleaseBindingsResponse{}, err
	}
	return ListReleaseBindingsResponse{
		Bindings: bindings,
	}, nil
}

func (h *MCPHandler) PatchReleaseBinding(ctx context.Context, namespaceName, projectName, componentName, bindingName string, req *models.PatchReleaseBindingRequest) (any, error) {
	return h.Services.ComponentService.PatchReleaseBinding(ctx, namespaceName, projectName, componentName, bindingName, req)
}

func (h *MCPHandler) DeployRelease(ctx context.Context, namespaceName, projectName, componentName string, req *models.DeployReleaseRequest) (any, error) {
	return h.Services.ComponentService.DeployRelease(ctx, namespaceName, projectName, componentName, req)
}

func (h *MCPHandler) PromoteComponent(ctx context.Context, namespaceName, projectName, componentName string, req *models.PromoteComponentRequest) (any, error) {
	binding, err := h.Services.ComponentService.PromoteComponent(ctx, &services.PromoteComponentPayload{
		PromoteComponentRequest: *req,
		ComponentName:           componentName,
		ProjectName:             projectName,
		NamespaceName:           namespaceName,
	})
	return binding, err
}

func (h *MCPHandler) CreateWorkload(ctx context.Context, namespaceName, projectName, componentName string, workloadSpec interface{}) (any, error) {
	// Convert interface{} to WorkloadSpec
	workloadSpecBytes, err := json.Marshal(workloadSpec)
	if err != nil {
		return nil, err
	}

	var spec openchoreov1alpha1.WorkloadSpec
	if err := json.Unmarshal(workloadSpecBytes, &spec); err != nil {
		return nil, err
	}

	return h.Services.ComponentService.CreateComponentWorkload(ctx, namespaceName, projectName, componentName, &spec)
}

func (h *MCPHandler) GetComponentSchema(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	return h.Services.ComponentService.GetComponentSchema(ctx, namespaceName, projectName, componentName)
}

func (h *MCPHandler) GetComponentReleaseSchema(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (any, error) {
	return h.Services.ComponentService.GetComponentReleaseSchema(ctx, namespaceName, projectName, componentName, releaseName)
}

func (h *MCPHandler) ListComponentTraits(ctx context.Context, namespaceName, projectName, componentName string) (any, error) {
	traits, err := h.Services.ComponentService.ListComponentTraits(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return ListComponentTraitsResponse{}, err
	}
	return ListComponentTraitsResponse{
		Traits: traits,
	}, nil
}

func (h *MCPHandler) UpdateComponentTraits(ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentTraitsRequest) (any, error) {
	return h.Services.ComponentService.UpdateComponentTraits(ctx, namespaceName, projectName, componentName, req)
}

func (h *MCPHandler) GetEnvironmentRelease(ctx context.Context, namespaceName, projectName, componentName, environmentName string) (any, error) {
	return h.Services.ComponentService.GetEnvironmentRelease(ctx, namespaceName, projectName, componentName, environmentName)
}

func (h *MCPHandler) PatchComponent(ctx context.Context, namespaceName, projectName, componentName string, req *models.PatchComponentRequest) (any, error) {
	return h.Services.ComponentService.PatchComponent(ctx, namespaceName, projectName, componentName, req)
}

func (h *MCPHandler) TriggerWorkflowRunForComponent(ctx context.Context, namespaceName, projectName, componentName, commit string) (any, error) {
	return h.Services.WorkflowRunService.TriggerWorkflow(ctx, namespaceName, projectName, componentName, commit)
}
