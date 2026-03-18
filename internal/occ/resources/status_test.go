// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetResourceStatus(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-time.Hour))

	tests := []struct {
		name               string
		conditions         []metav1.Condition
		priorityConditions []string
		defaultStatus      string
		readyStatus        string
		notReadyStatus     string
		want               string
	}{
		{
			name:               "empty conditions returns default",
			conditions:         nil,
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Pending",
		},
		{
			name: "priority condition True",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "True", Reason: "AllGood", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Ready (AllGood)",
		},
		{
			name: "priority condition False with message",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "False", Reason: "BuildFailed", Message: "compilation error", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "NotReady (BuildFailed: compilation error)",
		},
		{
			name: "multiple priorities picks first match",
			conditions: []metav1.Condition{
				{Type: "Available", Status: "True", Reason: "MinReplicas", LastTransitionTime: now},
				{Type: "Ready", Status: "False", Reason: "NotYet", Message: "waiting", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready", "Available"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "NotReady (NotYet: waiting)",
		},
		{
			name: "no priority match falls back to latest True condition",
			conditions: []metav1.Condition{
				{Type: "Synced", Status: "True", Reason: "ReconcileSuccess", LastTransitionTime: earlier},
				{Type: "Available", Status: "True", Reason: "MinReplicas", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Available: MinReplicas",
		},
		{
			name: "no priority match falls back to latest False condition",
			conditions: []metav1.Condition{
				{Type: "Synced", Status: "True", Reason: "OK", LastTransitionTime: earlier},
				{Type: "Available", Status: "False", Reason: "NoReplicas", Message: "0 available", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Available: False - 0 available",
		},
		{
			name: "single non-priority condition True",
			conditions: []metav1.Condition{
				{Type: "Synced", Status: "True", Reason: "OK", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Synced: OK",
		},
		{
			name: "single non-priority condition False",
			conditions: []metav1.Condition{
				{Type: "Synced", Status: "False", Reason: "Error", Message: "sync failed", LastTransitionTime: now},
			},
			priorityConditions: []string{"Ready"},
			defaultStatus:      "Pending",
			readyStatus:        "Ready",
			notReadyStatus:     "NotReady",
			want:               "Synced: False - sync failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetResourceStatus(tt.conditions, tt.priorityConditions, tt.defaultStatus, tt.readyStatus, tt.notReadyStatus)
			if got != tt.want {
				t.Errorf("GetResourceStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetReadyStatus(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name           string
		conditions     []metav1.Condition
		defaultStatus  string
		readyStatus    string
		notReadyStatus string
		want           string
	}{
		{
			name:           "empty conditions returns default",
			conditions:     nil,
			defaultStatus:  "Pending",
			readyStatus:    "Ready",
			notReadyStatus: "NotReady",
			want:           "Pending",
		},
		{
			name: "Ready True delegates correctly",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "True", Reason: "AllGood", LastTransitionTime: now},
			},
			defaultStatus:  "Pending",
			readyStatus:    "Ready",
			notReadyStatus: "NotReady",
			want:           "Ready (AllGood)",
		},
		{
			name: "Ready False delegates correctly",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: "False", Reason: "Failing", Message: "pods crashing", LastTransitionTime: now},
			},
			defaultStatus:  "Pending",
			readyStatus:    "Ready",
			notReadyStatus: "NotReady",
			want:           "NotReady (Failing: pods crashing)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetReadyStatus(tt.conditions, tt.defaultStatus, tt.readyStatus, tt.notReadyStatus)
			if got != tt.want {
				t.Errorf("GetReadyStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
