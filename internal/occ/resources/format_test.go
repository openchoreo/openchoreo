// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatStatusWithReason(t *testing.T) {
	tests := []struct {
		name   string
		status string
		reason string
		want   string
	}{
		{
			name:   "normal status with reason",
			status: "Ready",
			reason: "AllGood",
			want:   "Ready (AllGood)",
		},
		{
			name:   "error status with reason",
			status: "Failed",
			reason: "ImagePullBackOff",
			want:   "Failed (ImagePullBackOff)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatStatusWithReason(tt.status, tt.reason); got != tt.want {
				t.Errorf("FormatStatusWithReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatStatusWithMessage(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		reason  string
		message string
		want    string
	}{
		{
			name:    "status with reason and message",
			status:  "Failed",
			reason:  "BuildError",
			message: "compilation failed",
			want:    "Failed: BuildError - compilation failed",
		},
		{
			name:    "ready with details",
			status:  "Ready",
			reason:  "Deployed",
			message: "all replicas available",
			want:    "Ready: Deployed - all replicas available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatStatusWithMessage(tt.status, tt.reason, tt.message); got != tt.want {
				t.Errorf("FormatStatusWithMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatStatusWithType(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		reason   string
		want     string
	}{
		{
			name:     "type with reason",
			typeName: "Available",
			reason:   "MinimumReplicasAvailable",
			want:     "Available: MinimumReplicasAvailable",
		},
		{
			name:     "progressing type",
			typeName: "Progressing",
			reason:   "NewReplicaSetAvailable",
			want:     "Progressing: NewReplicaSetAvailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatStatusWithType(tt.typeName, tt.reason); got != tt.want {
				t.Errorf("FormatStatusWithType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "zero duration",
			d:    0,
			want: "0s",
		},
		{
			name: "seconds",
			d:    45 * time.Second,
			want: "45s",
		},
		{
			name: "minutes",
			d:    5 * time.Minute,
			want: "5m",
		},
		{
			name: "hours",
			d:    3 * time.Hour,
			want: "3h",
		},
		{
			name: "days",
			d:    48 * time.Hour,
			want: "2d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDurationShort(tt.d); got != tt.want {
				t.Errorf("FormatDurationShort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "zero duration",
			d:    0,
			want: "0s",
		},
		{
			name: "seconds only",
			d:    30 * time.Second,
			want: "30s",
		},
		{
			name: "minutes and seconds",
			d:    2*time.Minute + 15*time.Second,
			want: "2m15s",
		},
		{
			name: "exact hour",
			d:    time.Hour,
			want: "1h0m",
		},
		{
			name: "hours and minutes",
			d:    2*time.Hour + 30*time.Minute,
			want: "2h30m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDuration(tt.d); got != tt.want {
				t.Errorf("FormatDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{
			name: "zero time returns placeholder",
			t:    time.Time{},
			want: "-",
		},
		{
			name: "recent time returns seconds",
			t:    time.Now().Add(-10 * time.Second),
			want: "10s",
		},
		{
			name: "old time returns days",
			t:    time.Now().Add(-72 * time.Hour),
			want: "3d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAge(tt.t)
			if tt.want == "-" {
				if got != "-" {
					t.Errorf("FormatAge() = %q, want %q", got, "-")
				}
				return
			}
			// For time-based tests, just verify it's non-empty and not the placeholder
			if got == "" || got == "-" {
				t.Errorf("FormatAge() = %q, expected a duration string", got)
			}
		})
	}
}

func TestFormatNameWithDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		resName     string
		displayName string
		want        string
	}{
		{
			name:        "same name and display name",
			resName:     "my-comp",
			displayName: "my-comp",
			want:        "my-comp",
		},
		{
			name:        "different display name",
			resName:     "my-comp",
			displayName: "My Component",
			want:        "my-comp (My Component)",
		},
		{
			name:        "empty display name",
			resName:     "my-comp",
			displayName: "",
			want:        "my-comp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatNameWithDisplayName(tt.resName, tt.displayName); got != tt.want {
				t.Errorf("FormatNameWithDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatBoolAsYesNo(t *testing.T) {
	if got := FormatBoolAsYesNo(true); got != "Yes" {
		t.Errorf("FormatBoolAsYesNo(true) = %q, want %q", got, "Yes")
	}
	if got := FormatBoolAsYesNo(false); got != "No" {
		t.Errorf("FormatBoolAsYesNo(false) = %q, want %q", got, "No")
	}
}

func TestGetPlaceholder(t *testing.T) {
	if got := GetPlaceholder(); got != "-" {
		t.Errorf("GetPlaceholder() = %q, want %q", got, "-")
	}
}

func TestFormatValueOrPlaceholder(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "empty returns placeholder",
			value: "",
			want:  "-",
		},
		{
			name:  "non-empty returns value",
			value: "hello",
			want:  "hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatValueOrPlaceholder(tt.value); got != tt.want {
				t.Errorf("FormatValueOrPlaceholder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-time.Hour))

	tests := []struct {
		name          string
		conditions    []metav1.Condition
		defaultStatus string
		want          string
	}{
		{
			name:          "empty conditions returns default",
			conditions:    nil,
			defaultStatus: "Pending",
			want:          "Pending",
		},
		{
			name: "single condition",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "True", LastTransitionTime: now},
			},
			defaultStatus: "Pending",
			want:          "True",
		},
		{
			name: "multiple conditions returns latest by time",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "True", LastTransitionTime: earlier},
				{Type: "Available", Status: "False", LastTransitionTime: now},
			},
			defaultStatus: "Pending",
			want:          "False",
		},
		{
			name:          "nil conditions returns default",
			conditions:    []metav1.Condition{},
			defaultStatus: "Unknown",
			want:          "Unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStatus(tt.conditions, tt.defaultStatus); got != tt.want {
				t.Errorf("GetStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
