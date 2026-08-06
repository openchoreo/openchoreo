// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	testComponentReleaseName = "checkout-service-7"
	testComponentReleaseUID  = "cr-uid-7"
)

func makeDeliveryRelease() *openchoreov1alpha1.RenderedRelease {
	return &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "checkout-service-dev",
			Namespace: "acme",
			UID:       types.UID("rr-uid-1"),
			Labels: map[string]string{
				labels.LabelKeyComponentReleaseName: testComponentReleaseName,
				labels.LabelKeyComponentReleaseUID:  testComponentReleaseUID,
			},
		},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   "shop",
				ComponentName: "checkout-service",
			},
			EnvironmentName: "dev",
			TargetPlane:     targetPlaneDataPlane,
		},
	}
}

func makeDeliveryDeployment() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetName("checkout-service-dev-deployment")
	obj.SetNamespace("dp-acme-shop-dev-1234")
	obj.SetLabels(map[string]string{
		labels.LabelKeyRenderedReleaseResourceID: "deployment",
		labels.LabelKeyProjectUID:                "project-uid-1",
		labels.LabelKeyComponentUID:              "component-uid-1",
		labels.LabelKeyEnvironmentUID:            "environment-uid-1",
	})
	return obj
}

func manifestStatus(id string, health openchoreov1alpha1.HealthStatus) openchoreov1alpha1.RenderedManifestStatus {
	return openchoreov1alpha1.RenderedManifestStatus{ID: id, HealthStatus: health}
}

func listDeliveryEvents(t *testing.T, cl client.Client) []corev1.Event {
	t.Helper()
	list := &corev1.EventList{}
	if err := cl.List(context.Background(), list); err != nil {
		t.Fatalf("list events: %v", err)
	}
	return list.Items
}

