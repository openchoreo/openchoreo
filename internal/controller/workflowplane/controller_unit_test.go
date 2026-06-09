// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
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

func TestPopulateAgentConnectionStatus_Connected_SingleAgent(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 1,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-1", Namespace: "default"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wp.Status.AgentConnection == nil {
		t.Fatal("expected AgentConnection to be set")
	}
	if !wp.Status.AgentConnection.Connected {
		t.Errorf("expected Connected=true")
	}
	if wp.Status.AgentConnection.ConnectedAgents != 1 {
		t.Errorf("expected ConnectedAgents=1, got %d", wp.Status.AgentConnection.ConnectedAgents)
	}
	if wp.Status.AgentConnection.Message != "1 agent connected" {
		t.Errorf("unexpected message: %q", wp.Status.AgentConnection.Message)
	}
	if wp.Status.AgentConnection.LastConnectedTime == nil {
		t.Errorf("expected LastConnectedTime to be set on transition to connected")
	}
}

func TestPopulateAgentConnectionStatus_Connected_HAMode(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 3,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-ha", Namespace: "default"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wp.Status.AgentConnection.Message != "3 agents connected (HA mode)" {
		t.Errorf("unexpected HA message: %q", wp.Status.AgentConnection.Message)
	}
}

func TestPopulateAgentConnectionStatus_AlreadyConnected_NoTransition(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 1,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-already", Namespace: "default"},
	}
	wp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
		Connected: true,
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// LastConnectedTime should NOT be set since previouslyConnected was true
	if wp.Status.AgentConnection.LastConnectedTime != nil {
		t.Errorf("expected LastConnectedTime to remain nil when already connected")
	}
}

func TestPopulateAgentConnectionStatus_DisconnectedTransition(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       false,
		ConnectedAgents: 0,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-down", Namespace: "default"},
	}
	wp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
		Connected: true,
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wp.Status.AgentConnection.Connected {
		t.Errorf("expected Connected=false")
	}
	if wp.Status.AgentConnection.Message != "No agents connected" {
		t.Errorf("unexpected message: %q", wp.Status.AgentConnection.Message)
	}
	if wp.Status.AgentConnection.LastDisconnectedTime == nil {
		t.Errorf("expected LastDisconnectedTime to be set on transition to disconnected")
	}
}

func TestPopulateAgentConnectionStatus_AlreadyDisconnected_NoTransition(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected: false,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-still-down", Namespace: "default"},
	}
	wp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
		Connected: false,
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wp.Status.AgentConnection.LastDisconnectedTime != nil {
		t.Errorf("expected LastDisconnectedTime to remain nil when already disconnected")
	}
}

func TestPopulateAgentConnectionStatus_PlaneIDOverride(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 1,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-pid", Namespace: "default"},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			PlaneID: "custom-plane",
		},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wp.Status.AgentConnection.Connected {
		t.Errorf("expected Connected=true with PlaneID override")
	}
}

func TestPopulateAgentConnectionStatus_GatewayError(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusInternalServerError, nil)
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-err", Namespace: "default"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), wp); err == nil {
		t.Error("expected error when gateway returns 500")
	}
}

func TestInvalidateCache_WithExplicitPlaneID(t *testing.T) {
	mgr := kubernetesClient.NewManager()
	r := &Reconciler{ClientMgr: mgr, CacheVersion: "v2"}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-invalidate", Namespace: "default"},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			PlaneID: "explicit-plane",
		},
	}
	// Should not panic and should call RemoveClient on both primary and fallback keys.
	r.invalidateCache(context.Background(), wp)
}

func TestInvalidateCache_DefaultsToName(t *testing.T) {
	mgr := kubernetesClient.NewManager()
	r := &Reconciler{ClientMgr: mgr, CacheVersion: "v2"}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-invalidate-default", Namespace: "default"},
	}
	// effectivePlaneID == workflowPlane.Name → no fallback key path executed.
	r.invalidateCache(context.Background(), wp)
}

func TestWorkflowPlaneCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/workflowplane-cleanup"
	if WorkflowPlaneCleanupFinalizer != want {
		t.Errorf("WorkflowPlaneCleanupFinalizer: got %q, want %q", WorkflowPlaneCleanupFinalizer, want)
	}
}
