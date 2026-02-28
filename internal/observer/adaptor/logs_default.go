// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// DefaultLogsAdaptor queries logs from OpenSearch when the external logs backend is not enabled.
// It creates and manages its own OpenSearch client internally.
type DefaultLogsAdaptor struct {
	osClient     *opensearch.Client
	queryBuilder *opensearch.QueryBuilder
	logger       *slog.Logger
}

// NewDefaultLogsAdaptor creates a new DefaultLogsAdaptor instance with its own OpenSearch client
func NewDefaultLogsAdaptor(cfg *config.OpenSearchConfig, logger *slog.Logger) (*DefaultLogsAdaptor, error) {
	osClient, err := opensearch.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client for default adaptor: %w", err)
	}

	return &DefaultLogsAdaptor{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder(cfg.IndexPrefix),
		logger:       logger,
	}, nil
}

// GetComponentApplicationLogs retrieves component application logs from OpenSearch.
func (a *DefaultLogsAdaptor) GetComponentApplicationLogs(
	ctx context.Context,
	params observability.ComponentApplicationLogsParams,
) (*observability.ComponentApplicationLogsResult, error) {
	a.logger.Debug("GetComponentApplicationLogs called on default adaptor",
		"componentId", params.ComponentID,
		"environmentId", params.EnvironmentID,
		"projectId", params.ProjectID,
		"namespace", params.Namespace,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"hasSearchPhrase", params.SearchPhrase != "",
		"limit", params.Limit)

	startTimeStr := params.StartTime.Format(time.RFC3339)
	endTimeStr := params.EndTime.Format(time.RFC3339)

	// Convert observability params to the new API query params
	// This properly handles optional filters (only adds term filters when values are non-empty)
	queryParams := opensearch.ComponentLogsQueryParamsV1{
		StartTime:     startTimeStr,
		EndTime:       endTimeStr,
		NamespaceName: params.Namespace,     // OpenChoreo namespace name
		ProjectID:     params.ProjectID,     // Will be empty if not resolved
		ComponentID:   params.ComponentID,   // Will be empty if not resolved
		EnvironmentID: params.EnvironmentID, // Will be empty if not resolved
		SearchPhrase:  params.SearchPhrase,
		LogLevels:     params.LogLevels,
		Limit:         params.Limit,
		SortOrder:     params.SortOrder,
	}

	// Generate indices based on time range
	indices, err := a.queryBuilder.GenerateIndices(startTimeStr, endTimeStr)
	if err != nil {
		a.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query using the new API method that properly handles optional filters
	query, err := a.queryBuilder.BuildComponentLogsQueryV1(queryParams)
	if err != nil {
		a.logger.Error("Failed to build component logs query", "error", err)
		return nil, fmt.Errorf("failed to build component logs query: %w", err)
	}

	// Execute search
	response, err := a.osClient.Search(ctx, indices, query)
	if err != nil {
		a.logger.Error("Failed to execute component logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]observability.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		osEntry := opensearch.ParseLogEntry(hit)
		// Convert opensearch.LogEntry to observability.LogEntry
		logs = append(logs, observability.LogEntry{
			Timestamp: osEntry.Timestamp,
			Log:       osEntry.Log,
			LogLevel:  osEntry.LogLevel,
			// UIDs
			ComponentID:   osEntry.ComponentID,
			EnvironmentID: osEntry.EnvironmentID,
			ProjectID:     osEntry.ProjectID,
			// Names
			ComponentName:   osEntry.ComponentName,
			EnvironmentName: osEntry.EnvironmentName,
			ProjectName:     osEntry.ProjectName,
			NamespaceName:   osEntry.NamespaceName,
			// Other fields
			Version:       osEntry.Version,
			VersionID:     osEntry.VersionID,
			PodNamespace:  osEntry.PodNamespace,
			PodID:         osEntry.PodID,
			PodName:       osEntry.PodName,
			ContainerName: osEntry.ContainerName,
			Labels:        osEntry.Labels,
		})
	}

	a.logger.Debug("Component logs retrieved from OpenSearch",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &observability.ComponentApplicationLogsResult{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetWorkflowLogs retrieves workflow logs from OpenSearch.
func (a *DefaultLogsAdaptor) GetWorkflowLogs(
	ctx context.Context,
	params observability.WorkflowLogsParams,
) (*observability.WorkflowLogsResult, error) {
	a.logger.Debug("GetWorkflowLogs called on default adaptor",
		"namespace", params.Namespace,
		"workflowRunName", params.WorkflowRunName,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"searchPhrase", params.SearchPhrase,
		"limit", params.Limit)

	// Convert observability params to opensearch query params
	queryParams := opensearch.WorkflowRunQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime:     params.StartTime.Format(time.RFC3339),
			EndTime:       params.EndTime.Format(time.RFC3339),
			SearchPhrase:  params.SearchPhrase,
			LogLevels:     params.LogLevels,
			Limit:         params.Limit,
			SortOrder:     params.SortOrder,
			NamespaceName: params.Namespace,
		},
		WorkflowRunID: params.WorkflowRunName,
	}

	// Set default limit if not specified
	if queryParams.Limit <= 0 {
		queryParams.Limit = 100
	}

	// Set default sort order if not specified
	if queryParams.SortOrder == "" {
		queryParams.SortOrder = "desc"
	}

	// Generate indices based on time range
	indices, err := a.queryBuilder.GenerateIndices(queryParams.StartTime, queryParams.EndTime)
	if err != nil {
		a.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := a.queryBuilder.BuildWorkflowRunLogsQuery(queryParams)

	// Execute search
	response, err := a.osClient.Search(ctx, indices, query)
	if err != nil {
		a.logger.Error("Failed to execute workflow logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]observability.WorkflowLogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		osEntry := opensearch.ParseLogEntry(hit)
		// Convert opensearch.LogEntry to observability.WorkflowLogEntry
		logs = append(logs, observability.WorkflowLogEntry{
			Timestamp:     osEntry.Timestamp,
			Log:           osEntry.Log,
			LogLevel:      osEntry.LogLevel,
			PodNamespace:  osEntry.PodNamespace,
			PodID:         osEntry.PodID,
			PodName:       osEntry.PodName,
			ContainerName: osEntry.ContainerName,
			Labels:        osEntry.Labels,
		})
	}

	a.logger.Debug("Workflow logs retrieved from OpenSearch",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &observability.WorkflowLogsResult{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}
