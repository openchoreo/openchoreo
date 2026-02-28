// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	defaultLimit      = 100
	maxLimit          = 10000
	defaultSortOrder  = "desc"
	maxQueryTimeRange = 30 * 24 * time.Hour // 30 days
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
	if *limit > maxLimit {
		return fmt.Errorf("limit cannot exceed %d", maxLimit)
	}
	return nil
}

// ValidateAndSetSortOrder validates and sets default for sort order
func ValidateAndSetSortOrder(sortOrder *string) error {
	if *sortOrder == "" {
		*sortOrder = defaultSortOrder
		return nil
	}
	if *sortOrder != "asc" && *sortOrder != "desc" {
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
