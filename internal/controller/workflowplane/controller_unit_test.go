// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestNewWorkflowPlaneCreatedCondition(t *testing.T) {
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
			wantReason:  "WorkflowPlaneCreated",
			wantMessage: "Workflowplane is created",
		},
		{
			name:        "generation 5",
			generation:  5,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "WorkflowPlaneCreated",
			wantMessage: "Workflowplane is created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewWorkflowPlaneCreatedCondition(tt.generation)

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
		wp := &openchoreov1alpha1.WorkflowPlane{}
		if r.shouldIgnoreReconcile(wp) {
			t.Error("expected false when no conditions are set")
		}
	})

	t.Run("returns true when Created condition is set", func(t *testing.T) {
		wp := &openchoreov1alpha1.WorkflowPlane{}
		wp.Status.Conditions = []metav1.Condition{
			NewWorkflowPlaneCreatedCondition(1),
		}
		if !r.shouldIgnoreReconcile(wp) {
			t.Error("expected true when Created condition is present")
		}
	})

	t.Run("returns false when unrelated condition is set", func(t *testing.T) {
		wp := &openchoreov1alpha1.WorkflowPlane{}
		wp.Status.Conditions = []metav1.Condition{
			{
				Type:               "SomeOtherCondition",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "Test",
			},
		}
		if r.shouldIgnoreReconcile(wp) {
			t.Error("expected false when only unrelated condition is set")
		}
	})
}

func TestPopulateAgentConnectionStatus_NilGateway(t *testing.T) {
	r := &Reconciler{GatewayClient: nil}
	wp := &openchoreov1alpha1.WorkflowPlane{}

	err := r.populateAgentConnectionStatus(context.Background(), wp)
	if err != nil {
		t.Errorf("expected nil error when GatewayClient is nil, got: %v", err)
	}
	if wp.Status.AgentConnection != nil {
		t.Error("expected AgentConnection to remain nil when GatewayClient is nil")
	}
}

func TestWorkflowPlaneCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/workflowplane-cleanup"
	if WorkflowPlaneCleanupFinalizer != want {
		t.Errorf("WorkflowPlaneCleanupFinalizer: got %q, want %q", WorkflowPlaneCleanupFinalizer, want)
	}
}
