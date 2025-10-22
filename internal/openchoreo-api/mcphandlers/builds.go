// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

func (h *MCPHandler) ListBuildTemplates(ctx context.Context, orgName string) (string, error) {
	templates, err := h.Services.BuildService.ListBuildTemplates(ctx, orgName)
	if err != nil {
		return "", err
	}

	return marshalResponse(templates)
}

func (h *MCPHandler) TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (string, error) {
	build, err := h.Services.BuildService.TriggerBuild(ctx, orgName, projectName, componentName, commit)
	if err != nil {
		return "", err
	}

	return marshalResponse(build)
}

func (h *MCPHandler) ListBuilds(ctx context.Context, orgName, projectName, componentName string) (string, error) {
	builds, err := h.Services.BuildService.ListBuilds(ctx, orgName, projectName, componentName)
	if err != nil {
		return "", err
	}

	return marshalResponse(builds)
}
