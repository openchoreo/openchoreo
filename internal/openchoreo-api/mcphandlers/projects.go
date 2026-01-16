// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListProjectsResponse struct {
	Projects []*models.ProjectResponse `json:"projects"`
}

func (h *MCPHandler) ListProjects(ctx context.Context, namespaceName string) (any, error) {
	projects, err := h.Services.ProjectService.ListProjects(ctx, namespaceName)
	if err != nil {
		return ListProjectsResponse{}, err
	}

	return ListProjectsResponse{
		Projects: projects,
	}, nil
}

func (h *MCPHandler) GetProject(ctx context.Context, namespaceName, projectName string) (any, error) {
	return h.Services.ProjectService.GetProject(ctx, namespaceName, projectName)
}

func (h *MCPHandler) CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (any, error) {
	return h.Services.ProjectService.CreateProject(ctx, namespaceName, req)
}
