// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// Validates that the limit is a positive integer and does not exceed 10000
func validateLimit(limit *int) error {
	if *limit == 0 {
		*limit = 100
	} else if *limit <= 0 {
		return fmt.Errorf("limit must be a positive integer")
	} else if *limit > 10000 {
		return fmt.Errorf("limit cannot exceed 10000. If you need to fetch more logs, please use pagination")
	}

	return nil
}

// Validates that the sortOrder is either "asc" or "desc"
func validateSortOrder(sortOrder *string) error {
	if *sortOrder == "" {
		*sortOrder = defaultSortOrder
	} else if *sortOrder != "asc" && *sortOrder != "desc" {
		return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
	}
	return nil
}

// Validates that componentUIDs is a valid array of non-empty strings
func validateComponentUIDs(componentUIDs []string) error {
	if len(componentUIDs) == 0 {
		return nil // Empty array is valid (optional parameter)
	}

	for _, uid := range componentUIDs {
		if uid == "" {
			return fmt.Errorf("componentUid array cannot contain empty strings")
		}
	}

	return nil
}

// Validates the startTime and endTime strings. Performs the following checks
// 1. Both fields are present
// 2. Both fields are in RFC3339 format
// 3. endTime is after startTime
func validateTimes(startTime string, endTime string) error {
	if startTime == "" {
		return fmt.Errorf("Required field startTime not found")
	}

	if endTime == "" {
		return fmt.Errorf("Required field endTime not found")
	}

	// Validate time format
	if _, err := time.Parse(time.RFC3339, startTime); err != nil {
		return fmt.Errorf("startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	if _, err := time.Parse(time.RFC3339, endTime); err != nil {
		return fmt.Errorf("endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	// Validate that end time is after start time
	parsedStartTime, _ := time.Parse(time.RFC3339, startTime)
	parsedEndTime, _ := time.Parse(time.RFC3339, endTime)

	if parsedEndTime.Before(parsedStartTime) {
		return fmt.Errorf("endTime (%s) must be after startTime (%s)", parsedEndTime, parsedStartTime)
	}

	return nil
}

// validateTraceID validates a trace ID string that may contain wildcard characters (* and ?)
// Valid characters: hexadecimal digits (0-9, a-f, A-F) and wildcards (* and ?)
// Empty trace IDs are considered valid (optional parameter)
func validateTraceID(traceID string) error {
	if traceID == "" {
		return nil // Empty trace ID is valid (optional parameter)
	}

	// Check each character
	for i, char := range traceID {
		isValid := (char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F') ||
			char == '*' ||
			char == '?'

		if !isValid {
			return fmt.Errorf("traceId contains invalid character '%c' at position %d. Only hexadecimal characters (0-9, a-f, A-F) and wildcards (*, ?) are allowed", char, i)
		}
	}

	return nil
}

// validateAlertingRule ensures the alerting rule payload contains required fields
// and uses supported values.
func validateAlertingRule(req types.AlertingRuleRequest) error {
	// Metadata validations
	if strings.TrimSpace(req.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(req.Metadata.ComponentUID) == "" {
		return fmt.Errorf("metadata.component-uid is required")
	}
	if strings.TrimSpace(req.Metadata.ProjectUID) == "" {
		return fmt.Errorf("metadata.project-uid is required")
	}
	if strings.TrimSpace(req.Metadata.EnvironmentUID) == "" {
		return fmt.Errorf("metadata.environment-uid is required")
	}
	if strings.TrimSpace(req.Metadata.Severity) == "" {
		return fmt.Errorf("metadata.severity is required")
	}

	// Source validations
	if req.Source.Type != "log" { // TODO: Add validation for metric-based alert rules
		return fmt.Errorf("source.type must be 'log'")
	}
	if req.Source.Type == "log" && strings.TrimSpace(req.Source.Query) == "" {
		return fmt.Errorf("source.query is required for log-based alert rules")
	}

	// Condition validations
	windowDuration, err := time.ParseDuration(req.Condition.Window)
	if err != nil {
		return fmt.Errorf("condition.window must be a valid duration (e.g., 5m): %w", err)
	}
	intervalDuration, err := time.ParseDuration(req.Condition.Interval)
	if err != nil {
		return fmt.Errorf("condition.interval must be a valid duration (e.g., 1m): %w", err)
	}

	if intervalDuration <= 0 {
		return fmt.Errorf("condition.interval must be greater than zero")
	}
	if windowDuration <= 0 {
		return fmt.Errorf("condition.window must be greater than zero")
	}
	if intervalDuration > windowDuration {
		return fmt.Errorf("condition.interval must not exceed condition.window")
	}

	allowedOps := []string{"gt", "gte", "lt", "lte", "eq", "neq"}
	if !slices.Contains(allowedOps, req.Condition.Operator) {
		return fmt.Errorf("condition.operator must be one of %s", strings.Join(allowedOps, ", "))
	}

	if req.Condition.Threshold <= 0 {
		return fmt.Errorf("condition.threshold must be greater than zero")
	}

	return nil
}
