// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"slices"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	defaultLimit      = 100
	defaultSortOrder  = "desc"
	sortOrderAsc      = "asc"
	maxQueryTimeRange = 30 * 24 * time.Hour // 30 days

	sourceTypeLog    = "log"
	sourceTypeMetric = "metric"
)

// ValidateLogsQueryRequest validates the LogsQueryRequest
func ValidateLogsQueryRequest(req *types.LogsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	// Validate search scope
	if req.SearchScope == nil {
		return fmt.Errorf("searchScope is required")
	}

	// Exactly one of component or workflow must be set
	if req.SearchScope.Component == nil && req.SearchScope.Workflow == nil {
		return fmt.Errorf("searchScope must be either a ComponentSearchScope (with namespace, and optionally project/component/environment) or WorkflowSearchScope (with namespace, and optionally workflowRunName)")
	}
	if req.SearchScope.Component != nil && req.SearchScope.Workflow != nil {
		return fmt.Errorf("searchScope cannot be both ComponentSearchScope and WorkflowSearchScope")
	}

	// Validate component scope if present
	if req.SearchScope.Component != nil {
		if err := validateComponentScope(req.SearchScope.Component); err != nil {
			return err
		}
	}

	// Validate workflow scope if present
	if req.SearchScope.Workflow != nil {
		if err := validateWorkflowScope(req.SearchScope.Workflow); err != nil {
			return err
		}
	}

	// Validate time range
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	// Validate and set defaults for limit
	if err := ValidateAndSetLimit(&req.Limit); err != nil {
		return err
	}

	// Validate and set defaults for sort order
	if err := ValidateAndSetSortOrder(&req.SortOrder); err != nil {
		return err
	}

	// Validate log levels if provided
	if err := ValidateLogLevels(req.LogLevels); err != nil {
		return err
	}

	return nil
}

// validateComponentScope validates the ComponentSearchScope
// Per OpenAPI spec, only namespace is required
func validateComponentScope(scope *types.ComponentSearchScope) error {
	if scope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// project, component, and environment are optional per OpenAPI spec
	// but if component is provided, project is required
	if scope.Project == "" && scope.Component != "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	return nil
}

// validateWorkflowScope validates the WorkflowSearchScope
// Per OpenAPI spec, only namespace is required
func validateWorkflowScope(scope *types.WorkflowSearchScope) error {
	if scope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// workflowRunName is optional per OpenAPI spec
	return nil
}

// ValidateTimeRange validates start and end time strings
func ValidateTimeRange(startTime, endTime string) error {
	if startTime == "" {
		return fmt.Errorf("startTime is required")
	}
	if endTime == "" {
		return fmt.Errorf("endTime is required")
	}

	parsedStart, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return fmt.Errorf("startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	parsedEnd, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return fmt.Errorf("endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	if parsedEnd.Before(parsedStart) {
		return fmt.Errorf("endTime must be after startTime")
	}

	if parsedEnd.Sub(parsedStart) > maxQueryTimeRange {
		return fmt.Errorf("query time range cannot exceed %d days", maxQueryTimeRange/24/time.Hour)
	}

	return nil
}

// ValidateAndSetLimit validates and sets default for limit
func ValidateAndSetLimit(limit *int) error {
	if *limit == 0 {
		*limit = defaultLimit
		return nil
	}
	if *limit < 0 {
		return fmt.Errorf("limit must be a positive integer")
	}
	if *limit > config.MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
	}
	return nil
}

// ValidateAndSetSortOrder validates and sets default for sort order
func ValidateAndSetSortOrder(sortOrder *string) error {
	if *sortOrder == "" {
		*sortOrder = defaultSortOrder
		return nil
	}
	if *sortOrder != sortOrderAsc && *sortOrder != defaultSortOrder {
		return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
	}
	return nil
}

// ValidateMetricsQueryRequest validates the MetricsQueryRequest
func ValidateMetricsQueryRequest(req *types.MetricsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request must not be nil")
	}

	// Validate metric type
	if req.Metric == "" {
		return fmt.Errorf("metric is required")
	}
	if req.Metric != types.MetricTypeResource && req.Metric != types.MetricTypeHTTP {
		return fmt.Errorf("metric must be either %s or %s", types.MetricTypeResource, types.MetricTypeHTTP)
	}

	// Validate time range
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	// Validate searchScope (required for metrics)
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if req.SearchScope.Component != "" && req.SearchScope.Project == "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}

	// Validate step format if provided
	if req.Step != nil && *req.Step != "" {
		step, err := time.ParseDuration(*req.Step)
		if err != nil {
			return fmt.Errorf("step must be a valid duration (e.g. 1m, 5m, 15m, 30m, 1h): %w", err)
		}
		if step <= 0 {
			return fmt.Errorf("step must be greater than 0")
		}
	}

	return nil
}

// ValidateLogLevels validates the log levels array
func ValidateLogLevels(logLevels []string) error {
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	seen := make(map[string]struct{}, len(logLevels))

	for _, level := range logLevels {
		if !validLevels[level] {
			return fmt.Errorf("invalid log level '%s'; valid levels are: DEBUG, INFO, WARN, ERROR", level)
		}
		if _, exists := seen[level]; exists {
			return fmt.Errorf("duplicate log level '%s' is not allowed", level)
		}
		seen[level] = struct{}{}
	}
	return nil
}

