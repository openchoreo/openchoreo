// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	observerlabels "github.com/openchoreo/openchoreo/internal/observer/labels"
	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
	observertypes "github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

const (
	logLevelDebug            = "debug"
	alertRuleActionCreated   = "created"
	alertRuleActionUpdated   = "updated"
	alertRuleActionUnchanged = "unchanged"
)

// OpenSearchClient interface for testing
type OpenSearchClient interface {
	Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error)
	GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error)
	SearchMonitorByName(ctx context.Context, name string) (id string, exists bool, err error)
	GetMonitorByID(ctx context.Context, monitorID string) (monitor map[string]interface{}, err error)
	CreateMonitor(ctx context.Context, monitor map[string]interface{}) (id string, lastUpdateTime int64, err error)
	UpdateMonitor(ctx context.Context, monitorID string, monitor map[string]interface{}) (lastUpdateTime int64, err error)
	DeleteMonitor(ctx context.Context, monitorID string) error
	WriteAlertEntry(ctx context.Context, entry map[string]interface{}) (string, error)
	HealthCheck(ctx context.Context) error
}

// LoggingService provides logging and metrics functionality
type LoggingService struct {
	osClient       OpenSearchClient
	queryBuilder   *opensearch.QueryBuilder
	metricsService *prometheus.MetricsService
	k8sClient      client.Client
	config         *config.Config
	logger         *slog.Logger
	logsBackend    observability.LogsBackend // Optional: Logs backend for fetching logs
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"totalCount"`
	Took       int                   `json:"tookMs"`
}

// HTTPMetricsTimeSeries represents HTTP metrics as time series data. This is what will be returned by the
// POST /api/metrics/component/http API
type HTTPMetricsTimeSeries struct {
	LatencyPercentile50th    []prometheus.TimeValuePoint `json:"latencyPercentile50th"`
	LatencyPercentile90th    []prometheus.TimeValuePoint `json:"latencyPercentile90th"`
	LatencyPercentile99th    []prometheus.TimeValuePoint `json:"latencyPercentile99th"`
	MeanLatency              []prometheus.TimeValuePoint `json:"meanLatency"`
	RequestCount             []prometheus.TimeValuePoint `json:"requestCount"`
	SuccessfulRequestCount   []prometheus.TimeValuePoint `json:"successfulRequestCount"`
	UnsuccessfulRequestCount []prometheus.TimeValuePoint `json:"unsuccessfulRequestCount"`
}

// NewLoggingService creates a new logging service instance
// logsBackend is optional - if nil, OpenSearch will be used for component logs
func NewLoggingService(osClient OpenSearchClient, metricsService *prometheus.MetricsService, k8sClient client.Client, cfg *config.Config, logger *slog.Logger, logsBackend observability.LogsBackend) *LoggingService {
	return &LoggingService{
		osClient:       osClient,
		queryBuilder:   opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		metricsService: metricsService,
		k8sClient:      k8sClient,
		config:         cfg,
		logger:         logger,
		logsBackend:    logsBackend,
	}
}

// GetBuildLogs retrieves logs for a specific build using V2 wildcard search
func (s *LoggingService) GetBuildLogs(ctx context.Context, params opensearch.BuildQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting build logs for build_id: " + params.BuildID)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildBuildLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute build logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Build logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// uuidRegex matches standard UUID format (8-4-4-4-12 hex characters)
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// resolveWorkflowRunName resolves a workflow run identifier to its name.
// If the identifier is a UUID, it looks up the WorkflowRun CR by UID and returns its name.
// If it's not a UUID, it returns the identifier as-is (assuming it's already the name).
func (s *LoggingService) resolveWorkflowRunName(ctx context.Context, runID, namespaceName string) string {
	// If it doesn't look like a UUID, assume it's already the name
	if !isUUID(runID) {
		s.logger.Debug("Run ID is not a UUID, using as name", "run_id", runID)
		return runID
	}

	// If we don't have a k8s client, we can't resolve the UUID
	if s.k8sClient == nil {
		s.logger.Warn("Cannot resolve UUID to name: k8s client not configured, using UUID as-is", "run_id", runID)
		return runID
	}

	s.logger.Debug("Run ID appears to be a UUID, looking up WorkflowRun name",
		"run_id", runID,
		"namespace", namespaceName)

	// List WorkflowRuns in the namespace and find the one with matching UID
	var wfRunList choreoapis.WorkflowRunList
	if err := s.k8sClient.List(ctx, &wfRunList, client.InNamespace(namespaceName)); err != nil {
		s.logger.Warn("Failed to list WorkflowRuns, using UUID as-is",
			"error", err,
			"namespace", namespaceName,
			"run_id", runID)
		return runID
	}

	// Find the WorkflowRun with matching UID
	for _, wfRun := range wfRunList.Items {
		if string(wfRun.UID) == runID {
			s.logger.Debug("Resolved UUID to WorkflowRun name",
				"uuid", runID,
				"name", wfRun.Name)
			return wfRun.Name
		}
	}

	s.logger.Warn("WorkflowRun not found by UUID, using UUID as-is",
		"run_id", runID,
		"namespace", namespaceName)
	return runID
}

// GetWorkflowRunLogs retrieves logs for a specific workflow run using V2 wildcard search
func (s *LoggingService) GetWorkflowRunLogs(ctx context.Context, params opensearch.WorkflowRunQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting workflow run logs",
		"workflow_run_id", params.WorkflowRunID,
		"namespace_name", params.NamespaceName)

	// Resolve workflow run ID to name if it's a UUID
	resolvedName := s.resolveWorkflowRunName(ctx, params.WorkflowRunID, params.NamespaceName)

	// Update params with resolved name
	if resolvedName != params.WorkflowRunID {
		s.logger.Info("Resolved workflow run UUID to name",
			"uuid", params.WorkflowRunID,
			"name", resolvedName)
		params.WorkflowRunID = resolvedName
	}

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildWorkflowRunLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute workflow run logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Workflow run logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetComponentLogs retrieves logs for a specific component
// If experimental.use.logs.backend is enabled, uses logs backend
// Otherwise, falls back to OpenSearch
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (*observability.ComponentApplicationLogsResult, error) {
	s.logger.Info("Getting component logs",
		"component_id", params.ComponentID,
		"environment_id", params.EnvironmentID,
		"search_phrase", params.SearchPhrase,
		"use_backend", s.config.Experimental.UseLogsBackend)

	// Check if backend is enabled and available
	if s.config.Experimental.UseLogsBackend && s.logsBackend != nil {
		// Parse time parameters for backend
		startTime, err := time.Parse(time.RFC3339, params.StartTime)
		if err != nil {
			s.logger.Error("Failed to parse start time", "error", err)
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}
		endTime, err := time.Parse(time.RFC3339, params.EndTime)
		if err != nil {
			s.logger.Error("Failed to parse end time", "error", err)
			return nil, fmt.Errorf("failed to parse end time: %w", err)
		}

		// Convert to observability package params for backend
		backendParams := observability.ComponentApplicationLogsParams{
			ComponentID:   params.ComponentID,
			EnvironmentID: params.EnvironmentID,
			ProjectID:     params.ProjectID,
			Namespace:     params.Namespace,
			StartTime:     startTime,
			EndTime:       endTime,
			SearchPhrase:  params.SearchPhrase,
			LogLevels:     params.LogLevels,
			Versions:      params.Versions,
			VersionIDs:    params.VersionIDs,
			Limit:         params.Limit,
			SortOrder:     params.SortOrder,
		}

		return s.getComponentApplicationLogsFromBackend(ctx, backendParams)
	}

	// Fallback: Use OpenSearch in Observer
	return s.getComponentLogsFromOpenSearch(ctx, params)
}

// getComponentLogsFromBackend fetches logs from the configured logs backend (e.g., OpenObserve)
// Backend implements observability.LogsBackend interface and returns observability.ComponentLogsResult directly
func (s *LoggingService) getComponentApplicationLogsFromBackend(ctx context.Context, params observability.ComponentApplicationLogsParams) (*observability.ComponentApplicationLogsResult, error) {
	// Call the logs backend directly - it implements observability.LogsBackend interface
	result, err := s.logsBackend.GetComponentApplicationLogs(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get component logs from backend", "error", err)
		return nil, fmt.Errorf("Failed to get component logs from backend: %w", err)
	}

	s.logger.Debug("Component logs retrieved from backend",
		"count", len(result.Logs),
		"total", result.TotalCount)

	return result, nil
}

// getComponentLogsFromOpenSearch fetches logs from OpenSearch
func (s *LoggingService) getComponentLogsFromOpenSearch(ctx context.Context, params opensearch.ComponentQueryParams) (*observability.ComponentApplicationLogsResult, error) {
	s.logger.Info("Using OpenSearch for component logs (legacy)")

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
	logs := make([]observability.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		osEntry := opensearch.ParseLogEntry(hit)
		// Convert opensearch.LogEntry to observability.LogEntry
		logs = append(logs, observability.LogEntry{
			Timestamp:     osEntry.Timestamp,
			Log:           osEntry.Log,
			LogLevel:      osEntry.LogLevel,
			ComponentID:   osEntry.ComponentID,
			EnvironmentID: osEntry.EnvironmentID,
			ProjectID:     osEntry.ProjectID,
			Version:       osEntry.Version,
			VersionID:     osEntry.VersionID,
			Namespace:     osEntry.Namespace,
			PodID:         osEntry.PodID,
			ContainerName: osEntry.ContainerName,
			Labels:        osEntry.Labels,
		})
	}

	s.logger.Info("Component logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &observability.ComponentApplicationLogsResult{
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
		"namespace_name", params.NamespaceName,
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

// GetNamespaceLogs retrieves logs for a namespace with custom filters
func (s *LoggingService) GetNamespaceLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting namespace logs",
		"namespace_name", params.NamespaceName,
		"environment_id", params.EnvironmentID,
		"pod_labels", podLabels,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build namespace-specific query
	query := s.queryBuilder.BuildNamespaceLogsQuery(params, podLabels)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute namespace logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Namespace logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetComponentWorkflowRunLogs retrieves log entries for a component workflow run
func (s *LoggingService) GetComponentWorkflowRunLogs(ctx context.Context, runName, stepName string, limit int) ([]opensearch.ComponentWorkflowRunLogEntry, error) {
	logger := s.logger.With("run_name", runName, "step_name", stepName)
	logger.Debug("Getting component workflow run logs")

	// Generate indices (empty times means search all indices)
	indices, err := s.queryBuilder.GenerateIndices("", "")
	if err != nil {
		logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query using query builder
	query := s.queryBuilder.BuildComponentWorkflowRunLogsQuery(opensearch.ComponentWorkflowRunQueryParams{
		RunName:  runName,
		StepName: stepName,
		Limit:    limit,
	})

	// Print query for debugging
	queryJSON, err := json.Marshal(query)
	if err != nil {
		logger.Error("Failed to marshal query", "error", err)
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}
	logger.Debug("Component workflow run logs query", "query", string(queryJSON))

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		logger.Error("Failed to execute component workflow run logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Extract log entries with timestamp from hits
	logs := make([]opensearch.ComponentWorkflowRunLogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		log, _ := hit.Source["log"].(string)
		ts, _ := hit.Source["@timestamp"].(string)
		logs = append(logs, opensearch.ComponentWorkflowRunLogEntry{
			Log:       log,
			Timestamp: ts,
		})
	}

	logger.Info("Component workflow run logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return logs, nil
}

func (s *LoggingService) GetTraces(ctx context.Context, params opensearch.TracesRequestParams) (*opensearch.TraceResponse, error) {
	s.logger.Debug("Fetching traces from OpenSearch")

	// Build traces query
	query := s.queryBuilder.BuildTracesQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, []string{"otel-traces-*"}, query)
	if err != nil {
		s.logger.Error("Failed to execute traces search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse spans and group by traceId
	traceMap := make(map[string][]opensearch.Span)
	for _, hit := range response.Hits.Hits {
		span := opensearch.ParseSpanEntry(hit)
		traceID := opensearch.GetTraceID(hit)
		if traceID != "" {
			traceMap[traceID] = append(traceMap[traceID], span)
		}
	}

	// Convert map to traces array and sort by traceID for consistent ordering
	traces := make([]opensearch.Trace, 0, len(traceMap))
	traceIDs := make([]string, 0, len(traceMap))

	for traceID := range traceMap {
		traceIDs = append(traceIDs, traceID)
	}

	// Sort trace IDs to ensure deterministic ordering
	sort.Strings(traceIDs)

	for _, traceID := range traceIDs {
		traces = append(traces, opensearch.Trace{
			TraceID: traceID,
			Spans:   traceMap[traceID],
		})
	}

	s.logger.Debug("Traces retrieved",
		"traceCount", len(traces))

	return &opensearch.TraceResponse{
		Traces: traces,
		Took:   response.Took,
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

// UpsertAlertRule creates or updates an alert rule in the observability backend
func (s *LoggingService) UpsertAlertRule(ctx context.Context, sourceType string, rule observertypes.AlertingRuleRequest) (*observertypes.AlertingRuleSyncResponse, error) {
	// Decide the observability backend based on the type of rule
	switch sourceType {
	case "log":
		return s.UpsertOpenSearchAlertRule(ctx, rule)
	case "metric":
		return s.UpsertPrometheusAlertRule(ctx, rule)
	default:
		return nil, fmt.Errorf("invalid alert rule source type: %s", sourceType)
	}
}

// UpsertOpenSearchAlertRule creates or updates an alert rule in OpenSearch
func (s *LoggingService) UpsertOpenSearchAlertRule(ctx context.Context, rule observertypes.AlertingRuleRequest) (*observertypes.AlertingRuleSyncResponse, error) {
	// Build the alert rule body
	alertRuleBody, err := s.queryBuilder.BuildLogAlertingRuleMonitorBody(rule)
	if err != nil {
		return nil, fmt.Errorf("failed to build log alerting rule body: %w", err)
	}

	// Print the alert rule body in a pretty json format for debugging
	if s.config.LogLevel == logLevelDebug {
		alertRuleBodyJSON, err := json.Marshal(alertRuleBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal alert rule body: %w", err)
		}
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, alertRuleBodyJSON, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to indent alert rule body: %w", err)
		}
		fmt.Println("Alert rule body:")
		fmt.Println(prettyJSON.String())
		fmt.Println("--------------------------------")
	}

	// Check if the alert rule already exists
	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, rule.Metadata.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}

	action := alertRuleActionCreated
	backendID := monitorID
	lastUpdateTime := int64(0)

	if exists {
		s.logger.Debug("Alert rule already exists. Checking if update is needed.",
			"rule_name", rule.Metadata.Name,
			"monitor_id", backendID)

		// Get the existing monitor to compare
		existingMonitor, err := s.osClient.GetMonitorByID(ctx, backendID)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing alert rule: %w", err)
		}

		// Compare the existing monitor with the new alert rule body
		if s.monitorsAreEqual(existingMonitor, alertRuleBody) {
			s.logger.Debug("Alert rule unchanged, skipping update.",
				"rule_name", rule.Metadata.Name,
				"monitor_id", backendID)
			action = alertRuleActionUnchanged
			// Use current time since we're not updating
			lastUpdateTime = time.Now().UnixMilli()
		} else {
			s.logger.Debug("Alert rule changed, updating.",
				"rule_name", rule.Metadata.Name,
				"monitor_id", backendID)

			// Update the alert rule
			lastUpdateTime, err = s.osClient.UpdateMonitor(ctx, backendID, alertRuleBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update alert rule: %w", err)
			}
			action = alertRuleActionUpdated
		}
	} else {
		s.logger.Debug("Alert rule does not exist. Creating the alert rule.",
			"rule_name", rule.Metadata.Name)

		// Create the alert rule
		backendID, lastUpdateTime, err = s.osClient.CreateMonitor(ctx, alertRuleBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create alert rule: %w", err)
		}
	}

	// Return the alert rule ID
	return &observertypes.AlertingRuleSyncResponse{
		Status:     "synced",
		LogicalID:  rule.Metadata.Name,
		BackendID:  backendID,
		Action:     action,
		LastSynced: time.UnixMilli(lastUpdateTime).UTC().Format(time.RFC3339),
	}, nil
}

// monitorsAreEqual compares two monitor bodies to determine if they are equal
// It converts both monitors to MonitorBody struct to filter out metadata fields
func (s *LoggingService) monitorsAreEqual(existing, new map[string]interface{}) bool {
	// Convert existing monitor to MonitorBody struct (filters out metadata)
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		s.logger.Warn("Failed to marshal existing monitor for comparison", "error", err)
		return false
	}

	var existingMonitor opensearch.MonitorBody
	if err := json.Unmarshal(existingJSON, &existingMonitor); err != nil {
		s.logger.Warn("Failed to unmarshal existing monitor to MonitorBody", "error", err)
		return false
	}

	// Convert new monitor to MonitorBody struct (filters out metadata)
	newJSON, err := json.Marshal(new)
	if err != nil {
		s.logger.Warn("Failed to marshal new monitor for comparison", "error", err)
		return false
	}

	var newMonitor opensearch.MonitorBody
	if err := json.Unmarshal(newJSON, &newMonitor); err != nil {
		s.logger.Warn("Failed to unmarshal new monitor to MonitorBody", "error", err)
		return false
	}

	isMonitorObjectEqual := reflect.DeepEqual(existingMonitor, newMonitor)
	if isMonitorObjectEqual {
		return true
	}

	s.logger.Debug("Monitors are not equal")

	// Marshal monitor bodies to log for debugging
	existingBodyJSON, err := json.Marshal(existingMonitor)
	if err != nil {
		s.logger.Warn("Failed to marshal existing MonitorBody for comparison", "error", err)
		return false
	}
	newBodyJSON, err := json.Marshal(newMonitor)
	if err != nil {
		s.logger.Warn("Failed to marshal new MonitorBody for comparison", "error", err)
		return false
	}

	s.logger.Debug("Monitor comparison details",
		"existing", string(existingBodyJSON),
		"new", string(newBodyJSON))

	return false
}

// DeleteAlertRule deletes an alert rule from the observability backend
func (s *LoggingService) DeleteAlertRule(ctx context.Context, sourceType string, ruleName string) (*observertypes.AlertingRuleSyncResponse, error) {
	// Decide the observability backend based on the type of rule
	switch sourceType {
	case "log":
		return s.DeleteOpenSearchAlertRule(ctx, ruleName)
	case "metric":
		return s.DeleteMetricAlertRule(ctx, ruleName)
	default:
		return nil, fmt.Errorf("invalid alert rule source type: %s", sourceType)
	}
}

// DeleteOpenSearchAlertRule deletes an alert rule from OpenSearch
func (s *LoggingService) DeleteOpenSearchAlertRule(ctx context.Context, ruleName string) (*observertypes.AlertingRuleSyncResponse, error) {
	// Search for the monitor by name to get its ID
	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}

	if !exists {
		// Rule doesn't exist - return a response indicating it wasn't found
		now := time.Now().UTC().Format(time.RFC3339)
		return &observertypes.AlertingRuleSyncResponse{
			Status:     "not_found",
			LogicalID:  ruleName,
			BackendID:  "",
			Action:     "not_found",
			LastSynced: now,
		}, nil
	}

	// Delete the monitor
	err = s.osClient.DeleteMonitor(ctx, monitorID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete alert rule: %w", err)
	}

	s.logger.Debug("Alert rule deleted successfully", "rule_name", ruleName, "monitor_id", monitorID)

	// Return the deletion response
	now := time.Now().UTC().Format(time.RFC3339)
	return &observertypes.AlertingRuleSyncResponse{
		Status:     "deleted",
		LogicalID:  ruleName,
		BackendID:  monitorID,
		Action:     "deleted",
		LastSynced: now,
	}, nil
}

// UpsertPrometheusAlertRule creates or updates a metric-based alert rule as a PrometheusRule CR
func (s *LoggingService) UpsertPrometheusAlertRule(ctx context.Context, rule observertypes.AlertingRuleRequest) (*observertypes.AlertingRuleSyncResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	// Create the alert rule builder
	alertRuleBuilder := prometheus.NewAlertRuleBuilder(s.config.Alerting.ObservabilityNamespace)

	// Build the PrometheusRule CR
	prometheusRule, err := alertRuleBuilder.BuildPrometheusRule(rule)
	if err != nil {
		return nil, fmt.Errorf("failed to build PrometheusRule: %w", err)
	}

	// Log the generated rule for debugging
	if len(prometheusRule.Spec.Groups) > 0 && len(prometheusRule.Spec.Groups[0].Rules) > 0 {
		group := prometheusRule.Spec.Groups[0]
		rule := group.Rules[0]
		s.logger.Debug("Generated PrometheusRule",
			"name", prometheusRule.Name,
			"namespace", prometheusRule.Namespace,
			"groupName", group.Name,
			"interval", group.Interval,
			"alertName", rule.Alert,
			"expression", rule.Expr.String(),
			"for", rule.For,
			"labels", rule.Labels)
	}

	// Check if the PrometheusRule already exists
	existingRule := &monitoringv1.PrometheusRule{}
	err = s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      rule.Metadata.Name,
	}, existingRule)

	action := alertRuleActionCreated
	now := time.Now()

	if err == nil {
		// Rule exists, check if update is needed
		if s.prometheusRulesAreEqual(existingRule, prometheusRule) {
			s.logger.Debug("PrometheusRule unchanged, skipping update",
				"rule_name", rule.Metadata.Name,
				"namespace", s.config.Alerting.ObservabilityNamespace)
			return &observertypes.AlertingRuleSyncResponse{
				Status:     "synced",
				LogicalID:  rule.Metadata.Name,
				BackendID:  string(existingRule.UID),
				Action:     alertRuleActionUnchanged,
				LastSynced: now.UTC().Format(time.RFC3339),
			}, nil
		}

		// Update the existing rule
		existingRule.Spec = prometheusRule.Spec
		existingRule.Labels = prometheusRule.Labels
		if err := s.k8sClient.Update(ctx, existingRule); err != nil {
			return nil, fmt.Errorf("failed to update PrometheusRule: %w", err)
		}
		action = alertRuleActionUpdated
		s.logger.Debug("PrometheusRule updated",
			"rule_name", rule.Metadata.Name,
			"namespace", s.config.Alerting.ObservabilityNamespace)
	} else if apierrors.IsNotFound(err) {
		// Create new rule
		if err := s.k8sClient.Create(ctx, prometheusRule); err != nil {
			return nil, fmt.Errorf("failed to create PrometheusRule: %w", err)
		}
		s.logger.Debug("PrometheusRule created",
			"rule_name", rule.Metadata.Name,
			"namespace", s.config.Alerting.ObservabilityNamespace)
	} else {
		return nil, fmt.Errorf("failed to get existing PrometheusRule: %w", err)
	}

	// Get the UID of the created/updated rule
	if action == alertRuleActionCreated {
		// Re-fetch to get the UID
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Namespace: s.config.Alerting.ObservabilityNamespace,
			Name:      rule.Metadata.Name,
		}, prometheusRule); err != nil {
			s.logger.Warn("Failed to re-fetch PrometheusRule for UID", "error", err)
		}
	}

	backendID := string(prometheusRule.UID)
	if action == alertRuleActionUpdated {
		backendID = string(existingRule.UID)
	}

	return &observertypes.AlertingRuleSyncResponse{
		Status:     "synced",
		LogicalID:  rule.Metadata.Name,
		BackendID:  backendID,
		Action:     action,
		LastSynced: now.UTC().Format(time.RFC3339),
	}, nil
}

// prometheusRulesAreEqual compares two PrometheusRule specs to determine if they are equal
func (s *LoggingService) prometheusRulesAreEqual(existing, new *monitoringv1.PrometheusRule) bool {
	// Compare specs using JSON marshaling for deep comparison
	existingJSON, err := json.Marshal(existing.Spec)
	if err != nil {
		s.logger.Warn("Failed to marshal existing PrometheusRule spec", "error", err)
		return false
	}

	newJSON, err := json.Marshal(new.Spec)
	if err != nil {
		s.logger.Warn("Failed to marshal new PrometheusRule spec", "error", err)
		return false
	}

	return string(existingJSON) == string(newJSON)
}

// DeleteMetricAlertRule deletes a metric-based alert rule (PrometheusRule CR)
func (s *LoggingService) DeleteMetricAlertRule(ctx context.Context, ruleName string) (*observertypes.AlertingRuleSyncResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	// Try to get the existing PrometheusRule
	existingRule := &monitoringv1.PrometheusRule{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, existingRule)

	now := time.Now().UTC().Format(time.RFC3339)

	if apierrors.IsNotFound(err) {
		// Rule doesn't exist
		return &observertypes.AlertingRuleSyncResponse{
			Status:     "not_found",
			LogicalID:  ruleName,
			BackendID:  "",
			Action:     "not_found",
			LastSynced: now,
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get PrometheusRule: %w", err)
	}

	// Delete the PrometheusRule
	backendID := string(existingRule.UID)
	if err := s.k8sClient.Delete(ctx, existingRule); err != nil {
		return nil, fmt.Errorf("failed to delete PrometheusRule: %w", err)
	}

	s.logger.Debug("PrometheusRule deleted successfully",
		"rule_name", ruleName,
		"namespace", s.config.Alerting.ObservabilityNamespace)

	return &observertypes.AlertingRuleSyncResponse{
		Status:     "deleted",
		LogicalID:  ruleName,
		BackendID:  backendID,
		Action:     "deleted",
		LastSynced: now,
	}, nil
}

// GetComponentHTTPMetrics retrieves HTTP metrics for a component
func (s *LoggingService) GetComponentHTTPMetrics(ctx context.Context, componentID, environmentID, projectID string, startTime, endTime time.Time) (*HTTPMetricsTimeSeries, error) {
	s.logger.Debug("Getting resource metrics",
		"project", projectID,
		"component", componentID,
		"environment", environmentID,
		"start", startTime,
		"end", endTime)

	step := 5 * time.Minute
	metrics := &HTTPMetricsTimeSeries{}

	// Build component label filter using query builder
	labelFilter := prometheus.BuildLabelFilter(componentID, projectID, environmentID)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var queryErrors []error

	// Request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.RequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Successful request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildSuccessfulHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Successful request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query successful request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("successful request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.SuccessfulRequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Unsuccessful request count
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildUnsuccessfulHTTPRequestCountQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Unsuccessful request count query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query unsuccessful request count", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("unsuccessful request count: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.UnsuccessfulRequestCount = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Mean latency
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.BuildMeanHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Mean latency query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query mean latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("mean latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.MeanLatency = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 50th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build50thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 50th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 50th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("50th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile50th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 90th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build90thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 90th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 90th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("90th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile90th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	// Latency Percentile - 99th
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := prometheus.Build99thPercentileHTTPRequestLatencyQuery(labelFilter)
		if s.config.LogLevel == logLevelDebug {
			fmt.Println("Latency 99th percentile query:", query)
		}
		response, err := s.metricsService.QueryRangeTimeSeries(ctx, query, startTime, endTime, step)
		if err != nil {
			s.logger.Warn("Failed to query 99th percentile latency", "error", err)
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("99th percentile latency: %w", err))
			mu.Unlock()
			return
		}
		if len(response.Data.Result) > 0 {
			mu.Lock()
			metrics.LatencyPercentile99th = prometheus.ConvertTimeSeriesToTimeValuePoints(response.Data.Result[0])
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Check if any errors occurred during metric queries
	if len(queryErrors) > 0 {
		s.logger.Error("Failed to fetch one or more HTTP metrics", "errors", queryErrors)
		return nil, fmt.Errorf("internal error occurred when fetching one or more HTTP metrics")
	}

	s.logger.Debug("HTTP metrics time series retrieved",
		"request_count", len(metrics.RequestCount),
		"successful_request_count", len(metrics.SuccessfulRequestCount),
		"unsuccessful_request_count", len(metrics.UnsuccessfulRequestCount),
		"mean_latency", len(metrics.MeanLatency),
		"latency_50th_points", len(metrics.LatencyPercentile50th),
		"latency_90th_points", len(metrics.LatencyPercentile90th),
		"latency_99th_points", len(metrics.LatencyPercentile99th),
	)

	return metrics, nil
}

// GetComponentResourceMetrics retrieves resource usage metrics for a component as time series
func (s *LoggingService) GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID string, startTime, endTime time.Time) (*prometheus.ResourceMetricsTimeSeries, error) {
	s.logger.Debug("Getting resource metrics",
		"project", projectID,
		"component", componentID,
		"environment", environmentID,
		"start", startTime,
		"end", endTime)

	// Define step interval for time series queries (5 minute intervals)
	step := 5 * time.Minute

	metrics := &prometheus.ResourceMetricsTimeSeries{}

	// Build component label filter using query builder
	labelFilter := prometheus.BuildLabelFilter(componentID, projectID, environmentID)

	// CPU usage
	cpuUsageQuery := prometheus.BuildCPUUsageQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU usage query:", cpuUsageQuery)
	}
	cpuResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuUsageQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU usage", "error", err)
	} else if len(cpuResp.Data.Result) > 0 {
		metrics.CPUUsage = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuResp.Data.Result[0])
	}

	// Memory usage
	memUsageQuery := prometheus.BuildMemoryUsageQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory usage query:", memUsageQuery)
	}
	memResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memUsageQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory usage", "error", err)
	} else if len(memResp.Data.Result) > 0 {
		metrics.Memory = prometheus.ConvertTimeSeriesToTimeValuePoints(memResp.Data.Result[0])
	}

	// CPU requests
	cpuRequestQuery := prometheus.BuildCPURequestsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU requests query:", cpuRequestQuery)
	}
	cpuRequestResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuRequestQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU requests", "error", err)
	} else if len(cpuRequestResp.Data.Result) > 0 {
		metrics.CPURequests = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuRequestResp.Data.Result[0])
	}

	// CPU limits
	cpuLimitQuery := prometheus.BuildCPULimitsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("CPU limit query:", cpuLimitQuery)
	}
	cpuLimitResp, err := s.metricsService.QueryRangeTimeSeries(ctx, cpuLimitQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query CPU limits", "error", err)
	} else if len(cpuLimitResp.Data.Result) > 0 {
		metrics.CPULimits = prometheus.ConvertTimeSeriesToTimeValuePoints(cpuLimitResp.Data.Result[0])
	}

	// Memory requests
	memRequestQuery := prometheus.BuildMemoryRequestsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory requests query:", memRequestQuery)
	}
	memRequestResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memRequestQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory requests", "error", err)
	} else if len(memRequestResp.Data.Result) > 0 {
		metrics.MemoryRequests = prometheus.ConvertTimeSeriesToTimeValuePoints(memRequestResp.Data.Result[0])
	}

	// Memory limits
	memLimitQuery := prometheus.BuildMemoryLimitsQuery(labelFilter)
	if s.config.LogLevel == logLevelDebug {
		fmt.Println("Memory limit query:", memLimitQuery)
	}
	memLimitResp, err := s.metricsService.QueryRangeTimeSeries(ctx, memLimitQuery, startTime, endTime, step)
	if err != nil {
		s.logger.Warn("Failed to query memory limits", "error", err)
	} else if len(memLimitResp.Data.Result) > 0 {
		metrics.MemoryLimits = prometheus.ConvertTimeSeriesToTimeValuePoints(memLimitResp.Data.Result[0])
	}

	s.logger.Debug("Resource metrics time series retrieved",
		"cpu_usage_points", len(metrics.CPUUsage),
		"cpu_requests_points", len(metrics.CPURequests),
		"cpu_limits_points", len(metrics.CPULimits),
		"memory_points", len(metrics.Memory),
		"memory_requests_points", len(metrics.MemoryRequests),
		"memory_limits_points", len(metrics.MemoryLimits))

	return metrics, nil
}

// SendAlertNotification sends an alert notification via the configured notification channel
func (s *LoggingService) SendAlertNotification(ctx context.Context, alertDetails *observertypes.AlertDetails) error {
	// If no notification channel is specified, log and skip
	if alertDetails.NotificationChannel == "" {
		s.logger.Warn("Missing notification channel in alert details, skipping notification",
			"ruleName", alertDetails.AlertName,
			"notificationChannel", alertDetails.NotificationChannel)
		return nil
	}

	// Fetch the notification channel configuration from Kubernetes
	channelConfig, err := s.getNotificationChannelConfig(ctx, alertDetails.NotificationChannel)
	if err != nil {
		s.logger.Error("Failed to get notification channel config",
			"error", err,
			"channelName", alertDetails.NotificationChannel)
		return fmt.Errorf("failed to get notification channel config: %w", err)
	}

	// Send notification using the notifications package
	return notifications.SendAlertNotification(ctx, channelConfig, alertDetails, s.logger)
}

// getNotificationChannelConfig fetches the notification channel configuration from Kubernetes
// It reads the ConfigMap and Secret for the notification channel and resolves to NotificationChannelConfig
func (s *LoggingService) getNotificationChannelConfig(ctx context.Context, channelName string) (*notifications.NotificationChannelConfig, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	// Use label selector to find the ConfigMap for the notification channel
	// The controller adds the openchoreo.dev/notification-channel-name label when creating resources
	labelSelector := client.MatchingLabels{
		labels.LabelKeyNotificationChannelName: channelName,
	}

	var configMap *corev1.ConfigMap
	configMapList := &corev1.ConfigMapList{}
	if err := s.k8sClient.List(ctx, configMapList, labelSelector); err != nil {
		return nil, fmt.Errorf("failed to list ConfigMaps with label selector: %w", err)
	}

	if len(configMapList.Items) == 0 {
		return nil, fmt.Errorf("failed to find notification channel ConfigMap with label %s=%s", labels.LabelKeyNotificationChannelName, channelName)
	}
	configMap = configMapList.Items[0].DeepCopy()

	// Use label selector to find the Secret for the notification channel
	var secret *corev1.Secret
	secretList := &corev1.SecretList{}
	if err := s.k8sClient.List(ctx, secretList, labelSelector); err != nil {
		return nil, fmt.Errorf("failed to list Secrets with label selector: %w", err)
	}

	if len(secretList.Items) == 0 {
		return nil, fmt.Errorf("failed to find notification channel Secret with label %s=%s", labels.LabelKeyNotificationChannelName, channelName)
	}
	secret = secretList.Items[0].DeepCopy()

	// Get channel type from ConfigMap
	channelType := configMap.Data["type"]
	if channelType == "" {
		return nil, fmt.Errorf("notification channel type not found in ConfigMap")
	}

	config := &notifications.NotificationChannelConfig{
		Type: channelType,
	}

	// Parse configuration based on channel type
	switch channelType {
	case "email":
		emailConfig, err := notifications.PrepareEmailNotificationConfig(configMap, secret, s.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare email notification config: %w", err)
		}
		config.Email = emailConfig

	case "webhook":
		webhookConfig, err := notifications.PrepareWebhookNotificationConfig(configMap, secret, s.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare webhook notification config: %w", err)
		}
		config.Webhook = webhookConfig

	default:
		return nil, fmt.Errorf("unsupported notification channel type: %s", channelType)
	}

	return config, nil
}

// StoreAlertEntry stores an alert entry in the logs backend and returns the alert ID
func (s *LoggingService) StoreAlertEntry(ctx context.Context, alertDetails *observertypes.AlertDetails) (string, error) {
	alertEntry := map[string]interface{}{
		"@timestamp":      alertDetails.AlertTimestamp,
		"alert_rule_name": alertDetails.AlertName,
		"alert_value":     alertDetails.AlertValue,
		"labels": map[string]interface{}{
			observerlabels.ComponentID:   alertDetails.ComponentID,
			observerlabels.EnvironmentID: alertDetails.EnvironmentID,
			observerlabels.ProjectID:     alertDetails.ProjectID,
		},
		"enable_ai_rca": alertDetails.AlertAIRootCauseAnalysisEnabled,
	}

	alertID, err := s.osClient.WriteAlertEntry(ctx, alertEntry)
	if err != nil {
		s.logger.Error("Failed to write alert entry to OpenSearch", "error", err)
		return "", fmt.Errorf("failed to write alert entry: %w", err)
	}

	return alertID, nil
}

// GetObservabilityAlertRuleByName retrieves an ObservabilityAlertRule by name and namespace
func (s *LoggingService) GetObservabilityAlertRuleByName(ctx context.Context, ruleName, namespace string) (*choreoapis.ObservabilityAlertRule, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	alertRule := &choreoapis.ObservabilityAlertRule{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      ruleName,
		Namespace: namespace,
	}, alertRule); err != nil {
		return nil, fmt.Errorf("failed to get ObservabilityAlertRule %s/%s: %w", namespace, ruleName, err)
	}

	return alertRule, nil
}

// TriggerRCAAnalysis triggers an AI RCA analysis for the given alert.
// It enriches the payload with CRD data and sends a request to the RCA service.
func (s *LoggingService) TriggerRCAAnalysis(rcaServiceURL string, alertID string, alertDetails *observertypes.AlertDetails, alertRule *choreoapis.ObservabilityAlertRule) {
	// Build the rule info with basic name
	ruleInfo := map[string]interface{}{
		"name": alertDetails.AlertName,
	}

	// Enrich with CRD data if available
	if alertRule != nil {
		if alertRule.Spec.Description != "" {
			ruleInfo["description"] = alertRule.Spec.Description
		}
		if alertRule.Spec.Severity != "" {
			ruleInfo["severity"] = string(alertRule.Spec.Severity)
		}

		ruleInfo["source"] = map[string]interface{}{
			"type":   string(alertRule.Spec.Source.Type),
			"query":  alertRule.Spec.Source.Query,
			"metric": alertRule.Spec.Source.Metric,
		}

		ruleInfo["condition"] = map[string]interface{}{
			"window":    alertRule.Spec.Condition.Window.Duration.String(),
			"interval":  alertRule.Spec.Condition.Interval.Duration.String(),
			"operator":  alertRule.Spec.Condition.Operator,
			"threshold": alertRule.Spec.Condition.Threshold,
		}

		s.logger.Debug("Enriched RCA payload with ObservabilityAlertRule data", "ruleName", alertDetails.AlertName)
	}

	// Build the RCA service request payload
	rcaPayload := map[string]interface{}{
		"componentUid":   alertDetails.ComponentID,
		"projectUid":     alertDetails.ProjectID,
		"environmentUid": alertDetails.EnvironmentID,
		"alert": map[string]interface{}{
			"id":        alertID,
			"value":     alertDetails.AlertValue,
			"timestamp": alertDetails.AlertTimestamp,
			"rule":      ruleInfo,
		},
	}

	// Send request to AI RCA service
	payloadBytes, err := json.Marshal(rcaPayload)
	if err != nil {
		s.logger.Error("Failed to marshal RCA request payload", "error", err)
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(rcaServiceURL+"/api/v1/agent/rca", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		s.logger.Error("Failed to send RCA analysis request", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Error("RCA analysis request returned non-success status", "statusCode", resp.StatusCode, "alertID", alertID)
	} else {
		s.logger.Debug("AI RCA analysis triggered", "alertID", alertID)
	}
}

// ParseOpenSearchAlertPayload parses the OpenSearch alert payload
// Returns: ruleName, ruleNamespace, alertValue, timestamp, error
func (s *LoggingService) ParseOpenSearchAlertPayload(requestBody map[string]interface{}) (string, string, string, string, error) {
	ruleName, _ := requestBody["ruleName"].(string)
	ruleNamespace, _ := requestBody["ruleNamespace"].(string)

	// alertValue comes from {{ctx.results.0.hits.total.value}} which is a number
	var alertValue string
	if v, ok := requestBody["alertValue"].(float64); ok {
		alertValue = strconv.FormatFloat(v, 'f', -1, 64)
	} else if v, ok := requestBody["alertValue"].(string); ok {
		alertValue = v
	}

	timestamp, _ := requestBody["timestamp"].(string)

	if ruleName == "" {
		return "", "", "", "", fmt.Errorf("ruleName is required in OpenSearch alert payload")
	}
	if ruleNamespace == "" {
		return "", "", "", "", fmt.Errorf("ruleNamespace is required in OpenSearch alert payload")
	}

	return ruleName, ruleNamespace, alertValue, timestamp, nil
}

// ParsePrometheusAlertPayload parses the Prometheus Alertmanager webhook payload
// Returns: ruleName, ruleNamespace, alertValue, timestamp, error
func (s *LoggingService) ParsePrometheusAlertPayload(requestBody map[string]interface{}) (string, string, string, string, error) {
	// Alertmanager sends alerts in an array
	alerts, ok := requestBody["alerts"].([]interface{})
	if !ok || len(alerts) == 0 {
		return "", "", "", "", fmt.Errorf("no alerts found in Prometheus payload")
	}

	// Get the first alert
	alert, ok := alerts[0].(map[string]interface{})
	if !ok {
		return "", "", "", "", fmt.Errorf("invalid alert format in Prometheus payload")
	}
	// Ignore alert if not in "firing" state
	status, _ := alert["status"].(string)
	if status != "firing" {
		return "", "", "", "", fmt.Errorf("alert is not in firing state")
	}

	// Extract from annotations (where we put rule_name, rule_namespace, alert_value)
	annotations, _ := alert["annotations"].(map[string]interface{})
	ruleName, _ := annotations["rule_name"].(string)
	ruleNamespace, _ := annotations["rule_namespace"].(string)
	alertValue, _ := annotations["alert_value"].(string)

	// Extract timestamp from startsAt
	timestamp, _ := alert["startsAt"].(string)

	if ruleName == "" {
		return "", "", "", "", fmt.Errorf("rule_name is required in Prometheus alert annotations")
	}
	if ruleNamespace == "" {
		return "", "", "", "", fmt.Errorf("rule_namespace is required in Prometheus alert annotations")
	}

	return ruleName, ruleNamespace, alertValue, timestamp, nil
}

// EnrichAlertDetails enriches the alert details with the ObservabilityAlertRule CR details
func (s *LoggingService) EnrichAlertDetails(alertRule *choreoapis.ObservabilityAlertRule, alertValue string, timestamp string) (*observertypes.AlertDetails, error) {
	return &observertypes.AlertDetails{
		AlertName:                       alertRule.Spec.Name,
		AlertTimestamp:                  timestamp,
		AlertSeverity:                   string(alertRule.Spec.Severity),
		AlertDescription:                alertRule.Spec.Description,
		AlertThreshold:                  strconv.FormatInt(alertRule.Spec.Condition.Threshold, 10),
		AlertValue:                      alertValue,
		AlertType:                       string(alertRule.Spec.Source.Type),
		ComponentID:                     alertRule.Labels["openchoreo.dev/component-uid"],
		EnvironmentID:                   alertRule.Labels["openchoreo.dev/environment-uid"],
		ProjectID:                       alertRule.Labels["openchoreo.dev/project-uid"],
		Component:                       alertRule.Labels["openchoreo.dev/component"],
		Project:                         alertRule.Labels["openchoreo.dev/project"],
		Environment:                     alertRule.Labels["openchoreo.dev/environment"],
		NotificationChannel:             alertRule.Spec.NotificationChannel,
		AlertAIRootCauseAnalysisEnabled: alertRule.Spec.EnableAiRootCauseAnalysis,
	}, nil
}
