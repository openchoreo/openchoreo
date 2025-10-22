// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (h *MCPHandler) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (string, error) {
	component, err := h.Services.ComponentService.CreateComponent(ctx, orgName, projectName, req)
	if err != nil {
		return "", err
	}

	return marshalResponse(component)
}

func (h *MCPHandler) ListComponents(ctx context.Context, orgName, projectName string) (string, error) {
	components, err := h.Services.ComponentService.ListComponents(ctx, orgName, projectName)
	if err != nil {
		return "", err
	}

	return marshalResponse(components)
}

func (h *MCPHandler) GetComponent(ctx context.Context, orgName, projectName, componentName string, additionalResources []string) (string, error) {
	component, err := h.Services.ComponentService.GetComponent(ctx, orgName, projectName, componentName, additionalResources)
	if err != nil {
		return "", err
	}

	return marshalResponse(component)
}

func (h *MCPHandler) GetComponentBinding(ctx context.Context, orgName, projectName, componentName, environment string) (string, error) {
	binding, err := h.Services.ComponentService.GetComponentBinding(ctx, orgName, projectName, componentName, environment)
	if err != nil {
		return "", err
	}

	return marshalResponse(binding)
}

func (h *MCPHandler) UpdateComponentBinding(ctx context.Context, orgName, projectName, componentName, bindingName string, req *models.UpdateBindingRequest) (string, error) {
	binding, err := h.Services.ComponentService.UpdateComponentBinding(ctx, orgName, projectName, componentName, bindingName, req)
	if err != nil {
		return "", err
	}

	return marshalResponse(binding)
}

func (h *MCPHandler) GetComponentObserverURL(ctx context.Context, orgName, projectName, componentName, environmentName string) (string, error) {
	observerURL, err := h.Services.ComponentService.GetComponentObserverURL(ctx, orgName, projectName, componentName, environmentName)
	if err != nil {
		return "", err
	}

	return marshalResponse(observerURL)
}

func (h *MCPHandler) GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (string, error) {
	observerURL, err := h.Services.ComponentService.GetBuildObserverURL(ctx, orgName, projectName, componentName)
	if err != nil {
		return "", err
	}

	return marshalResponse(observerURL)
}

func (h *MCPHandler) GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (string, error) {
	workloads, err := h.Services.ComponentService.GetComponentWorkloads(ctx, orgName, projectName, componentName)
	if err != nil {
		return "", err
	}

	return marshalResponse(workloads)
}
