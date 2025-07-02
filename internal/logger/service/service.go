// Copyright (c) 2025 openchoreo
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/openchoreo/openchoreo/internal/logger/config"
	"github.com/openchoreo/openchoreo/internal/logger/opensearch"
)

// OpenSearchClient interface for testing
type OpenSearchClient interface {
	Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error)
	GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error)
	HealthCheck(ctx context.Context) error
}

// LoggingService provides logging functionality
type LoggingService struct {
	osClient     OpenSearchClient
	queryBuilder *opensearch.QueryBuilder
	config       *config.Config
	logger       *zap.Logger
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"total_count"`
	Took       int                   `json:"took_ms"`
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(osClient OpenSearchClient, cfg *config.Config, logger *zap.Logger) *LoggingService {
	return &LoggingService{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		config:       cfg,
		logger:       logger,
	}
}

// GetComponentLogs retrieves logs for a specific component using V2 wildcard search
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.QueryParams) (*LogResponse, error) {
	s.logger.Info("Getting component logs",
		zap.String("component_id", params.ComponentID),
		zap.String("environment_id", params.EnvironmentID),
		zap.String("search_phrase", params.SearchPhrase))

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", zap.Error(err))
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildComponentLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute component logs search", zap.Error(err))
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Component logs retrieved",
		zap.Int("count", len(logs)),
		zap.Int("total", response.Hits.Total.Value))

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetProjectLogs retrieves logs for a specific project using V2 wildcard search
func (s *LoggingService) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	s.logger.Info("Getting project logs",
		zap.String("project_id", params.ProjectID),
		zap.String("environment_id", params.EnvironmentID),
		zap.Strings("component_ids", componentIDs),
		zap.String("search_phrase", params.SearchPhrase))

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", zap.Error(err))
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildProjectLogsQuery(params, componentIDs)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute project logs search", zap.Error(err))
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Project logs retrieved",
		zap.Int("count", len(logs)),
		zap.Int("total", response.Hits.Total.Value))

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetGatewayLogs retrieves gateway logs using V2 wildcard search
func (s *LoggingService) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting gateway logs",
		zap.String("organization_id", params.OrganizationID),
		zap.Strings("gateway_vhosts", params.GatewayVHosts),
		zap.String("search_phrase", params.SearchPhrase))

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", zap.Error(err))
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildGatewayLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute gateway logs search", zap.Error(err))
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Gateway logs retrieved",
		zap.Int("count", len(logs)),
		zap.Int("total", response.Hits.Total.Value))

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetOrganizationLogs retrieves logs for an organization with custom filters
func (s *LoggingService) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting organization logs",
		zap.String("organization_id", params.OrganizationID),
		zap.String("environment_id", params.EnvironmentID),
		zap.Any("pod_labels", podLabels),
		zap.String("search_phrase", params.SearchPhrase))

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", zap.Error(err))
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build organization-specific query
	query := s.queryBuilder.BuildOrganizationLogsQuery(params, podLabels)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute organization logs search", zap.Error(err))
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Organization logs retrieved",
		zap.Int("count", len(logs)),
		zap.Int("total", response.Hits.Total.Value))

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// HealthCheck performs a health check on the service
func (s *LoggingService) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.osClient.HealthCheck(ctx); err != nil {
		s.logger.Error("Health check failed", zap.Error(err))
		return fmt.Errorf("opensearch health check failed: %w", err)
	}

	s.logger.Debug("Health check passed")
	return nil
}
