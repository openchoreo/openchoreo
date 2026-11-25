// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"net/http"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
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
			op := &openchoreov1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "op-1", Namespace: "default"},
			}
			if err := r.populateAgentConnectionStatus(context.Background(), op); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *notifies != tt.wantNotifies {
				t.Errorf("expected %d gateway notifications, got %d", tt.wantNotifies, *notifies)
			}
			if op.Status.AgentConnection == nil {
				t.Fatal("expected AgentConnection to be set")
			}
			if op.Status.AgentConnection.Connected != tt.status.Connected {
				t.Errorf("expected Connected=%v", tt.status.Connected)
			}
		})
	}
}

// A status request can land on a gateway replica that does not own the agent's
// connection and reports it connected without a LastSeen. That reading must
// not clear the heartbeat an earlier one recorded.
func TestPopulateAgentConnectionStatus_LastHeartbeatTime(t *testing.T) {
	recorded := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	newer := recorded.Add(30 * time.Second)

	tests := []struct {
		name     string
		lastSeen time.Time
		want     time.Time
	}{
		{name: "no LastSeen keeps the recorded heartbeat", want: recorded},
		{name: "newer LastSeen advances the heartbeat", lastSeen: newer, want: newer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
				Connected:       true,
				ConnectedAgents: 1,
				LastSeen:        tt.lastSeen,
			})
			defer shutdown()

			r := &Reconciler{GatewayClient: gwClient}
			plane := &openchoreov1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "plane-1", Namespace: "default"},
			}
			plane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:         true,
				LastHeartbeatTime: &metav1.Time{Time: recorded},
			}
			if err := r.populateAgentConnectionStatus(context.Background(), plane); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := plane.Status.AgentConnection.LastHeartbeatTime
			if got == nil || !got.Time.Equal(tt.want) {
				t.Errorf("LastHeartbeatTime = %v, want %v", got, tt.want)
			}
		})
	}
}
