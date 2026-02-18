// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testStatusReadyAllGood = "Ready (AllGood)"
	testStatusPending      = "Pending"
)

// ---- FormatStatusWithReason ----

func TestFormatStatusWithReason(t *testing.T) {
	got := FormatStatusWithReason("Ready", "AllGood")
	want := testStatusReadyAllGood
	if got != want {
		t.Errorf("FormatStatusWithReason = %q, want %q", got, want)
	}
}

func TestFormatStatusWithReason_EmptyReason(t *testing.T) {
	got := FormatStatusWithReason("Ready", "")
	want := "Ready ()"
	if got != want {
		t.Errorf("FormatStatusWithReason empty reason = %q, want %q", got, want)
	}
}

// ---- FormatStatusWithMessage ----

func TestFormatStatusWithMessage(t *testing.T) {
	got := FormatStatusWithMessage("Failed", "Error", "connection refused")
	want := "Failed: Error - connection refused"
	if got != want {
		t.Errorf("FormatStatusWithMessage = %q, want %q", got, want)
	}
}

// ---- FormatStatusWithType ----

func TestFormatStatusWithType(t *testing.T) {
	got := FormatStatusWithType("Ready", "AllGood")
	want := "Ready: AllGood"
	if got != want {
		t.Errorf("FormatStatusWithType = %q, want %q", got, want)
	}
}

// ---- FormatAge ----

func TestFormatAge_Zero(t *testing.T) {
	got := FormatAge(time.Time{})
	if got != "-" {
		t.Errorf("FormatAge(zero) = %q, want %q", got, "-")
	}
}

func TestFormatAge_Recent(t *testing.T) {
	t1 := time.Now().Add(-30 * time.Second)
	got := FormatAge(t1)
	if got == "-" || got == "" {
		t.Errorf("FormatAge(30s ago) = %q, should not be empty/dash", got)
	}
}

// ---- FormatDurationShort ----

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
		{"less than minute", 30 * time.Second, "30s"},
		{"exactly one minute", 60 * time.Second, "1m"},
		{"exactly one hour", 60 * time.Minute, "1h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationShort(tt.d)
			if got != tt.expected {
				t.Errorf("FormatDurationShort(%v) = %q, want %q", tt.d, got, tt.expected)
			}
		})
	}
}

// ---- FormatDuration ----

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 2*time.Minute + 30*time.Second, "2m30s"},
		{"hours and minutes", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"exactly 1 minute", 60 * time.Second, "1m0s"},
		{"exactly 1 hour", 60 * time.Minute, "1h0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.expected)
			}
		})
	}
}

// ---- FormatNameWithDisplayName ----

func TestFormatNameWithDisplayName_Different(t *testing.T) {
	got := FormatNameWithDisplayName("my-project", "My Project")
	want := "my-project (My Project)"
	if got != want {
		t.Errorf("FormatNameWithDisplayName = %q, want %q", got, want)
	}
}

func TestFormatNameWithDisplayName_Same(t *testing.T) {
	got := FormatNameWithDisplayName("my-project", "my-project")
	if got != "my-project" {
		t.Errorf("FormatNameWithDisplayName (same) = %q, want %q", got, "my-project")
	}
}

func TestFormatNameWithDisplayName_EmptyDisplay(t *testing.T) {
	got := FormatNameWithDisplayName("my-project", "")
	if got != "my-project" {
		t.Errorf("FormatNameWithDisplayName (empty display) = %q, want %q", got, "my-project")
	}
}

// ---- FormatBoolAsYesNo ----

func TestFormatBoolAsYesNo(t *testing.T) {
	if FormatBoolAsYesNo(true) != "Yes" {
		t.Errorf("FormatBoolAsYesNo(true) = %q, want Yes", FormatBoolAsYesNo(true))
	}
	if FormatBoolAsYesNo(false) != "No" {
		t.Errorf("FormatBoolAsYesNo(false) = %q, want No", FormatBoolAsYesNo(false))
	}
}

// ---- GetPlaceholder ----

func TestGetPlaceholder(t *testing.T) {
	got := GetPlaceholder()
	if got != "-" {
		t.Errorf("GetPlaceholder() = %q, want -", got)
	}
}

// ---- FormatValueOrPlaceholder ----

func TestFormatValueOrPlaceholder_NonEmpty(t *testing.T) {
	got := FormatValueOrPlaceholder("hello")
	if got != "hello" {
		t.Errorf("FormatValueOrPlaceholder(non-empty) = %q, want hello", got)
	}
}

func TestFormatValueOrPlaceholder_Empty(t *testing.T) {
	got := FormatValueOrPlaceholder("")
	if got != "-" {
		t.Errorf("FormatValueOrPlaceholder(empty) = %q, want -", got)
	}
}

// ---- GetStatus ----

func TestGetStatus_NoConditions(t *testing.T) {
	got := GetStatus(nil, testStatusPending)
	if got != testStatusPending {
		t.Errorf("GetStatus(no conditions) = %q, want Pending", got)
	}
}

func TestGetStatus_SingleCondition(t *testing.T) {
	now := metav1.Now()
	conditions := []metav1.Condition{
		{
			Type:               "Ready",
			Status:             "True",
			LastTransitionTime: now,
		},
	}
	got := GetStatus(conditions, testStatusPending)
	if got != "True" {
		t.Errorf("GetStatus(single condition) = %q, want True", got)
	}
}

func TestGetStatus_MultipleConditions_ReturnsLatest(t *testing.T) {
	now := metav1.Now()
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	conditions := []metav1.Condition{
		{
			Type:               "OldCondition",
			Status:             "False",
			LastTransitionTime: past,
		},
		{
			Type:               "NewCondition",
			Status:             "True",
			LastTransitionTime: now,
		},
	}
	got := GetStatus(conditions, testStatusPending)
	if got != "True" {
		t.Errorf("GetStatus(multiple) = %q, want True (from latest condition)", got)
	}
}