func findEventByReason(events []corev1.Event, reason string) *corev1.Event {
	for i := range events {
		if events[i].Reason == reason {
			return &events[i]
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// deliveryContextFor
// ─────────────────────────────────────────────────────────────

func TestDeliveryContextFor(t *testing.T) {
	deployment := makeDeliveryDeployment()

	t.Run("resolves context for component data-plane release", func(t *testing.T) {
		dc := deliveryContextFor(makeDeliveryRelease(), []*unstructured.Unstructured{deployment})
		if dc == nil {
			t.Fatal("expected delivery context, got nil")
		}
		wantRollout := testComponentReleaseUID + ".rr-uid-1"
		if dc.rolloutID != wantRollout {
			t.Errorf("rolloutID = %q, want %q", dc.rolloutID, wantRollout)
		}
		if dc.componentReleaseName != testComponentReleaseName {
			t.Errorf("componentReleaseName = %q, want %q", dc.componentReleaseName, testComponentReleaseName)
		}
		if dc.primary != deployment {
			t.Error("expected primary to be the deployment")
		}
	})

	t.Run("nil for observability plane releases", func(t *testing.T) {
		release := makeDeliveryRelease()
		release.Spec.TargetPlane = targetPlaneObservabilityPlane
		if dc := deliveryContextFor(release, []*unstructured.Unstructured{deployment}); dc != nil {
			t.Error("expected nil context for observability plane")
		}
	})

	t.Run("nil for non-component owners", func(t *testing.T) {
		release := makeDeliveryRelease()
		release.Spec.Owner.ComponentName = ""
		if dc := deliveryContextFor(release, []*unstructured.Unstructured{deployment}); dc != nil {
			t.Error("expected nil context for project-level release")
		}
	})

	t.Run("nil when ComponentRelease labels are not stamped", func(t *testing.T) {
		release := makeDeliveryRelease()
		release.Labels = nil
		if dc := deliveryContextFor(release, []*unstructured.Unstructured{deployment}); dc != nil {
			t.Error("expected nil context without ComponentRelease labels")
		}
	})

	t.Run("nil without a primary workload resource", func(t *testing.T) {
		configMap := &unstructured.Unstructured{}
		configMap.SetAPIVersion("v1")
		configMap.SetKind("ConfigMap")
		if dc := deliveryContextFor(makeDeliveryRelease(), []*unstructured.Unstructured{configMap}); dc != nil {
			t.Error("expected nil context without a workload resource")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// summarizeHealth
// ─────────────────────────────────────────────────────────────

func TestSummarizeHealth(t *testing.T) {
	t.Run("empty statuses are not healthy", func(t *testing.T) {
		allHealthy, degradedID := summarizeHealth(nil)
		if allHealthy || degradedID != "" {
			t.Errorf("got allHealthy=%v degradedID=%q, want false and empty", allHealthy, degradedID)
		}
	})

	t.Run("healthy and suspended count as settled", func(t *testing.T) {
		allHealthy, degradedID := summarizeHealth([]openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("a", openchoreov1alpha1.HealthStatusHealthy),
			manifestStatus("b", openchoreov1alpha1.HealthStatusSuspended),
		})
		if !allHealthy || degradedID != "" {
			t.Errorf("got allHealthy=%v degradedID=%q, want true and empty", allHealthy, degradedID)
		}
	})

	t.Run("progressing blocks healthy without degrading", func(t *testing.T) {
		allHealthy, degradedID := summarizeHealth([]openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("a", openchoreov1alpha1.HealthStatusHealthy),
			manifestStatus("b", openchoreov1alpha1.HealthStatusProgressing),
		})
		if allHealthy || degradedID != "" {
			t.Errorf("got allHealthy=%v degradedID=%q, want false and empty", allHealthy, degradedID)
		}
	})

	t.Run("degraded resource is reported", func(t *testing.T) {
		allHealthy, degradedID := summarizeHealth([]openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("a", openchoreov1alpha1.HealthStatusHealthy),
			manifestStatus("b", openchoreov1alpha1.HealthStatusDegraded),
		})
		if allHealthy || degradedID != "b" {
			t.Errorf("got allHealthy=%v degradedID=%q, want false and b", allHealthy, degradedID)
		}
	})
}

// ─────────────────────────────────────────────────────────────
// reconcileDeliveryEvents
// ─────────────────────────────────────────────────────────────

func TestReconcileDeliveryEvents(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}

	t.Run("progressing rollout emits Started only", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		statuses := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusProgressing),
		}
		r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)

		events := listDeliveryEvents(t, cl)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		started := findEventByReason(events, reasonDeploymentStarted)
		if started == nil {
			t.Fatal("expected DeploymentStarted event")
		}
		if started.Type != corev1.EventTypeNormal {
			t.Errorf("Started type = %q, want Normal", started.Type)
		}
		if started.Namespace != deployment.GetNamespace() {
			t.Errorf("event namespace = %q, want %q", started.Namespace, deployment.GetNamespace())
		}
		if started.InvolvedObject.Name != deployment.GetName() {
			t.Errorf("involvedObject = %q, want %q", started.InvolvedObject.Name, deployment.GetName())
		}

		var payload deliveryEventPayload
		if err := json.Unmarshal([]byte(started.Message), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.RenderedReleaseUID != dc.rolloutID {
			t.Errorf("payload renderedReleaseUid = %q, want %q", payload.RenderedReleaseUID, dc.rolloutID)
		}
		if payload.ComponentReleaseName != testComponentReleaseName {
			t.Errorf("payload componentReleaseName = %q, want %q", payload.ComponentReleaseName, testComponentReleaseName)
		}
		if payload.ProjectUID != "project-uid-1" || payload.ComponentUID != "component-uid-1" ||
			payload.EnvironmentUID != "environment-uid-1" {
			t.Errorf("payload scope UIDs = %q/%q/%q, want stamped label values",
				payload.ProjectUID, payload.ComponentUID, payload.EnvironmentUID)
		}
		if payload.Phase != "Started" {
			t.Errorf("payload phase = %q, want Started", payload.Phase)
		}

		if release.Status.Delivery == nil || release.Status.Delivery.StartedAt == nil {
			t.Error("expected StartedAt marker to be set")
		}
		if release.Status.Delivery.SucceededAt != nil {
			t.Error("SucceededAt must not be set while progressing")
		}
	})

	t.Run("healthy rollout emits Succeeded once", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		statuses := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}

		r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)
		r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)

		events := listDeliveryEvents(t, cl)
		if len(events) != 2 {
			t.Fatalf("expected Started+Succeeded, got %d events", len(events))
		}
		if findEventByReason(events, reasonDeploymentSucceeded) == nil {
			t.Fatal("expected DeploymentSucceeded event")
		}
		if release.Status.Delivery.SucceededAt == nil {
			t.Error("expected SucceededAt marker to be set")
		}
	})
}

