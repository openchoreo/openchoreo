// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// LogsService provides logging functionality for the new API
type LogsService struct {
	logsAdapter observability.LogsAdapter
	config      *config.Config
	resolver    *ResourceUIDResolver
	logger      *slog.Logger
}

var (
	// ErrLogsResolveSearchScope indicates a failure while resolving scope/resource identifiers.
	ErrLogsResolveSearchScope = errors.New("logs search scope resolution failed")
	// ErrLogsRetrieval indicates a failure while retrieving logs from the adapter.
	ErrLogsRetrieval = errors.New("logs retrieval failed")
)

// NewLogsService creates a new LogsService instance backed by the HTTP logs adapter.
// The resolver is passed in as it's shared across multiple services.
func NewLogsService(
	logsAdapter observability.LogsAdapter,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*LogsService, error) {
	if logsAdapter == nil {
		return nil, fmt.Errorf("logs adapter is required")
	}
	return &LogsService{
		logsAdapter: logsAdapter,
		config:      cfg,
		resolver:    resolver,
		logger:      logger,
	}, nil
}

// internalSearchScope holds resolved UIDs for internal use
type internalSearchScope struct {
	// For component scope
	NamespaceName  string
	ProjectUID     string
	ComponentUID   string
	EnvironmentUID string

	// For workflow scope
	WorkflowRunName string
	TaskName        string

	// Scope type indicator
	IsWorkflowScope bool
}

// QueryLogs queries logs based on the provided request, forwarding to the logs adapter.
func (s *LogsService) QueryLogs(ctx context.Context, req *types.LogsQueryRequest) (*types.LogsQueryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	s.logger.Info("QueryLogs called",
		"startTime", req.StartTime,
		"endTime", req.EndTime,
		"hasSearchPhrase", req.SearchPhrase != "",
		"limit", req.Limit)

	// Convert request to internal representation with resolved UIDs
	scope, err := resolveSearchScope(ctx, s.resolver, req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrLogsResolveSearchScope, err)
	}

	// Parse time parameters
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		s.logger.Error("Failed to parse start time", "error", err)
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		s.logger.Error("Failed to parse end time", "error", err)
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	// Route to appropriate handler based on scope type
	if scope.IsWorkflowScope {
		return s.queryWorkflowLogs(ctx, scope, startTime, endTime, req)
	}
	return s.queryComponentLogs(ctx, scope, startTime, endTime, req)
}

// queryComponentLogs handles component log queries
func (s *LogsService) queryComponentLogs(
	ctx context.Context,
	scope *internalSearchScope,
	startTime, endTime time.Time,
	req *types.LogsQueryRequest,
) (*types.LogsQueryResponse, error) {
	s.logger.Debug("Component search scope",
		"namespaceName", scope.NamespaceName,
		"projectUid", scope.ProjectUID,
		"componentUid", scope.ComponentUID,
		"environmentUid", scope.EnvironmentUID)

	params := observability.ComponentApplicationLogsParams{
		ComponentID:   scope.ComponentUID,
		EnvironmentID: scope.EnvironmentUID,
		ProjectID:     scope.ProjectUID,
		Namespace:     scope.NamespaceName,
		StartTime:     startTime,
		EndTime:       endTime,
		SearchPhrase:  req.SearchPhrase,
		LogLevels:     req.LogLevels,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
	}

	result, err := s.logsAdapter.GetComponentApplicationLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get component logs from adapter", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrLogsRetrieval, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: component logs adapter returned nil result", ErrLogsRetrieval)
	}

	s.logger.Debug("Component logs retrieved from adapter",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return s.convertComponentLogsToResponse(result), nil
}

