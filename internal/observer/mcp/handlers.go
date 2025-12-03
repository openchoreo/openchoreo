// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

type MCPHandler struct {
	Service *service.LoggingService
}

// GetComponentLogs retrieves logs for a specific component
func (h *MCPHandler) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (any, error) {
	return h.Service.GetComponentLogs(ctx, params)
}

// GetProjectLogs retrieves logs for a specific project
func (h *MCPHandler) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (any, error) {
	return h.Service.GetProjectLogs(ctx, params, componentIDs)
}

// GetGatewayLogs retrieves gateway logs
func (h *MCPHandler) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (any, error) {
	return h.Service.GetGatewayLogs(ctx, params)
}

// GetOrganizationLogs retrieves logs for an entire organization
func (h *MCPHandler) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (any, error) {
	return h.Service.GetOrganizationLogs(ctx, params, podLabels)
}

// GetTraces retrieves distributed tracing spans for a specific component
func (h *MCPHandler) GetTraces(ctx context.Context, params opensearch.TracesRequestParams) (any, error) {
	return h.Service.GetTraces(ctx, params)
}

// GetComponentResourceMetrics retrieves resource usage metrics for a component
func (h *MCPHandler) GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error) {
	// Parse time strings to time.Time
	startTimeObj, err := parseRFC3339Time(startTime)
	if err != nil {
		return nil, err
	}

	endTimeObj, err := parseRFC3339Time(endTime)
	if err != nil {
		return nil, err
	}

	return h.Service.GetComponentResourceMetrics(ctx, componentID, environmentID, projectID, startTimeObj, endTimeObj)
}

// GetComponentHTTPMetrics retrieves HTTP metrics for a component
func (h *MCPHandler) GetComponentHTTPMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error) {
	// Parse time strings to time.Time
	startTimeObj, err := parseRFC3339Time(startTime)
	if err != nil {
		return nil, err
	}

	endTimeObj, err := parseRFC3339Time(endTime)
	if err != nil {
		return nil, err
	}

	return h.Service.GetComponentHTTPMetrics(ctx, componentID, environmentID, projectID, startTimeObj, endTimeObj)
}

func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return t, nil
}
