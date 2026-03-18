// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatStatusWithReason(t *testing.T) {
	tests := []struct {
		name   string
		status string
		reason string
		want   string
	}{
		{name: "normal status with reason", status: "Ready", reason: "AllGood", want: "Ready (AllGood)"},
		{name: "error status with reason", status: "Failed", reason: "ImagePullBackOff", want: "Failed (ImagePullBackOff)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatStatusWithReason(tt.status, tt.reason))
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
		{name: "status with reason and message", status: "Failed", reason: "BuildError", message: "compilation failed", want: "Failed: BuildError - compilation failed"},
		{name: "ready with details", status: "Ready", reason: "Deployed", message: "all replicas available", want: "Ready: Deployed - all replicas available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatStatusWithMessage(tt.status, tt.reason, tt.message))
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
		{name: "type with reason", typeName: "Available", reason: "MinimumReplicasAvailable", want: "Available: MinimumReplicasAvailable"},
		{name: "progressing type", typeName: "Progressing", reason: "NewReplicaSetAvailable", want: "Progressing: NewReplicaSetAvailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatStatusWithType(tt.typeName, tt.reason))
		})
	}
}

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero duration", d: 0, want: "0s"},
		{name: "seconds", d: 45 * time.Second, want: "45s"},
		{name: "minutes", d: 5 * time.Minute, want: "5m"},
		{name: "hours", d: 3 * time.Hour, want: "3h"},
		{name: "days", d: 48 * time.Hour, want: "2d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDurationShort(tt.d))
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero duration", d: 0, want: "0s"},
		{name: "seconds only", d: 30 * time.Second, want: "30s"},
		{name: "minutes and seconds", d: 2*time.Minute + 15*time.Second, want: "2m15s"},
		{name: "exact hour", d: time.Hour, want: "1h0m"},
		{name: "hours and minutes", d: 2*time.Hour + 30*time.Minute, want: "2h30m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDuration(tt.d))
		})
	}
}

func TestFormatAge(t *testing.T) {
	t.Run("zero time returns placeholder", func(t *testing.T) {
		assert.Equal(t, "-", FormatAge(time.Time{}))
	})

	t.Run("recent time returns non-empty duration", func(t *testing.T) {
		got := FormatAge(time.Now().Add(-10 * time.Second))
		assert.NotEmpty(t, got)
		assert.NotEqual(t, "-", got)
	})

	t.Run("old time returns non-empty duration", func(t *testing.T) {
		got := FormatAge(time.Now().Add(-72 * time.Hour))
		assert.NotEmpty(t, got)
		assert.NotEqual(t, "-", got)
	})
}

func TestFormatNameWithDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		resName     string
		displayName string
		want        string
	}{
		{name: "same name and display name", resName: "my-comp", displayName: "my-comp", want: "my-comp"},
		{name: "different display name", resName: "my-comp", displayName: "My Component", want: "my-comp (My Component)"},
		{name: "empty display name", resName: "my-comp", displayName: "", want: "my-comp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatNameWithDisplayName(tt.resName, tt.displayName))
		})
	}
}

func TestFormatBoolAsYesNo(t *testing.T) {
	assert.Equal(t, "Yes", FormatBoolAsYesNo(true))
	assert.Equal(t, "No", FormatBoolAsYesNo(false))
}

func TestGetPlaceholder(t *testing.T) {
	assert.Equal(t, "-", GetPlaceholder())
}

func TestFormatValueOrPlaceholder(t *testing.T) {
	assert.Equal(t, "-", FormatValueOrPlaceholder(""))
	assert.Equal(t, "hello", FormatValueOrPlaceholder("hello"))
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
		{name: "empty conditions returns default", conditions: nil, defaultStatus: "Pending", want: "Pending"},
		{name: "single condition", conditions: []metav1.Condition{
			{Type: "Ready", Status: "True", LastTransitionTime: now},
		}, defaultStatus: "Pending", want: "True"},
		{name: "multiple conditions returns latest by time", conditions: []metav1.Condition{
			{Type: "Ready", Status: "True", LastTransitionTime: earlier},
			{Type: "Available", Status: "False", LastTransitionTime: now},
		}, defaultStatus: "Pending", want: "False"},
		{name: "empty slice returns default", conditions: []metav1.Condition{}, defaultStatus: "Unknown", want: "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetStatus(tt.conditions, tt.defaultStatus))
		})
	}
}
