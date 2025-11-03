// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"

	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

type MCPHandler struct {
	Service *service.LoggingService
}

// GetComponentLogs retrieves logs for a specific component
func (h *MCPHandler) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (string, error) {
	result, err := h.Service.GetComponentLogs(ctx, params)
	if err != nil {
		return "", err
	}

	return marshalResponse(result)
}

// GetProjectLogs retrieves logs for a specific project
func (h *MCPHandler) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (string, error) {
	result, err := h.Service.GetProjectLogs(ctx, params, componentIDs)
	if err != nil {
		return "", err
	}

	return marshalResponse(result)
}

// GetGatewayLogs retrieves gateway logs
func (h *MCPHandler) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (string, error) {
	result, err := h.Service.GetGatewayLogs(ctx, params)
	if err != nil {
		return "", err
	}

	return marshalResponse(result)
}

// GetOrganizationLogs retrieves logs for an entire organization
func (h *MCPHandler) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (string, error) {
	result, err := h.Service.GetOrganizationLogs(ctx, params, podLabels)
	if err != nil {
		return "", err
	}

	return marshalResponse(result)
}

func marshalResponse(data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}
