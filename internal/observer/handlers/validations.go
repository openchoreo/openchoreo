// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"time"
)

// Validates that the limit is a positive integer and does not exceed 10000
func validateLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be a positive integer")
	}

	if limit > 10000 {
		return fmt.Errorf("limit cannot exceed 10000. If you need to fetch more logs, please use pagination")
	}

	return nil
}

// Validates that the sortOrder is either "asc" or "desc"
func validateSortOrder(sortOrder string) error {
	if sortOrder != "asc" && sortOrder != "desc" {
		return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
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
