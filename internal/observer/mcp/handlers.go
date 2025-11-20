// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

// GetComponentTraces retrieves distributed tracing spans for a specific component
func (h *MCPHandler) GetComponentTraces(ctx context.Context, params opensearch.ComponentTracesRequestParams) (string, error) {
	result, err := h.Service.GetComponentTraces(ctx, params)
	if err != nil {
		return "", err
	}

	return marshalResponse(result)
}

// GetComponentResourceMetrics retrieves resource usage metrics for a component
func (h *MCPHandler) GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (string, error) {
	// Parse time strings to time.Time
	startTimeObj, err := parseRFC3339Time(startTime)
	if err != nil {
		return "", err
	}

	endTimeObj, err := parseRFC3339Time(endTime)
	if err != nil {
		return "", err
	}

	result, err := h.Service.GetComponentResourceMetrics(ctx, componentID, environmentID, projectID, startTimeObj, endTimeObj)
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

func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return t, nil
}
