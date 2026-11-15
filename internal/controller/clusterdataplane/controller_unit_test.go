// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"context"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

// ---------------------------------------------------------------------------
// NewClusterDataPlaneCreatedCondition
// ---------------------------------------------------------------------------

func TestNewClusterDataPlaneCreatedCondition(t *testing.T) {
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
			wantReason:  "ClusterDataPlaneCreated",
			wantMessage: "ClusterDataplane is created",
		},
		{
			name:        "generation 1",
			generation:  1,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterDataPlaneCreated",
			wantMessage: "ClusterDataplane is created",
		},
		{
			name:        "large generation",
			generation:  99,
			wantType:    controller.TypeCreated,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterDataPlaneCreated",
			wantMessage: "ClusterDataplane is created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewClusterDataPlaneCreatedCondition(tt.generation)

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

// ---------------------------------------------------------------------------
// NewClusterDataPlaneFinalizingCondition
// ---------------------------------------------------------------------------

func TestNewClusterDataPlaneFinalizingCondition(t *testing.T) {
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
			wantType:    "Finalizing",
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterDataplaneFinalizing",
			wantMessage: "ClusterDataplane is finalizing",
		},
		{
			name:        "generation 3",
			generation:  3,
			wantType:    "Finalizing",
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "ClusterDataplaneFinalizing",
			wantMessage: "ClusterDataplane is finalizing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewClusterDataPlaneFinalizingCondition(tt.generation)

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

// ---------------------------------------------------------------------------
// shouldIgnoreReconcile
// ---------------------------------------------------------------------------

func TestShouldIgnoreReconcile(t *testing.T) {
	r := &Reconciler{}

	t.Run("returns false when no conditions set", func(t *testing.T) {
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		if r.shouldIgnoreReconcile(cdp) {
			t.Error("expected false when no conditions are set")
		}
	})

	t.Run("returns true when Created condition is set", func(t *testing.T) {
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		cdp.Status.Conditions = []metav1.Condition{
			NewClusterDataPlaneCreatedCondition(1),
		}
		if !r.shouldIgnoreReconcile(cdp) {
			t.Error("expected true when Created condition is present")
		}
	})

	t.Run("returns false when only unrelated condition is set", func(t *testing.T) {
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		cdp.Status.Conditions = []metav1.Condition{
			{
				Type:               "SomeOtherCondition",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "Test",
			},
		}
		if r.shouldIgnoreReconcile(cdp) {
			t.Error("expected false when only unrelated condition is set")
		}
	})

	t.Run("returns true when Created condition among multiple conditions", func(t *testing.T) {
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		cdp.Status.Conditions = []metav1.Condition{
			{
				Type:               "SomeOtherCondition",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "Test",
			},
			NewClusterDataPlaneCreatedCondition(2),
		}
		if !r.shouldIgnoreReconcile(cdp) {
			t.Error("expected true when Created condition is among multiple conditions")
		}
	})
}

// ---------------------------------------------------------------------------
// populateAgentConnectionStatus — nil gateway client guard
// ---------------------------------------------------------------------------

func TestPopulateAgentConnectionStatus_NilGateway(t *testing.T) {
	r := &Reconciler{GatewayClient: nil}
	cdp := &openchoreov1alpha1.ClusterDataPlane{}

	err := r.populateAgentConnectionStatus(context.Background(), cdp)
	if err != nil {
		t.Errorf("expected nil error when GatewayClient is nil, got: %v", err)
	}
	if cdp.Status.AgentConnection != nil {
		t.Error("expected AgentConnection to remain nil when GatewayClient is nil")
	}
}

// ---------------------------------------------------------------------------
// ClusterDataPlaneCleanupFinalizer constant
// ---------------------------------------------------------------------------

func TestClusterDataPlaneCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/clusterdataplane-cleanup"
	if ClusterDataPlaneCleanupFinalizer != want {
		t.Errorf("ClusterDataPlaneCleanupFinalizer: got %q, want %q", ClusterDataPlaneCleanupFinalizer, want)
	}
}

// ---------------------------------------------------------------------------
// ConditionType and ConditionReason constants
// ---------------------------------------------------------------------------

func TestConditionConstants(t *testing.T) {
	t.Run("ConditionCreated value", func(t *testing.T) {
		if string(ConditionCreated) != "Created" {
			t.Errorf("ConditionCreated: got %q, want %q", ConditionCreated, "Created")
		}
	})

	t.Run("ConditionFinalizing value", func(t *testing.T) {
		if string(ConditionFinalizing) != "Finalizing" {
			t.Errorf("ConditionFinalizing: got %q, want %q", ConditionFinalizing, "Finalizing")
		}
	})

	t.Run("ReasonClusterDataPlaneCreated value", func(t *testing.T) {
		if string(ReasonClusterDataPlaneCreated) != "ClusterDataPlaneCreated" {
			t.Errorf("ReasonClusterDataPlaneCreated: got %q, want %q", ReasonClusterDataPlaneCreated, "ClusterDataPlaneCreated")
		}
	})

	t.Run("ReasonClusterDataplaneFinalizing value", func(t *testing.T) {
		if string(ReasonClusterDataplaneFinalizing) != "ClusterDataplaneFinalizing" {
			t.Errorf("ReasonClusterDataplaneFinalizing: got %q, want %q", ReasonClusterDataplaneFinalizing, "ClusterDataplaneFinalizing")
		}
	})
}

// The gateway authorizes a CR on existing agent connections only when it is
// notified, and the controller otherwise notifies once per create or spec
// change. When that grant is lost, the status refresh is the only place left to
// notice it and notify again. Without this the CR stays unusable until the
// agents happen to reconnect.
func TestPopulateAgentConnectionStatus_RenotifiesOnMissingGrant(t *testing.T) {
	tests := []struct {
		name         string
		status       gw.PlaneConnectionStatus
		notifyCode   int
		wantNotifies int
	}{
		{
			name:         "agents on the plane but none authorized for the CR",
			status:       gw.PlaneConnectionStatus{PlaneConnectedAgents: 2},
			notifyCode:   http.StatusOK,
			wantNotifies: 1,
		},
		{
			name:         "CR is authorized",
			status:       gw.PlaneConnectionStatus{Connected: true, ConnectedAgents: 2, PlaneConnectedAgents: 2},
			notifyCode:   http.StatusOK,
			wantNotifies: 0,
		},
		{
			// Also what a gateway that predates the plane count reports.
			name:         "no agent on the plane",
			status:       gw.PlaneConnectionStatus{},
			notifyCode:   http.StatusOK,
			wantNotifies: 0,
		},
		{
			// The refresh must still record the status; the next one retries.
			name:         "re-notification fails",
			status:       gw.PlaneConnectionStatus{PlaneConnectedAgents: 2},
			notifyCode:   http.StatusServiceUnavailable,
			wantNotifies: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gwClient, notifies, shutdown := testgateway.StartFakeGateway(tt.notifyCode, &tt.status)
			defer shutdown()

			r := &Reconciler{GatewayClient: gwClient}
			cdp := &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cdp-1"},
				Spec:       openchoreov1alpha1.ClusterDataPlaneSpec{PlaneID: "plane-1"},
			}
			if err := r.populateAgentConnectionStatus(context.Background(), cdp); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *notifies != tt.wantNotifies {
				t.Errorf("expected %d gateway notifications, got %d", tt.wantNotifies, *notifies)
			}
			if cdp.Status.AgentConnection == nil {
				t.Fatal("expected AgentConnection to be set")
			}
			if cdp.Status.AgentConnection.Connected != tt.status.Connected {
				t.Errorf("expected Connected=%v", tt.status.Connected)
			}
		})
	}
}