// queryWorkflowLogs handles workflow log queries
func (s *LogsService) queryWorkflowLogs(
	ctx context.Context,
	scope *internalSearchScope,
	startTime, endTime time.Time,
	req *types.LogsQueryRequest,
) (*types.LogsQueryResponse, error) {
	s.logger.Debug("Workflow search scope",
		"namespaceName", scope.NamespaceName,
		"workflowRunName", scope.WorkflowRunName)

	params := observability.WorkflowLogsParams{
		Namespace:       scope.NamespaceName,
		WorkflowRunName: scope.WorkflowRunName,
		TaskName:        scope.TaskName,
		StartTime:       startTime,
		EndTime:         endTime,
		SearchPhrase:    req.SearchPhrase,
		LogLevels:       req.LogLevels,
		Limit:           req.Limit,
		SortOrder:       req.SortOrder,
	}

	result, err := s.logsAdapter.GetWorkflowLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get workflow logs from adapter", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrLogsRetrieval, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: workflow logs adapter returned nil result", ErrLogsRetrieval)
	}

	s.logger.Debug("Workflow logs retrieved from adapter",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return s.convertWorkflowLogsToResponse(result), nil
}

// convertComponentLogsToResponse converts component logs result to types response
func (s *LogsService) convertComponentLogsToResponse(
	result *observability.ComponentApplicationLogsResult,
) *types.LogsQueryResponse {
	logs := make([]types.LogEntry, 0, len(result.Logs))
	for _, log := range result.Logs {
		logs = append(logs, types.LogEntry{
			Timestamp: log.Timestamp.Format(time.RFC3339),
			Log:       log.Log,
			Level:     log.LogLevel,
			Metadata: &types.LogMetadata{
				ComponentName:   log.ComponentName,
				ProjectName:     log.ProjectName,
				EnvironmentName: log.EnvironmentName,
				NamespaceName:   log.NamespaceName,
				ComponentUID:    log.ComponentID,
				ProjectUID:      log.ProjectID,
				EnvironmentUID:  log.EnvironmentID,
				ContainerName:   log.ContainerName,
				PodName:         log.PodName,
				PodNamespace:    log.PodNamespace,
			},
		})
	}

	return &types.LogsQueryResponse{
		Logs:   logs,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

// convertWorkflowLogsToResponse converts workflow logs result to types response
func (s *LogsService) convertWorkflowLogsToResponse(
	result *observability.WorkflowLogsResult,
) *types.LogsQueryResponse {
	logs := make([]types.LogEntry, 0, len(result.Logs))
	for _, log := range result.Logs {
		logs = append(logs, types.LogEntry{
			Timestamp: log.Timestamp.Format(time.RFC3339),
			Log:       log.Log,
			Level:     log.LogLevel,
		})
	}

	return &types.LogsQueryResponse{
		Logs:   logs,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

// resolveSearchScope converts a search scope to its internal representation with resolved UIDs
func resolveSearchScope(
	ctx context.Context,
	resolver *ResourceUIDResolver,
	searchScope *types.SearchScope,
) (*internalSearchScope, error) {
	if searchScope == nil {
		return nil, fmt.Errorf("search scope is required")
	}

	if searchScope.Workflow != nil {
		scope := searchScope.Workflow
		return &internalSearchScope{
			NamespaceName:   scope.Namespace,
			WorkflowRunName: scope.WorkflowRunName,
			TaskName:        scope.TaskName,
			IsWorkflowScope: true,
		}, nil
	}

	if searchScope.Component != nil {
		if resolver == nil {
			return nil, fmt.Errorf("resource UID resolver is not initialized")
		}
		scope := searchScope.Component
		projectUID, err := resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return nil, err
		}
		componentUID, err := resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return nil, err
		}
		environmentUID, err := resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
		if err != nil {
			return nil, err
		}
		return &internalSearchScope{
			NamespaceName:   scope.Namespace,
			ProjectUID:      projectUID,
			ComponentUID:    componentUID,
			EnvironmentUID:  environmentUID,
			IsWorkflowScope: false,
		}, nil
	}

	return nil, fmt.Errorf("invalid search scope")
}
