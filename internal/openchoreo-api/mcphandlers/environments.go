// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListEnvironmentsResponse struct {
	Environments []*models.EnvironmentResponse `json:"environments"`
}

func (h *MCPHandler) ListEnvironments(ctx context.Context, orgName string) (any, error) {
	// For MCP handlers, return all items by paginating through all pages
	var allItems []*models.EnvironmentResponse
	continueToken := ""

	for {
		opts := &models.ListOptions{
			Limit:    models.MaxPageLimit,
			Continue: continueToken,
		}
		result, err := h.Services.EnvironmentService.ListEnvironments(ctx, orgName, opts)
		if err != nil {
			return ListEnvironmentsResponse{}, err
		}

		allItems = append(allItems, result.Items...)

		if !result.Metadata.HasMore {
			break
		}
		continueToken = result.Metadata.Continue
	}

	// Warn if result may be truncated
	h.warnIfTruncated("environments", len(allItems))

	return ListEnvironmentsResponse{
		Environments: allItems,
	}, nil
}

func (h *MCPHandler) GetEnvironment(ctx context.Context, orgName, envName string) (any, error) {
	return h.Services.EnvironmentService.GetEnvironment(ctx, orgName, envName)
}

func (h *MCPHandler) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (any, error) {
	return h.Services.EnvironmentService.CreateEnvironment(ctx, orgName, req)
}
