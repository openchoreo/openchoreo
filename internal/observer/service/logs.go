// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/adaptor"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// LogsService provides logging functionality for the new API
type LogsService struct {
	logsBackend    observability.LogsBackend
	defaultAdaptor *adaptor.DefaultLogsAdaptor
	config         *config.Config
	resolver       *ResourceUIDResolver
	logger         *slog.Logger
}

var (
	// ErrLogsResolveSearchScope indicates a failure while resolving scope/resource identifiers.
	ErrLogsResolveSearchScope = errors.New("logs search scope resolution failed")
	// ErrLogsRetrieval indicates a failure while retrieving logs from backend or adaptor.
	ErrLogsRetrieval = errors.New("logs retrieval failed")
)

// NewLogsService creates a new LogsService instance.
// It initializes its own DefaultLogsAdaptor internally for OpenSearch queries.
// The resolver is passed in as it's shared across multiple services.
func NewLogsService(
	logsBackend observability.LogsBackend,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*LogsService, error) {
	var defaultAdaptor *adaptor.DefaultLogsAdaptor
	// Initialize default logs adaptor (queries OpenSearch when logs backend is not enabled)
	if !cfg.Experimental.UseLogsBackend || logsBackend == nil {
		var err error
		defaultAdaptor, err = adaptor.NewDefaultLogsAdaptor(&cfg.OpenSearch, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default logs adaptor: %w", err)
		}
	}

	return &LogsService{
		logsBackend:    logsBackend,
		defaultAdaptor: defaultAdaptor,
		config:         cfg,
		resolver:       resolver,
		logger:         logger,
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

	// Scope type indicator
	IsWorkflowScope bool
}

// QueryLogs queries logs based on the provided request
// If experimental.use.logs.backend is enabled, uses logs backend
// Otherwise, falls back to the default adaptor
func (s *LogsService) QueryLogs(ctx context.Context, req *types.LogsQueryRequest) (*types.LogsQueryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	s.logger.Info("QueryLogs called",
		"startTime", req.StartTime,
		"endTime", req.EndTime,
		"hasSearchPhrase", req.SearchPhrase != "",
		"limit", req.Limit,
		"useLogsBackend", s.config.Experimental.UseLogsBackend)

	// Convert request to internal representation with resolved UIDs
	scope, err := s.resolveSearchScope(ctx, req.SearchScope)
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

	// Build backend params for component logs
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

	var result *observability.ComponentApplicationLogsResult
	var err error

	// Check if backend is enabled and available
	if s.config.Experimental.UseLogsBackend && s.logsBackend != nil {
		s.logger.Debug("Using logs backend for component logs query")
		result, err = s.getComponentLogsFromBackend(ctx, params)
	} else {
		s.logger.Debug("Using default adaptor for component logs query")
		result, err = s.getComponentLogsFromDefaultAdaptor(ctx, params)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLogsRetrieval, err)
	}

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

	// Build backend params for workflow logs
	params := observability.WorkflowLogsParams{
		Namespace:       scope.NamespaceName,
		WorkflowRunName: scope.WorkflowRunName,
		StartTime:       startTime,
		EndTime:         endTime,
		SearchPhrase:    req.SearchPhrase,
		LogLevels:       req.LogLevels,
		Limit:           req.Limit,
		SortOrder:       req.SortOrder,
	}

	var result *observability.WorkflowLogsResult
	var err error

	// Check if backend is enabled and available
	if s.config.Experimental.UseLogsBackend && s.logsBackend != nil {
		s.logger.Debug("Using logs backend for workflow logs query")
		result, err = s.getWorkflowLogsFromBackend(ctx, params)
	} else {
		s.logger.Debug("Using default adaptor for workflow logs query")
		result, err = s.getWorkflowLogsFromDefaultAdaptor(ctx, params)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLogsRetrieval, err)
	}

	return s.convertWorkflowLogsToResponse(result, scope), nil
}

