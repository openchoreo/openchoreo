// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestNewObservabilityPlaneCreatedCondition(t *testing.T) {
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
			wantReason:  "ObservabilityPlaneCreated",
			wantMessage: "Observabilityplane is created",
		},
		{
			name:        "generation 5",
			generation:  5,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ObservabilityPlaneCreated",
			wantMessage: "Observabilityplane is created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewObservabilityPlaneCreatedCondition(tt.generation)

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

func TestShouldIgnoreReconcile(t *testing.T) {
	r := &Reconciler{}

	t.Run("returns false when no conditions set", func(t *testing.T) {
		op := &openchoreov1alpha1.ObservabilityPlane{}
		if r.shouldIgnoreReconcile(op) {
			t.Error("expected false when no conditions are set")
		}
	})

	t.Run("returns true when Created condition is set", func(t *testing.T) {
		op := &openchoreov1alpha1.ObservabilityPlane{}
		op.Status.Conditions = []metav1.Condition{
			NewObservabilityPlaneCreatedCondition(1),
		}
		if !r.shouldIgnoreReconcile(op) {
			t.Error("expected true when Created condition is present")
		}
	})

	t.Run("returns false when unrelated condition is set", func(t *testing.T) {
		op := &openchoreov1alpha1.ObservabilityPlane{}
		op.Status.Conditions = []metav1.Condition{
			{
				Type:               "SomeOtherCondition",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "Test",
			},
		}
		if r.shouldIgnoreReconcile(op) {
			t.Error("expected false when only unrelated condition is set")
		}
	})
}

func TestPopulateAgentConnectionStatus_NilGateway(t *testing.T) {
	r := &Reconciler{GatewayClient: nil}
	op := &openchoreov1alpha1.ObservabilityPlane{}

	err := r.populateAgentConnectionStatus(context.Background(), op)
	if err != nil {
		t.Errorf("expected nil error when GatewayClient is nil, got: %v", err)
	}
	if op.Status.AgentConnection != nil {
		t.Error("expected AgentConnection to remain nil when GatewayClient is nil")
	}
}

func TestObservabilityPlaneCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/observabilityplane-cleanup"
	if ObservabilityPlaneCleanupFinalizer != want {
		t.Errorf("ObservabilityPlaneCleanupFinalizer: got %q, want %q", ObservabilityPlaneCleanupFinalizer, want)
	}
}
