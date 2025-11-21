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
	environments, err := h.Services.EnvironmentService.ListEnvironments(ctx, orgName)
	if err != nil {
		return ListEnvironmentsResponse{}, err
	}
	return ListEnvironmentsResponse{
		Environments: environments,
	}, nil
}

func (h *MCPHandler) GetEnvironment(ctx context.Context, orgName, envName string) (any, error) {
	return h.Services.EnvironmentService.GetEnvironment(ctx, orgName, envName)
}

func (h *MCPHandler) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (any, error) {
	return h.Services.EnvironmentService.CreateEnvironment(ctx, orgName, req)
}
