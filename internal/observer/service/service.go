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
	"sort"
	"strconv"
	"strings"
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
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/template"
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
func NewLoggingService(osClient OpenSearchClient, metricsService *prometheus.MetricsService, k8sClient client.Client, cfg *config.Config, logger *slog.Logger) *LoggingService {
	return &LoggingService{
		osClient:       osClient,
		queryBuilder:   opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		metricsService: metricsService,
		k8sClient:      k8sClient,
		config:         cfg,
		logger:         logger,
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
func (s *LoggingService) UpsertAlertRule(ctx context.Context, sourceType string, rule types.AlertingRuleRequest) (*types.AlertingRuleSyncResponse, error) {
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
func (s *LoggingService) UpsertOpenSearchAlertRule(ctx context.Context, rule types.AlertingRuleRequest) (*types.AlertingRuleSyncResponse, error) {
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
	return &types.AlertingRuleSyncResponse{
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
func (s *LoggingService) DeleteAlertRule(ctx context.Context, sourceType string, ruleName string) (*types.AlertingRuleSyncResponse, error) {
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
func (s *LoggingService) DeleteOpenSearchAlertRule(ctx context.Context, ruleName string) (*types.AlertingRuleSyncResponse, error) {
	// Search for the monitor by name to get its ID
	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}

	if !exists {
		// Rule doesn't exist - return a response indicating it wasn't found
		now := time.Now().UTC().Format(time.RFC3339)
		return &types.AlertingRuleSyncResponse{
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
	return &types.AlertingRuleSyncResponse{
		Status:     "deleted",
		LogicalID:  ruleName,
		BackendID:  monitorID,
		Action:     "deleted",
		LastSynced: now,
	}, nil
}

// UpsertPrometheusAlertRule creates or updates a metric-based alert rule as a PrometheusRule CR
func (s *LoggingService) UpsertPrometheusAlertRule(ctx context.Context, rule types.AlertingRuleRequest) (*types.AlertingRuleSyncResponse, error) {
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
			return &types.AlertingRuleSyncResponse{
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

	return &types.AlertingRuleSyncResponse{
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
func (s *LoggingService) DeleteMetricAlertRule(ctx context.Context, ruleName string) (*types.AlertingRuleSyncResponse, error) {
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
		return &types.AlertingRuleSyncResponse{
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

	return &types.AlertingRuleSyncResponse{
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

// GetRCAReportsByProject retrieves RCA reports for a specific project with optional filtering
func (s *LoggingService) GetRCAReportsByProject(ctx context.Context, params opensearch.RCAReportQueryParams) (map[string]interface{}, error) {
	s.logger.Info("Getting RCA reports for project",
		"project_uid", params.ProjectUID,
		"environment_uid", params.EnvironmentUID,
		"component_count", len(params.ComponentUIDs),
		"status", params.Status)

	// Use wildcard index pattern for RCA reports
	indices := []string{"rca-reports-*"}

	// Build query
	query := s.queryBuilder.BuildRCAReportsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute RCA reports search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse reports using opensearch package helper
	reports := make([]map[string]interface{}, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		report := opensearch.ParseRCAReportSummary(hit)
		reports = append(reports, report)
	}

	s.logger.Debug("RCA reports retrieved",
		"count", len(reports),
		"total", response.Hits.Total.Value)

	return map[string]interface{}{
		"reports":    reports,
		"totalCount": response.Hits.Total.Value,
		"tookMs":     response.Took,
	}, nil
}

// GetRCAReportByAlert retrieves a single RCA report by alert with optional version
func (s *LoggingService) GetRCAReportByAlert(ctx context.Context, params opensearch.RCAReportByAlertQueryParams) (map[string]interface{}, error) {
	s.logger.Debug("Getting RCA report by alert",
		"alert_id", params.AlertID,
		"version", params.Version)

	// Use wildcard index pattern for RCA reports
	indices := []string{"rca-reports-*"}

	// First, get all available versions for this alert ID
	versionsQuery := s.queryBuilder.BuildRCAReportVersionsQuery(params.AlertID)
	versionsResponse, err := s.osClient.Search(ctx, indices, versionsQuery)
	if err != nil {
		s.logger.Error("Failed to query available versions", "error", err)
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}

	if len(versionsResponse.Hits.Hits) == 0 {
		s.logger.Info("No RCA report found for alert ID", "alert_id", params.AlertID)
		return nil, fmt.Errorf("report not found")
	}

	// Extract available versions using opensearch helper
	availableVersions := make([]int, 0, len(versionsResponse.Hits.Hits))
	for _, hit := range versionsResponse.Hits.Hits {
		version := opensearch.ExtractRCAReportVersion(hit)
		if version > 0 {
			availableVersions = append(availableVersions, version)
		}
	}

	// Build query for the specific report
	query := s.queryBuilder.BuildRCAReportByAlertQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute RCA report search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	if len(response.Hits.Hits) == 0 {
		s.logger.Info("RCA report not found", "alert_id", params.AlertID, "version", params.Version)
		return nil, fmt.Errorf("report not found")
	}

	// Parse the full report using opensearch package helper
	result := opensearch.ParseRCAReportDetailed(response.Hits.Hits[0], availableVersions)

	s.logger.Debug("RCA report retrieved",
		"alert_id", params.AlertID,
		"version", result["reportVersion"],
		"available_versions_count", len(availableVersions))

	return result, nil
}

// SendAlertNotification sends an alert notification via the configured notification channel
func (s *LoggingService) SendAlertNotification(ctx context.Context, requestBody map[string]interface{}, ruleName string) error {
	notificationChannelName := ""
	if channel, ok := requestBody["notificationChannel"].(string); ok && channel != "" {
		notificationChannelName = channel
	}

	// If no notification channel is specified, log and skip
	if notificationChannelName == "" {
		s.logger.Warn("Missing notification channel in webhook payload, skipping notification",
			"ruleName", ruleName,
			"notificationChannel", notificationChannelName)
		return nil
	}

	// Fetch the notification channel configuration from Kubernetes
	channelConfig, err := s.getNotificationChannelConfig(ctx, notificationChannelName)
	if err != nil {
		s.logger.Error("Failed to get notification channel config",
			"error", err,
			"channelName", notificationChannelName)
		return fmt.Errorf("failed to get notification channel config: %w", err)
	}

	// Send notification based on channel type
	switch channelConfig.Type {
	case "webhook":
		// Transform payload if template is provided, otherwise use raw alertDetails
		var payload map[string]interface{}
		if channelConfig.Webhook.PayloadTemplate != "" {
			// Render the template using CEL expressions
			renderedTemplate := s.renderTemplate(channelConfig.Webhook.PayloadTemplate, requestBody)

			// Parse the rendered template as JSON
			if err := json.Unmarshal([]byte(renderedTemplate), &payload); err != nil {
				s.logger.Error("Failed to parse rendered webhook payload template as JSON",
					"error", err,
					"renderedTemplate", renderedTemplate,
					"channelName", notificationChannelName)
				return fmt.Errorf("failed to parse webhook payload template as JSON: %w", err)
			}
			s.logger.Debug("Webhook payload template rendered and parsed",
				"channelName", notificationChannelName,
				"payload", payload)
		} else {
			// No template provided, use raw alertDetails
			payload = requestBody
		}

		// Send the webhook with the transformed payload
		if err := notifications.SendWebhookWithConfig(ctx, &channelConfig.Webhook, payload); err != nil {
			s.logger.Error("Failed to send alert notification webhook",
				"error", err,
				"channelName", notificationChannelName,
				"webhookURL", channelConfig.Webhook.URL,
				"payload", payload)
			return fmt.Errorf("failed to send alert notification webhook: %w", err)
		}
		s.logger.Debug("Alert notification sent successfully via webhook",
			"ruleName", ruleName,
			"channelName", notificationChannelName,
			"webhookURL", channelConfig.Webhook.URL,
			"usedTemplate", channelConfig.Webhook.PayloadTemplate != "")
		return nil

	case "email":
		// Render the incoming alert payload for human-friendly notifications
		payload, err := json.MarshalIndent(requestBody, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal alert payload: %w", err)
		}

		// Build subject and body using templates if available, otherwise use defaults
		subject := fmt.Sprintf("OpenChoreo alert triggered: %s", ruleName)
		if channelConfig.Email.SubjectTemplate != "" {
			subject = s.renderTemplate(channelConfig.Email.SubjectTemplate, requestBody)
		}

		emailBody := fmt.Sprintf("An alert was triggered at %s UTC.\n\nPayload:\n%s\n", time.Now().UTC().Format(time.RFC3339), string(payload))
		if channelConfig.Email.BodyTemplate != "" {
			emailBody = s.renderTemplate(channelConfig.Email.BodyTemplate, requestBody)
		}

		// Send the notification using the fetched config
		if err := notifications.SendEmailWithConfig(ctx, channelConfig, subject, emailBody); err != nil {
			s.logger.Error("Failed to send alert notification email",
				"error", err,
				"channelName", notificationChannelName,
				"recipients", channelConfig.Email.To)
			return fmt.Errorf("failed to send alert notification email: %w", err)
		}

		s.logger.Debug("Alert notification sent successfully",
			"ruleName", ruleName,
			"channelName", notificationChannelName,
			"recipients count", len(channelConfig.Email.To))
		return nil

	default:
		return fmt.Errorf("unsupported notification channel type: %s", channelConfig.Type)
	}
}

// getNotificationChannelConfig fetches the notification channel configuration from Kubernetes
// It reads the ConfigMap and Secret for the notification channel and resolves SMTP credentials
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

	// Parse SMTP port from ConfigMap
	smtpPort := 587 // default SMTP port
	if portStr, ok := configMap.Data["smtp.port"]; ok {
		if port, err := strconv.Atoi(portStr); err == nil {
			smtpPort = port
		}
	}

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
		// Parse recipients from ConfigMap (stored as string representation of array)
		var recipients []string
		if toStr, ok := configMap.Data["to"]; ok {
			// The 'to' field is stored as a string like "[email1@example.com email2@example.com]"
			// Parse it back to a slice
			recipients = parseRecipientsList(toStr)
		}

		config.Email = notifications.EmailConfig{
			SMTP: notifications.SMTPConfig{
				Host: configMap.Data["smtp.host"],
				Port: smtpPort,
				From: configMap.Data["from"],
			},
			To:              recipients,
			SubjectTemplate: configMap.Data["template.subject"],
			BodyTemplate:    configMap.Data["template.body"],
		}

		// Read SMTP credentials directly from the secret
		if secret != nil && secret.Data != nil {
			s.logger.Debug("Reading SMTP credentials from secret",
				"secretName", secret.Name,
				"secretNamespace", secret.Namespace)

			if username, ok := secret.Data["smtp.auth.username"]; ok {
				config.Email.SMTP.Username = string(username)
				s.logger.Debug("SMTP username loaded")
			} else {
				s.logger.Warn("SMTP username key not found in secret")
			}
			if password, ok := secret.Data["smtp.auth.password"]; ok {
				config.Email.SMTP.Password = string(password)
				s.logger.Debug("SMTP password loaded")
			} else {
				s.logger.Warn("SMTP password key not found in secret")
			}
		} else {
			s.logger.Warn("Secret is nil or has no data",
				"secretNil", secret == nil)
		}

		s.logger.Debug("Final SMTP config",
			"host", config.Email.SMTP.Host,
			"port", config.Email.SMTP.Port,
			"from", config.Email.SMTP.From,
			"hasUsername", config.Email.SMTP.Username != "",
			"hasPassword", config.Email.SMTP.Password != "")

	case "webhook":
		// Parse webhook URL
		webhookURL := configMap.Data["webhook.url"]
		if webhookURL == "" {
			return nil, fmt.Errorf("Alert webhook URL not found in ConfigMap")
		}

		// Parse headers from ConfigMap and Secret
		headers := make(map[string]string)
		if headerKeysStr, ok := configMap.Data["webhook.headers"]; ok && headerKeysStr != "" {
			headerKeys := strings.Split(headerKeysStr, ",")
			for _, key := range headerKeys {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				// Try to get inline value from ConfigMap first
				if inlineValue, ok := configMap.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
					headers[key] = inlineValue
				} else if secret != nil && secret.Data != nil {
					// Try to get value from Secret (for secret-referenced headers)
					if secretValue, ok := secret.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
						headers[key] = string(secretValue)
					}
				}
			}
		}

		// Parse payload template if provided
		payloadTemplate := configMap.Data["webhook.payloadTemplate"]

		config.Webhook = notifications.WebhookConfig{
			URL:             webhookURL,
			Headers:         headers,
			PayloadTemplate: payloadTemplate,
		}

		s.logger.Debug("Final webhook config",
			"url", config.Webhook.URL,
			"headerCount", len(config.Webhook.Headers),
			"hasPayloadTemplate", payloadTemplate != "")

	default:
		return nil, fmt.Errorf("unsupported notification channel type: %s", channelType)
	}

	return config, nil
}

// parseRecipientsList parses a string representation of recipients list
// The format is "[email1@example.com email2@example.com]" as stored by the controller
func parseRecipientsList(s string) []string {
	// Remove brackets if present
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)

	if s == "" {
		return nil
	}

	// Split by whitespace
	parts := strings.Fields(s)
	return parts
}

// renderTemplate performs CEL expression rendering by evaluating ${...} expressions using the template engine.
// If any CEL expression fails to resolve, a warning is logged and the original expression is preserved in the output.
func (s *LoggingService) renderTemplate(templateStr string, data map[string]interface{}) string {
	// Build the CEL input context with the alert data
	celInputs := map[string]any{
		"alertName":                       data["ruleName"],
		"alertTimestamp":                  data["timestamp"],
		"alertSeverity":                   data["severity"],
		"alertDescription":                data["description"],
		"alertThreshold":                  data["threshold"],
		"alertValue":                      data["value"],
		"alertType":                       data["type"],
		"component":                       data["component"],
		"project":                         data["project"],
		"environment":                     data["environment"],
		"componentId":                     data["componentUid"],
		"projectId":                       data["projectUid"],
		"environmentId":                   data["environmentUid"],
		"alertAIRootCauseAnalysisEnabled": data["enableAiRootCauseAnalysis"],
	}

	s.logger.Debug("CEL template rendering inputs", "alertData", celInputs)

	// Find all CEL expressions in the template
	expressions, err := template.FindCELExpressions(templateStr)
	if err != nil {
		s.logger.Warn("Failed to parse CEL expressions in template, returning original template",
			"error", err,
			"template", templateStr)
		return templateStr
	}

	// If no expressions found, return the template as-is
	if len(expressions) == 0 {
		return templateStr
	}

	// Create a new template engine for CEL evaluation
	engine := template.NewEngine()

	result := templateStr
	for _, match := range expressions {
		// Try to render this single expression
		rendered, err := engine.Render(match.FullExpr, celInputs)
		if err != nil {
			s.logger.Warn("Failed to resolve CEL expression, keeping original expression",
				"expression", match.FullExpr,
				"innerExpr", match.InnerExpr,
				"error", err)
			// Keep the original expression in the result
			continue
		}

		// Convert rendered result to string
		var replacement string
		switch v := rendered.(type) {
		case string:
			replacement = v
		default:
			replacement = fmt.Sprintf("%v", v)
		}

		// Replace only the first occurrence of this expression
		result = strings.Replace(result, match.FullExpr, replacement, 1)
	}

	return result
}

// StoreAlertEntry stores an alert entry in the logs backend and returns the alert ID
func (s *LoggingService) StoreAlertEntry(ctx context.Context, requestBody map[string]interface{}, ruleName string) (string, error) {
	alertEntry := map[string]interface{}{
		"@timestamp":      requestBody["timestamp"],
		"alert_rule_name": ruleName,
		"alert_value":     requestBody["alertValue"],
		"labels": map[string]interface{}{
			observerlabels.ComponentID:   requestBody["componentUid"],
			observerlabels.EnvironmentID: requestBody["environmentUid"],
			observerlabels.ProjectID:     requestBody["projectUid"],
		},
		"enable_ai_rca": requestBody["enableAiRootCauseAnalysis"],
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
// It enriches the payload with CRD data and sends an async request to the RCA service.
func (s *LoggingService) TriggerRCAAnalysis(ctx context.Context, rcaServiceURL string, alertID string, requestBody map[string]interface{}, alertRule *choreoapis.ObservabilityAlertRule) {
	ruleName, _ := requestBody["ruleName"].(string)

	// Build the rule info with basic name
	ruleInfo := map[string]interface{}{
		"name": ruleName,
	}

	// Enrich with CRD data if available
	if alertRule != nil {
		ruleInfo["namespace"] = alertRule.Namespace

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

		s.logger.Debug("Enriched RCA payload with ObservabilityAlertRule data", "ruleName", ruleName)
	}

	// Build the RCA service request payload
	rcaPayload := map[string]interface{}{
		"componentUid":   requestBody["componentUid"],
		"projectUid":     requestBody["projectUid"],
		"environmentUid": requestBody["environmentUid"],
		"alert": map[string]interface{}{
			"id":        alertID,
			"value":     requestBody["value"],
			"timestamp": requestBody["timestamp"],
			"rule":      ruleInfo,
		},
	}

	// Fire-and-forget request to AI RCA service
	go func() {
		payloadBytes, err := json.Marshal(rcaPayload)
		if err != nil {
			s.logger.Error("Failed to marshal RCA request payload", "error", err)
			return
		}

		resp, err := http.Post(rcaServiceURL+"/analyze", "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			s.logger.Error("Failed to send RCA analysis request", "error", err)
			return
		}
		if resp != nil {
			resp.Body.Close()
		}

		s.logger.Debug("AI RCA analysis triggered", "alertID", alertID)
	}()
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
func (s *LoggingService) EnrichAlertDetails(alertRule *choreoapis.ObservabilityAlertRule, alertValue string, timestamp string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"timestamp":                 timestamp,
		"severity":                  string(alertRule.Spec.Severity),
		"ruleName":                  alertRule.Spec.Name,
		"description":               alertRule.Spec.Description,
		"type":                      string(alertRule.Spec.Source.Type),
		"threshold":                 strconv.FormatInt(alertRule.Spec.Condition.Threshold, 10),
		"value":                     alertValue,
		"componentUid":              alertRule.Labels["openchoreo.dev/component-uid"],
		"environmentUid":            alertRule.Labels["openchoreo.dev/environment-uid"],
		"projectUid":                alertRule.Labels["openchoreo.dev/project-uid"],
		"component":                 alertRule.Labels["openchoreo.dev/component"],
		"project":                   alertRule.Labels["openchoreo.dev/project"],
		"environment":               alertRule.Labels["openchoreo.dev/environment"],
		"notificationChannel":       alertRule.Spec.NotificationChannel,
		"enableAiRootCauseAnalysis": alertRule.Spec.EnableAiRootCauseAnalysis,
	}, nil
}
