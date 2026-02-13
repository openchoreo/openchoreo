// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// durationRegex matches duration strings with optional days, hours, minutes, and seconds
	// Supports formats like: "90d", "10d 1h 10m 100s", "10d1h10m100s", "1h30m", "1000s"
	durationRegex = regexp.MustCompile(`^(?:(\d+)d)?(?:\s*)(?:(\d+)h)?(?:\s*)(?:(\d+)m)?(?:\s*)(?:(\d+)s)?$`)
)

// ParseDuration parses a duration string that supports days in addition to standard Go duration units.
//
// Supported formats:
//   - "90d" (days only)
//   - "10d 1h 10m 100s" (multi-unit with spaces)
//   - "10d1h10m100s" (multi-unit without spaces)
//   - "1h30m" (standard Go format)
//   - "1000s" (seconds only)
//
// Units: d (days), h (hours), m (minutes), s (seconds)
//
// Returns:
//   - time.Duration: The parsed duration
//   - error: An error if the format is invalid or the duration is negative
//
// Examples:
//
//	ParseDuration("90d")          // 90 days
//	ParseDuration("10d 1h 30m")   // 10 days, 1 hour, 30 minutes
//	ParseDuration("1h30m")        // 1 hour, 30 minutes
//	ParseDuration("1000s")        // 1000 seconds
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("duration string is empty")
	}

	// Trim whitespace
	s = strings.TrimSpace(s)

	// Try standard Go duration parsing first (handles formats like "1h30m", "5m", "100s")
	if !strings.Contains(s, "d") {
		d, err := time.ParseDuration(s)
		if err == nil {
			if d < 0 {
				return 0, fmt.Errorf("duration must be non-negative: %s", s)
			}
			return d, nil
		}
	}

	// Parse custom format with days
	matches := durationRegex.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration format: %s (expected format: e.g., '90d', '10d 1h 30m', '1h30m')", s)
	}

	var totalDuration time.Duration

	// Parse days (matches[1])
	if matches[1] != "" {
		days, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid days value: %s", matches[1])
		}
		totalDuration += time.Duration(days) * 24 * time.Hour
	}

	// Parse hours (matches[2])
	if matches[2] != "" {
		hours, err := strconv.ParseInt(matches[2], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hours value: %s", matches[2])
		}
		totalDuration += time.Duration(hours) * time.Hour
	}

	// Parse minutes (matches[3])
	if matches[3] != "" {
		minutes, err := strconv.ParseInt(matches[3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes value: %s", matches[3])
		}
		totalDuration += time.Duration(minutes) * time.Minute
	}

	// Parse seconds (matches[4])
	if matches[4] != "" {
		seconds, err := strconv.ParseInt(matches[4], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds value: %s", matches[4])
		}
		totalDuration += time.Duration(seconds) * time.Second
	}

	// Ensure at least one unit was specified
	if totalDuration == 0 {
		return 0, fmt.Errorf("duration must specify at least one unit (d, h, m, s): %s", s)
	}

	return totalDuration, nil
}

// ValidateDuration validates that a duration string is valid and non-negative.
// Returns nil if valid, error otherwise.
func ValidateDuration(s string) error {
	if s == "" {
		return fmt.Errorf("duration string is empty")
	}

	_, err := ParseDuration(s)
	return err
}
