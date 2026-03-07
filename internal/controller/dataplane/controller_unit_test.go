// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestNewDataPlaneCreatedCondition(t *testing.T) {
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
			wantType:    string(ConditionCreated),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  string(ReasonDataPlaneCreated),
			wantMessage: "Dataplane is created",
		},
		{
			name:        "generation 5",
			generation:  5,
			wantType:    string(ConditionCreated),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  string(ReasonDataPlaneCreated),
			wantMessage: "Dataplane is created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDataPlaneCreatedCondition(tt.generation)

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

func TestNewDataPlaneFinalizingCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
	}{
		{name: "generation 0", generation: 0},
		{name: "generation 3", generation: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDataPlaneFinalizingCondition(tt.generation)

			if cond.Type != string(ConditionFinalizing) {
				t.Errorf("Type: got %q, want %q", cond.Type, string(ConditionFinalizing))
			}
			if cond.Status != metav1.ConditionTrue {
				t.Errorf("Status: got %q, want True", cond.Status)
			}
			if cond.Reason != string(ReasonDataplaneFinalizing) {
				t.Errorf("Reason: got %q, want %q", cond.Reason, string(ReasonDataplaneFinalizing))
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
		dp := &openchoreov1alpha1.DataPlane{}
		if r.shouldIgnoreReconcile(dp) {
			t.Error("expected false when no conditions are set")
		}
	})

	t.Run("returns false when only Finalizing condition set", func(t *testing.T) {
		dp := &openchoreov1alpha1.DataPlane{}
		dp.Status.Conditions = []metav1.Condition{
			NewDataPlaneFinalizingCondition(1),
		}
		if r.shouldIgnoreReconcile(dp) {
			t.Error("expected false when only Finalizing condition is set")
		}
	})

	t.Run("returns true when Created condition is set", func(t *testing.T) {
		dp := &openchoreov1alpha1.DataPlane{}
		dp.Status.Conditions = []metav1.Condition{
			NewDataPlaneCreatedCondition(1),
		}
		if !r.shouldIgnoreReconcile(dp) {
			t.Error("expected true when Created condition is present")
		}
	})

	t.Run("returns true when both Created and Finalizing conditions are set", func(t *testing.T) {
		dp := &openchoreov1alpha1.DataPlane{}
		dp.Status.Conditions = []metav1.Condition{
			NewDataPlaneCreatedCondition(1),
			NewDataPlaneFinalizingCondition(1),
		}
		if !r.shouldIgnoreReconcile(dp) {
			t.Error("expected true when Created condition is among the conditions")
		}
	})
}

func TestPopulateAgentConnectionStatus_NilGateway(t *testing.T) {
	r := &Reconciler{GatewayClient: nil}
	dp := &openchoreov1alpha1.DataPlane{}

	err := r.populateAgentConnectionStatus(context.Background(), dp)
	if err != nil {
		t.Errorf("expected nil error when GatewayClient is nil, got: %v", err)
	}
	if dp.Status.AgentConnection != nil {
		t.Error("expected AgentConnection to remain nil when GatewayClient is nil")
	}
}

func TestNewDeletionBlockedCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		message    string
	}{
		{name: "generation 1", generation: 1, message: "Deletion blocked: dataplane is still referenced by 2 environment(s)"},
		{name: "generation 3", generation: 3, message: "Deletion blocked: dataplane is still referenced by 1 environment(s)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDeletionBlockedCondition(tt.generation, tt.message)

			if cond.Type != string(ConditionFinalizing) {
				t.Errorf("Type: got %q, want %q", cond.Type, string(ConditionFinalizing))
			}
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("Status: got %q, want False", cond.Status)
			}
			if cond.Reason != string(ReasonDeletionBlocked) {
				t.Errorf("Reason: got %q, want %q", cond.Reason, string(ReasonDeletionBlocked))
			}
			if cond.Message != tt.message {
				t.Errorf("Message: got %q, want %q", cond.Message, tt.message)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("ObservedGeneration: got %d, want %d", cond.ObservedGeneration, tt.generation)
			}
		})
	}
}

func TestConditionConstants(t *testing.T) {
	t.Run("ConditionCreated matches TypeCreated", func(t *testing.T) {
		if string(ConditionCreated) != controller.TypeCreated {
			t.Errorf("ConditionCreated %q must equal controller.TypeCreated %q", ConditionCreated, controller.TypeCreated)
		}
	})

	t.Run("ConditionCreated is 'Created'", func(t *testing.T) {
		if string(ConditionCreated) != "Created" {
			t.Errorf("ConditionCreated: got %q, want \"Created\"", ConditionCreated)
		}
	})

	t.Run("ConditionFinalizing is 'Finalizing'", func(t *testing.T) {
		if string(ConditionFinalizing) != "Finalizing" {
			t.Errorf("ConditionFinalizing: got %q, want \"Finalizing\"", ConditionFinalizing)
		}
	})

	t.Run("DataPlaneCleanupFinalizer value", func(t *testing.T) {
		const want = "openchoreo.dev/dataplane-cleanup"
		if DataPlaneCleanupFinalizer != want {
			t.Errorf("DataPlaneCleanupFinalizer: got %q, want %q", DataPlaneCleanupFinalizer, want)
		}
	})

	t.Run("ReasonDataPlaneCreated value", func(t *testing.T) {
		const want = "DataPlaneCreated"
		if string(ReasonDataPlaneCreated) != want {
			t.Errorf("ReasonDataPlaneCreated: got %q, want %q", ReasonDataPlaneCreated, want)
		}
	})

	t.Run("ReasonDataplaneFinalizing value", func(t *testing.T) {
		const want = "DataplaneFinalizing"
		if string(ReasonDataplaneFinalizing) != want {
			t.Errorf("ReasonDataplaneFinalizing: got %q, want %q", ReasonDataplaneFinalizing, want)
		}
	})

	t.Run("ReasonDeletionBlocked value", func(t *testing.T) {
		const want = "DeletionBlocked"
		if string(ReasonDeletionBlocked) != want {
			t.Errorf("ReasonDeletionBlocked: got %q, want %q", ReasonDeletionBlocked, want)
		}
	})
}
