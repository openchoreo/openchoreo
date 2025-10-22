// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (h *MCPHandler) ListEnvironments(ctx context.Context, orgName string) (string, error) {
	environments, err := h.Services.EnvironmentService.ListEnvironments(ctx, orgName)
	if err != nil {
		return "", err
	}

	return marshalResponse(environments)
}

func (h *MCPHandler) GetEnvironment(ctx context.Context, orgName, envName string) (string, error) {
	environment, err := h.Services.EnvironmentService.GetEnvironment(ctx, orgName, envName)
	if err != nil {
		return "", err
	}

	return marshalResponse(environment)
}

func (h *MCPHandler) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (string, error) {
	environment, err := h.Services.EnvironmentService.CreateEnvironment(ctx, orgName, req)
	if err != nil {
		return "", err
	}

	return marshalResponse(environment)
}