func TestReconcileDeliveryEventsEpisodes(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}

	t.Run("degraded rollout emits Failed then Recovered on heal", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		degraded := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusDegraded),
		}
		r.reconcileDeliveryEvents(ctx, cl, release, dc, degraded, nil)
		// Second degraded reconcile must not duplicate the open episode.
		r.reconcileDeliveryEvents(ctx, cl, release, dc, degraded, nil)

		events := listDeliveryEvents(t, cl)
		failed := findEventByReason(events, reasonDeploymentFailed)
		if failed == nil {
			t.Fatal("expected DeploymentFailed event")
		}
		if failed.Type != corev1.EventTypeWarning {
			t.Errorf("Failed type = %q, want Warning", failed.Type)
		}
		var payload deliveryEventPayload
		if err := json.Unmarshal([]byte(failed.Message), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.FailureReason != "Degraded" {
			t.Errorf("failureReason = %q, want Degraded (no live resource to inspect)", payload.FailureReason)
		}
		failedCount := 0
		for _, e := range events {
			if e.Reason == reasonDeploymentFailed {
				failedCount++
			}
		}
		if failedCount != 1 {
			t.Errorf("expected exactly 1 Failed event for an open episode, got %d", failedCount)
		}

		healthy := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}
		r.reconcileDeliveryEvents(ctx, cl, release, dc, healthy, nil)

		events = listDeliveryEvents(t, cl)
		if findEventByReason(events, reasonDeploymentRecovered) == nil {
			t.Fatal("expected DeploymentRecovered event after heal")
		}
		if findEventByReason(events, reasonDeploymentSucceeded) == nil {
			t.Fatal("expected DeploymentSucceeded event after heal")
		}
		if !release.Status.Delivery.RecoveredAt.After(release.Status.Delivery.FailedAt.Time) &&
			!release.Status.Delivery.RecoveredAt.Equal(release.Status.Delivery.FailedAt) {
			t.Error("RecoveredAt must not be before FailedAt")
		}
	})

	t.Run("new rollout resets markers and emits again", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		healthy := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}
		r.reconcileDeliveryEvents(ctx, cl, release, dc, healthy, nil)

		// New ComponentRelease bound: rollout identity changes.
		release.Labels[labels.LabelKeyComponentReleaseUID] = "cr-uid-8"
		release.Labels[labels.LabelKeyComponentReleaseName] = "checkout-service-8"
		dc2 := deliveryContextFor(release, desired)
		r.reconcileDeliveryEvents(ctx, cl, release, dc2, healthy, nil)

		if release.Status.Delivery.RolloutID != "cr-uid-8.rr-uid-1" {
			t.Errorf("RolloutID = %q, want cr-uid-8.rr-uid-1", release.Status.Delivery.RolloutID)
		}
		events := listDeliveryEvents(t, cl)
		if len(events) != 4 {
			t.Fatalf("expected 4 events (Started+Succeeded per rollout), got %d", len(events))
		}
	})

	t.Run("pre-existing event is treated as emitted", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		statuses := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusProgressing),
		}

		// Emit once, then wipe the marker as if the status update was lost.
		r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)
		release.Status.Delivery = nil
		r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)

		events := listDeliveryEvents(t, cl)
		if len(events) != 1 {
			t.Fatalf("expected AlreadyExists to collapse duplicate Started, got %d events", len(events))
		}
		if release.Status.Delivery == nil || release.Status.Delivery.StartedAt == nil {
			t.Error("expected StartedAt marker to be restored")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// markDeliveryApplyFailure
// ─────────────────────────────────────────────────────────────

func TestMarkDeliveryApplyFailure(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}

	t.Run("emits Failed with ApplyFailed reason once per episode", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		if changed := r.markDeliveryApplyFailure(ctx, cl, release, dc); !changed {
			t.Error("expected first apply failure to change delivery status")
		}
		if changed := r.markDeliveryApplyFailure(ctx, cl, release, dc); changed {
			t.Error("expected repeated apply failure to be a no-op")
		}

		events := listDeliveryEvents(t, cl)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		var payload deliveryEventPayload
		if err := json.Unmarshal([]byte(events[0].Message), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.FailureReason != failureReasonApplyFailed {
			t.Errorf("failureReason = %q, want %q", payload.FailureReason, failureReasonApplyFailed)
		}
	})
}