// getComponentLogsFromBackend fetches component logs from the configured logs backend
func (s *LogsService) getComponentLogsFromBackend(
	ctx context.Context,
	params observability.ComponentApplicationLogsParams,
) (*observability.ComponentApplicationLogsResult, error) {
	result, err := s.logsBackend.GetComponentApplicationLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get component logs from backend", "error", err)
		return nil, fmt.Errorf("failed to get component logs from backend: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("component logs backend returned nil result")
	}

	s.logger.Debug("Component logs retrieved from backend",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return result, nil
}

// getComponentLogsFromDefaultAdaptor fetches component logs from the default adaptor
func (s *LogsService) getComponentLogsFromDefaultAdaptor(
	ctx context.Context,
	params observability.ComponentApplicationLogsParams,
) (*observability.ComponentApplicationLogsResult, error) {
	if s.defaultAdaptor == nil {
		return nil, fmt.Errorf("default adaptor is not initialized")
	}
	result, err := s.defaultAdaptor.GetComponentApplicationLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get component logs from default adaptor", "error", err)
		return nil, fmt.Errorf("failed to get component logs from default adaptor: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("default adaptor returned nil result for component logs")
	}

	s.logger.Debug("Component logs retrieved from default adaptor",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return result, nil
}

// getWorkflowLogsFromBackend fetches workflow logs from the configured logs backend
func (s *LogsService) getWorkflowLogsFromBackend(
	ctx context.Context,
	params observability.WorkflowLogsParams,
) (*observability.WorkflowLogsResult, error) {
	if s.logsBackend == nil {
		return nil, fmt.Errorf("logs backend is not initialized")
	}
	result, err := s.logsBackend.GetWorkflowLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get workflow logs from backend", "error", err)
		return nil, fmt.Errorf("failed to get workflow logs from backend: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("workflow logs backend returned nil result")
	}

	s.logger.Debug("Workflow logs retrieved from backend",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return result, nil
}

// getWorkflowLogsFromDefaultAdaptor fetches workflow logs from the default adaptor
func (s *LogsService) getWorkflowLogsFromDefaultAdaptor(
	ctx context.Context,
	params observability.WorkflowLogsParams,
) (*observability.WorkflowLogsResult, error) {
	if s.defaultAdaptor == nil {
		return nil, fmt.Errorf("default adaptor is not initialized")
	}
	result, err := s.defaultAdaptor.GetWorkflowLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get workflow logs from default adaptor", "error", err)
		return nil, fmt.Errorf("failed to get workflow logs from default adaptor: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("default adaptor returned nil result for workflow logs")
	}

	s.logger.Debug("Workflow logs retrieved from default adaptor",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return result, nil
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
	scope *internalSearchScope,
) *types.LogsQueryResponse {
	logs := make([]types.LogEntry, 0, len(result.Logs))
	for _, log := range result.Logs {
		logs = append(logs, types.LogEntry{
			Timestamp: log.Timestamp.Format(time.RFC3339),
			Log:       log.Log,
			Level:     log.LogLevel,
			Metadata: &types.LogMetadata{
				NamespaceName: scope.NamespaceName,
				PodName:       log.PodName,
				PodNamespace:  log.PodNamespace,
			},
		})
	}

	return &types.LogsQueryResponse{
		Logs:   logs,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

// resolveSearchScope converts search scope to internal representation with resolved UIDs
func (s *LogsService) resolveSearchScope(ctx context.Context, searchScope *types.SearchScope) (*internalSearchScope, error) {
	if searchScope == nil {
		return nil, fmt.Errorf("search scope is required")
	}

	if searchScope.Workflow != nil {
		scope := searchScope.Workflow
		return &internalSearchScope{
			NamespaceName:   scope.Namespace,
			WorkflowRunName: scope.WorkflowRunName,
			IsWorkflowScope: true,
		}, nil
	}

	if searchScope.Component != nil {
		if s.resolver == nil {
			return nil, fmt.Errorf("resource UID resolver is not initialized")
		}
		scope := searchScope.Component
		projectUID, err := s.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return nil, err
		}
		componentUID, err := s.resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return nil, err
		}
		environmentUID, err := s.resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
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
