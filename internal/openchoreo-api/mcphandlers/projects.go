// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (h *MCPHandler) ListProjects(ctx context.Context, orgName string) (string, error) {
	projects, err := h.Services.ProjectService.ListProjects(ctx, orgName)
	if err != nil {
		return "", err
	}

	return marshalResponse(projects)
}

func (h *MCPHandler) GetProject(ctx context.Context, orgName, projectName string) (string, error) {
	project, err := h.Services.ProjectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		return "", err
	}

	return marshalResponse(project)
}

func (h *MCPHandler) CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (string, error) {
	project, err := h.Services.ProjectService.CreateProject(ctx, orgName, req)
	if err != nil {
		return "", err
	}

	return marshalResponse(project)
}
