// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"time"
)

// strPtr returns a pointer to the string, or nil if the string is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseRFC3339Time parses a time string in RFC3339 format.
func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return t, nil
}

// setDefaults applies default values for common query parameters.
func setDefaults(limit int, sortOrder string, logLevels []string) (int, string, []string) {
	if limit == 0 {
		limit = 100
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if logLevels == nil {
		logLevels = []string{}
	}
	return limit, sortOrder, logLevels
}

// validateComponentScope validates that the required scope fields are present.
func validateComponentScope(namespace, project, component string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if component != "" && project == "" {
		return fmt.Errorf("project is required when component is provided")
	}
	return nil
}
