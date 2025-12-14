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

func (h *MCPHandler) ListProjects(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.ProjectResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.ProjectService.ListProjects(ctx, orgName, opts)
		if err != nil {
			return ListProjectsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("projects", len(allItems))

	return ListProjectsResponse{
		Projects: allItems,
	}, nil
}

func (h *MCPHandler) GetProject(ctx context.Context, orgName, projectName string) (any, error) {
	return h.Services.ProjectService.GetProject(ctx, orgName, projectName)
}

func (h *MCPHandler) CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (any, error) {
	return h.Services.ProjectService.CreateProject(ctx, orgName, req)
}
