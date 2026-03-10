// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/clusterobservabilityplane"
)

func TestNewClusterObservabilityPlaneCreatedCondition(t *testing.T) {
	tests := []struct {
		name        string
		generation  int64
		wantType    string
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "generation 0",
			generation:  0,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterObservabilityPlaneCreated",
			wantMessage: "Observabilityplane is created",
		},
		{
			name:        "generation 1",
			generation:  1,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterObservabilityPlaneCreated",
			wantMessage: "Observabilityplane is created",
		},
		{
			name:        "generation 5",
			generation:  5,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterObservabilityPlaneCreated",
			wantMessage: "Observabilityplane is created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := clusterobservabilityplane.NewClusterObservabilityPlaneCreatedCondition(tt.generation)

			if cond.Type != tt.wantType {
				t.Errorf("Type: got %q, want %q", cond.Type, tt.wantType)
			}
			if cond.Status != tt.wantStatus {
				t.Errorf("Status: got %q, want %q", cond.Status, tt.wantStatus)
			}
			if cond.Reason != tt.wantReason {
				t.Errorf("Reason: got %q, want %q", cond.Reason, tt.wantReason)
			}
			if cond.Message != tt.wantMessage {
				t.Errorf("Message: got %q, want %q", cond.Message, tt.wantMessage)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("ObservedGeneration: got %d, want %d", cond.ObservedGeneration, tt.generation)
			}
		})
	}
}

func TestNewClusterObservabilityPlaneCreatedCondition_LastTransitionTimeSet(t *testing.T) {
	cond := clusterobservabilityplane.NewClusterObservabilityPlaneCreatedCondition(3)
	if cond.LastTransitionTime.IsZero() {
		t.Error("expected LastTransitionTime to be set, got zero value")
	}
}

func TestClusterObservabilityPlaneCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/clusterobservabilityplane-cleanup"
	if clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer != want {
		t.Errorf("ClusterObservabilityPlaneCleanupFinalizer: got %q, want %q",
			clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer, want)
	}
}