// validateAlertRuleRequest validates the new API AlertRuleRequest type.
func validateAlertRuleRequest(req gen.AlertRuleRequest) error {
	// Metadata validations
	if strings.TrimSpace(req.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if req.Metadata.ComponentUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.componentUid is required")
	}
	if req.Metadata.ProjectUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.projectUid is required")
	}
	if req.Metadata.EnvironmentUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.environmentUid is required")
	}

	// Source validations
	sourceType := string(req.Source.Type)
	if sourceType != sourceTypeLog && sourceType != sourceTypeMetric {
		return fmt.Errorf("source.type must be 'log' or 'metric'")
	}
	if sourceType == sourceTypeLog {
		if req.Source.Query == nil || strings.TrimSpace(*req.Source.Query) == "" {
			return fmt.Errorf("source.query is required for log-based alert rules")
		}
	}
	if sourceType == sourceTypeMetric {
		if req.Source.Metric == nil {
			return fmt.Errorf("source.metric is required for metric-based alert rules")
		}
		metric := string(*req.Source.Metric)
		if metric != "cpu_usage" && metric != "memory_usage" {
			return fmt.Errorf("source.metric must be 'cpu_usage' or 'memory_usage' for metric-based alert rules")
		}
	}

	// Condition validations
	windowDuration, err := time.ParseDuration(req.Condition.Window)
	if err != nil {
		return fmt.Errorf("condition.window must be a valid duration (e.g., 5m): %w", err)
	}
	if windowDuration <= 0 {
		return fmt.Errorf("condition.window must be greater than zero")
	}

	intervalDuration, err := time.ParseDuration(req.Condition.Interval)
	if err != nil {
		return fmt.Errorf("condition.interval must be a valid duration (e.g., 1m): %w", err)
	}
	if intervalDuration <= 0 {
		return fmt.Errorf("condition.interval must be greater than zero")
	}
	if intervalDuration > windowDuration {
		return fmt.Errorf("condition.interval must not exceed condition.window")
	}

	allowedOps := []string{"gt", "gte", "lt", "lte", "eq", "neq"}
	if !slices.Contains(allowedOps, string(req.Condition.Operator)) {
		return fmt.Errorf("condition.operator must be one of %s", strings.Join(allowedOps, ", "))
	}

	if req.Condition.Threshold <= 0 {
		return fmt.Errorf("condition.threshold must be greater than zero")
	}

	return nil
}

// validateSourceType checks that the sourceType path parameter is a known value.
// Returns an error with a descriptive message for use in a 400 Bad Request response.
func validateSourceType(sourceType string) error {
	switch sourceType {
	case sourceTypeLog, sourceTypeMetric:
		return nil
	default:
		return fmt.Errorf("sourceType %q is invalid: must be 'log' or 'metric'", sourceType)
	}
}

// ValidateTracesQueryRequest validates a traces query request
func ValidateTracesQueryRequest(req *gen.TracesQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	// Validate time range (generated types use time.Time directly)
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	startStr := req.StartTime.Format(time.RFC3339)
	endStr := req.EndTime.Format(time.RFC3339)
	if err := ValidateTimeRange(startStr, endStr); err != nil {
		return err
	}

	// Validate search scope (required for traces)
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// Handle pointer fields in SearchScope
	componentVal := ""
	if req.SearchScope.Component != nil {
		componentVal = *req.SearchScope.Component
	}
	projectVal := ""
	if req.SearchScope.Project != nil {
		projectVal = *req.SearchScope.Project
	}
	if componentVal != "" && projectVal == "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}

	// Validate and set defaults for limit
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}

	// Validate and set defaults for sort order
	if req.SortOrder != nil {
		validSort := string(*req.SortOrder)
		if validSort != sortOrderAsc && validSort != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}

	return nil
}

// ValidateAlertsQueryRequest validates an alerts query request.
func ValidateAlertsQueryRequest(req *gen.AlertsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	if err := ValidateTimeRange(req.StartTime.Format(time.RFC3339), req.EndTime.Format(time.RFC3339)); err != nil {
		return err
	}
	trimmedNamespace := strings.TrimSpace(req.SearchScope.Namespace)
	if trimmedNamespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	req.SearchScope.Namespace = trimmedNamespace
	if req.SearchScope.Component != nil && strings.TrimSpace(*req.SearchScope.Component) != "" &&
		(req.SearchScope.Project == nil || strings.TrimSpace(*req.SearchScope.Project) == "") {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}
	if req.SortOrder != nil {
		order := string(*req.SortOrder)
		if order != sortOrderAsc && order != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}
	return nil
}

// ValidateIncidentsQueryRequest validates an incidents query request.
func ValidateIncidentsQueryRequest(req *gen.IncidentsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	if err := ValidateTimeRange(req.StartTime.Format(time.RFC3339), req.EndTime.Format(time.RFC3339)); err != nil {
		return err
	}
	trimmedNamespace := strings.TrimSpace(req.SearchScope.Namespace)
	if trimmedNamespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	req.SearchScope.Namespace = trimmedNamespace
	if req.SearchScope.Component != nil && strings.TrimSpace(*req.SearchScope.Component) != "" &&
		(req.SearchScope.Project == nil || strings.TrimSpace(*req.SearchScope.Project) == "") {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}
	if req.SortOrder != nil {
		order := string(*req.SortOrder)
		if order != sortOrderAsc && order != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}
	return nil
}

// ValidateIncidentPutRequest validates an incident update (PUT) request.
func ValidateIncidentPutRequest(req *gen.IncidentPutRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	status := strings.TrimSpace(string(req.Status))
	if status == "" {
		return fmt.Errorf("status is required")
	}
	switch status {
	case string(gen.IncidentPutRequestStatusActive),
		string(gen.IncidentPutRequestStatusAcknowledged),
		string(gen.IncidentPutRequestStatusResolved):
		// valid
	default:
		return fmt.Errorf("status must be one of 'active', 'acknowledged', or 'resolved'")
	}
	return nil
}
