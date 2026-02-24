// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"time"
)

// FormatAge returns a human-readable age string for a given timestamp.
func FormatAge(t time.Time) string {
	duration := time.Since(t)
	if duration.Hours() < 1 {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration.Hours() < 24 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
}
