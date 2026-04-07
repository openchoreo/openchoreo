// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"context"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

// ---------- shouldIgnoreReconcile ----------

func TestShouldIgnoreReconcile_NoConditions(t *testing.T) {
	r := &Reconciler{}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	if r.shouldIgnoreReconcile(cwp) {
		t.Error("expected false when no conditions are set")
	}
}

func TestShouldIgnoreReconcile_WithCreatedCondition(t *testing.T) {
	r := &Reconciler{}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	cwp.Status.Conditions = []metav1.Condition{NewClusterWorkflowPlaneCreatedCondition(1)}
	if !r.shouldIgnoreReconcile(cwp) {
		t.Error("expected true when Created condition is present")
	}
}

func TestShouldIgnoreReconcile_WithUnrelatedCondition(t *testing.T) {
	r := &Reconciler{}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	cwp.Status.Conditions = []metav1.Condition{
		{
			Type:               "SomeOtherCondition",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "Test",
		},
	}
	if r.shouldIgnoreReconcile(cwp) {
		t.Error("expected false when only unrelated condition is set")
	}
}

// ---------- populateAgentConnectionStatus ----------

func TestPopulateAgentConnectionStatus_NilGateway(t *testing.T) {
	r := &Reconciler{GatewayClient: nil}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Errorf("expected nil error when GatewayClient is nil, got: %v", err)
	}
	if cwp.Status.AgentConnection != nil {
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
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-1"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-1"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwp.Status.AgentConnection == nil {
		t.Fatal("expected AgentConnection to be set")
	}
	if !cwp.Status.AgentConnection.Connected {
		t.Errorf("expected Connected=true")
	}
	if cwp.Status.AgentConnection.Message != "1 agent connected" {
		t.Errorf("unexpected message: %q", cwp.Status.AgentConnection.Message)
	}
	if cwp.Status.AgentConnection.LastConnectedTime == nil {
		t.Errorf("expected LastConnectedTime to be set on transition to connected")
	}
}

func TestPopulateAgentConnectionStatus_Connected_HAMode(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 5,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-ha"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-ha"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwp.Status.AgentConnection.Message != "5 agents connected (HA mode)" {
		t.Errorf("unexpected HA message: %q", cwp.Status.AgentConnection.Message)
	}
}

func TestPopulateAgentConnectionStatus_AlreadyConnected_NoTransition(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected:       true,
		ConnectedAgents: 1,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-already"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-already"},
	}
	cwp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{Connected: true}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwp.Status.AgentConnection.LastConnectedTime != nil {
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
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-down"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-down"},
	}
	cwp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{Connected: true}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwp.Status.AgentConnection.Connected {
		t.Errorf("expected Connected=false")
	}
	if cwp.Status.AgentConnection.Message != "No agents connected" {
		t.Errorf("unexpected message: %q", cwp.Status.AgentConnection.Message)
	}
	if cwp.Status.AgentConnection.LastDisconnectedTime == nil {
		t.Errorf("expected LastDisconnectedTime to be set on transition to disconnected")
	}
}

func TestPopulateAgentConnectionStatus_AlreadyDisconnected_NoTransition(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
		Connected: false,
	})
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-still-down"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-still"},
	}
	cwp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{Connected: false}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwp.Status.AgentConnection.LastDisconnectedTime != nil {
		t.Errorf("expected LastDisconnectedTime to remain nil when already disconnected")
	}
}

func TestPopulateAgentConnectionStatus_GatewayError(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusInternalServerError, nil)
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-err"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-err"},
	}
	if err := r.populateAgentConnectionStatus(context.Background(), cwp); err == nil {
		t.Error("expected error when gateway returns 500")
	}
}

// ---------- notifyGateway ----------

func TestNotifyGateway_Success(t *testing.T) {
	gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, nil)
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-notify"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-notify"},
	}
	if err := r.notifyGateway(context.Background(), cwp, "updated"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if *calls != 1 {
		t.Errorf("expected 1 notify call, got %d", *calls)
	}
}

func TestNotifyGateway_Failure(t *testing.T) {
	gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusInternalServerError, nil)
	defer shutdown()

	r := &Reconciler{GatewayClient: gwClient}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-notify-err"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-notify-err"},
	}
	if err := r.notifyGateway(context.Background(), cwp, "updated"); err == nil {
		t.Error("expected error when gateway returns 500")
	}
}

// ---------- invalidateCache ----------

func TestInvalidateCache(t *testing.T) {
	mgr := kubernetesClient.NewManager()
	r := &Reconciler{ClientMgr: mgr, CacheVersion: "v2"}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-cache"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-cache"},
	}
	// Should not panic; covers the RemoveClient call.
	r.invalidateCache(context.Background(), cwp)
}

// ---------- finalize ----------

func TestFinalize_NoFinalizer(t *testing.T) {
	r := &Reconciler{}
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-no-finalizer"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "plane-noop"},
	}
	if controllerutil.ContainsFinalizer(cwp, ClusterWorkflowPlaneCleanupFinalizer) {
		t.Fatal("precondition: cwp must not have the cleanup finalizer")
	}

	result, err := r.finalize(context.Background(), cwp.DeepCopy(), cwp)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("expected empty result, got: %+v", result)
	}
	if controllerutil.ContainsFinalizer(cwp, ClusterWorkflowPlaneCleanupFinalizer) {
		t.Error("finalize must not add the cleanup finalizer on the early-return path")
	}
}
