// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
)

func TestValidateMinutesHoursDuration_Valid(t *testing.T) {
	tests := []string{
		"1m",
		"5m",
		"15m",
		"1h",
		"2h",
		"60m",
	}

	for _, tt := range tests {
		if err := validateMinutesHoursDuration(tt, "condition.window"); err != nil {
			t.Fatalf("expected %q to be valid, got error: %v", tt, err)
		}
	}
}

func TestValidateMinutesHoursDuration_Invalid(t *testing.T) {
	tests := []string{
		"30s",
		"90s",
		"1m0s",
		"1m30s",
		"2h15m10s",
		"0s",
		"0m",
		"-1m",
	}

	for _, tt := range tests {
		if err := validateMinutesHoursDuration(tt, "condition.window"); err == nil {
			t.Fatalf("expected %q to be invalid, but got no error", tt)
		}
	}
}
