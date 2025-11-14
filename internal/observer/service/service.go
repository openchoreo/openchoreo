// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/rca"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	logger       *slog.Logger
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"totalCount"`
	Took       int                   `json:"tookMs"`
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(osClient OpenSearchClient, cfg *config.Config, logger *slog.Logger) *LoggingService {
	return &LoggingService{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		config:       cfg,
		logger:       logger,
	}
}

// RCAService provides AI-powered root cause analysis functionality
type RCAService struct {
	k8sClient client.Client
	config    *config.Config
	logger    *slog.Logger
}

// NewRCAService creates a new RCA service instance
func NewRCAService(k8sClient client.Client, cfg *config.Config, logger *slog.Logger) *RCAService {
	return &RCAService{
		k8sClient: k8sClient,
		config:    cfg,
		logger:    logger,
	}
}

// GetComponentLogs retrieves logs for a specific component using V2 wildcard search
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting component logs",
		"component_id", params.ComponentID,
		"environment_id", params.EnvironmentID,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildComponentLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute component logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Component logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetProjectLogs retrieves logs for a specific project using V2 wildcard search
func (s *LoggingService) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	s.logger.Info("Getting project logs",
		"project_id", params.ProjectID,
		"environment_id", params.EnvironmentID,
		"component_ids", componentIDs,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildProjectLogsQuery(params, componentIDs)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute project logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Project logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetGatewayLogs retrieves gateway logs using V2 wildcard search
func (s *LoggingService) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting gateway logs",
		"organization_id", params.OrganizationID,
		"gateway_vhosts", params.GatewayVHosts,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildGatewayLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute gateway logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Gateway logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetOrganizationLogs retrieves logs for an organization with custom filters
func (s *LoggingService) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting organization logs",
		"organization_id", params.OrganizationID,
		"environment_id", params.EnvironmentID,
		"pod_labels", podLabels,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build organization-specific query
	query := s.queryBuilder.BuildOrganizationLogsQuery(params, podLabels)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute organization logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Organization logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

func (s *LoggingService) GetComponentTraces(ctx context.Context, params opensearch.ComponentTracesRequestParams) (*opensearch.TraceResponse, error) {
	s.logger.Info("Getting component traces",
		"serviceName", params.ServiceName)

	// Build component traces query
	query := s.queryBuilder.BuildComponentTracesQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, []string{"otel-v1-apm-span"}, query)
	if err != nil {
		s.logger.Error("Failed to execute component traces search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	traces := make([]opensearch.Span, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		span := opensearch.ParseSpanEntry(hit)
		traces = append(traces, span)
	}

	s.logger.Info("Component traces retrieved",
		"count", len(traces),
		"total", response.Hits.Total.Value)

	return &opensearch.TraceResponse{
		Spans:      traces,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// HealthCheck performs a health check on the service
func (s *LoggingService) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.osClient.HealthCheck(ctx); err != nil {
		s.logger.Error("Health check failed", "error", err)
		return fmt.Errorf("opensearch health check failed: %w", err)
	}

	s.logger.Debug("Health check passed")
	return nil
}

// RCARequest represents a request to perform AI-powered root cause analysis
type RCARequest struct {
	RCAID    string          `json:"rca_id"`
	Metadata json.RawMessage `json:"metadata"`
}

// RCAResponse represents the response from creating an RCA job
type RCAResponse struct {
	JobName      string            `json:"jobName"`
	JobNamespace string            `json:"jobNamespace"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	Labels       map[string]string `json:"labels"`
}

// TriggerRCA creates a Kubernetes job to perform root cause analysis
func (s *RCAService) TriggerRCA(ctx context.Context, req RCARequest) (*RCAResponse, error) {
	if !s.config.RCA.Enabled {
		return nil, fmt.Errorf("AI RCA feature is not enabled")
	}

	// Check if k8s client is initialized
	if s.k8sClient == nil {
		s.logger.Error("Kubernetes client is not initialized")
		return nil, fmt.Errorf("kubernetes client not available")
	}

	s.logger.Info("Creating AI RCA job",
		"rca_id", req.RCAID)

	jobName := fmt.Sprintf("rca-%s", req.RCAID)

	// RCA job specification
	jobSpec := rca.JobSpec{
		Name:                    jobName,
		Namespace:               s.config.RCA.Namespace,
		ImageRepository:         s.config.RCA.ImageRepository,
		ImageTag:                s.config.RCA.ImageTag,
		ImagePullPolicy:         s.config.RCA.ImagePullPolicy,
		TTLSecondsAfterFinished: &s.config.RCA.TTLSecondsAfterFinished,
		ResourceLimitsCPU:       s.config.RCA.ResourceLimitsCPU,
		ResourceLimitsMemory:    s.config.RCA.ResourceLimitsMemory,
		ResourceRequestsCPU:     s.config.RCA.ResourceRequestsCPU,
		ResourceRequestsMemory:  s.config.RCA.ResourceRequestsMemory,
	}

	// RCA context
	rcaContext := rca.RCAContext{
		RCAID:    req.RCAID,
		Metadata: req.Metadata,
	}

	// Create the job
	job, err := rca.CreateJob(ctx, s.k8sClient, jobSpec, rcaContext)
	if err != nil {
		s.logger.Error("Failed to create RCA job", "error", err)
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	s.logger.Info("RCA job created successfully", "job_name", job.Name, "namespace", job.Namespace)

	response := &RCAResponse{
		JobName:      job.Name,
		JobNamespace: job.Namespace,
		Status:       "Created",
		CreatedAt:    job.CreationTimestamp.Format(time.RFC3339),
		Labels:       job.Labels,
	}

	return response, nil
}
