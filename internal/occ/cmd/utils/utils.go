// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"time"
)

// FormatAge returns a human-readable age string for a given timestamp.
func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "0s"
	}
	duration := time.Since(t)
	if duration < 0 {
		duration = 0
	}
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration.Hours() < 1 {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration.Hours() < 24 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
}
