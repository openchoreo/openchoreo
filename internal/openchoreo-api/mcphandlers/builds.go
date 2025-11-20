// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

type ListBuildTemplatesResponse struct {
	Templates any `json:"templates"`
}

type ListBuildsResponse struct {
	Builds any `json:"builds"`
}

func (h *MCPHandler) ListBuildTemplates(ctx context.Context, orgName string) (any, error) {
	templates, err := h.Services.BuildService.ListBuildTemplates(ctx, orgName)
	if err != nil {
		return ListBuildTemplatesResponse{}, err
	}
	return ListBuildTemplatesResponse{
		Templates: templates,
	}, nil
}

func (h *MCPHandler) TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (any, error) {
	return h.Services.BuildService.TriggerBuild(ctx, orgName, projectName, componentName, commit)
}

func (h *MCPHandler) ListBuilds(ctx context.Context, orgName, projectName, componentName string) (any, error) {
	builds, err := h.Services.BuildService.ListBuilds(ctx, orgName, projectName, componentName)
	if err != nil {
		return ListBuildsResponse{}, err
	}
	return ListBuildsResponse{
		Builds: builds,
	}, nil
}
