// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

// mustReconcileDelivery runs a delivery reconcile that is expected to succeed.
func mustReconcileDelivery(t *testing.T, r *Reconciler, ctx context.Context, cl client.Client,
	release *openchoreov1alpha1.RenderedRelease, dc *deliveryContext,
	statuses []openchoreov1alpha1.RenderedManifestStatus) {
	t.Helper()
	if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil); err != nil {
		t.Fatalf("reconcileDeliveryEvents returned %v, want nil", err)
	}
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
		mustReconcileDelivery(t, r, ctx, cl, release, dc, statuses)

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

		mustReconcileDelivery(t, r, ctx, cl, release, dc, statuses)
		mustReconcileDelivery(t, r, ctx, cl, release, dc, statuses)

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
		mustReconcileDelivery(t, r, ctx, cl, release, dc, degraded)
		// Second degraded reconcile must not duplicate the open episode.
		mustReconcileDelivery(t, r, ctx, cl, release, dc, degraded)

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
		mustReconcileDelivery(t, r, ctx, cl, release, dc, healthy)

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
		mustReconcileDelivery(t, r, ctx, cl, release, dc, healthy)

		// New ComponentRelease bound: rollout identity changes.
		release.Labels[labels.LabelKeyComponentReleaseUID] = "cr-uid-8"
		release.Labels[labels.LabelKeyComponentReleaseName] = "checkout-service-8"
		dc2 := deliveryContextFor(release, desired)
		mustReconcileDelivery(t, r, ctx, cl, release, dc2, healthy)

		if release.Status.Delivery.RolloutID != "cr-uid-8.rr-uid-1" {
			t.Errorf("RolloutID = %q, want cr-uid-8.rr-uid-1", release.Status.Delivery.RolloutID)
		}
		events := listDeliveryEvents(t, cl)
		if len(events) != 4 {
			t.Fatalf("expected 4 events (Started+Succeeded per rollout), got %d", len(events))
		}
	})

	// Failure and recovery events are named from a persisted episode counter, not the
	// clock. A wall-clock suffix would give the re-emission a different name, so
	// AlreadyExists could not collapse it and the aggregator would fold the episode twice.
	t.Run("episode names come from the counter, not the clock", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		degraded := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusDegraded),
		}
		healthy := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}

		mustReconcileDelivery(t, r, ctx, cl, release, dc, degraded)
		if got := release.Status.Delivery.FailureEpisode; got != 1 {
			t.Fatalf("FailureEpisode = %d, want 1 for the first episode", got)
		}
		failed := findEventByReason(listDeliveryEvents(t, cl), reasonDeploymentFailed)
		if failed == nil || !strings.HasSuffix(failed.Name, "-e1") {
			t.Fatalf("Failed event name = %q, want an -e1 episode suffix", eventName(failed))
		}

		// The status update is lost, so the next reconcile must derive the same episode
		// number and collapse rather than open a second episode.
		release.Status.Delivery = nil
		mustReconcileDelivery(t, r, ctx, cl, release, dc, degraded)
		if got := countEventsByReason(listDeliveryEvents(t, cl), reasonDeploymentFailed); got != 1 {
			t.Errorf("Failed events = %d, want 1 (re-emission must collapse)", got)
		}
		if got := release.Status.Delivery.FailureEpisode; got != 1 {
			t.Errorf("FailureEpisode = %d, want the counter restored to 1", got)
		}

		// Heal: the recovery closes episode 1 and carries its number.
		mustReconcileDelivery(t, r, ctx, cl, release, dc, healthy)
		recovered := findEventByReason(listDeliveryEvents(t, cl), reasonDeploymentRecovered)
		if recovered == nil || !strings.HasSuffix(recovered.Name, "-e1") {
			t.Fatalf("Recovered event name = %q, want the -e1 suffix of the episode it closes",
				eventName(recovered))
		}

		// A second failure opens episode 2 with its own name.
		mustReconcileDelivery(t, r, ctx, cl, release, dc, degraded)
		if got := release.Status.Delivery.FailureEpisode; got != 2 {
			t.Errorf("FailureEpisode = %d, want 2 for a new episode", got)
		}
		if got := countEventsByReason(listDeliveryEvents(t, cl), reasonDeploymentFailed); got != 2 {
			t.Errorf("Failed events = %d, want 2 across two distinct episodes", got)
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
		mustReconcileDelivery(t, r, ctx, cl, release, dc, statuses)
		release.Status.Delivery = nil
		mustReconcileDelivery(t, r, ctx, cl, release, dc, statuses)

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

// countEventsByReason counts emitted events carrying the given reason.
func countEventsByReason(events []corev1.Event, reason string) int {
	n := 0
	for i := range events {
		if events[i].Reason == reason {
			n++
		}
	}
	return n
}

// eventName renders an event's name for failure messages, tolerating nil.
func eventName(e *corev1.Event) string {
	if e == nil {
		return "<no event>"
	}
	return e.Name
}

// TestReconcileDeliveryEventsStopsOnEmissionFailure pins the phase ordering
// contract: a consumer folds these events chronologically, so a failed emission
// must defer the phases behind it rather than letting a later phase overtake the
// one that could not be written.
func TestReconcileDeliveryEventsStopsOnEmissionFailure(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}

	// failCreateFor rejects the named Event once, letting every other write through.
	failCreateFor := func(name string, failed *bool) interceptor.Funcs {
		return interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object,
				opts ...client.CreateOption) error {
				if event, ok := obj.(*corev1.Event); ok && event.Name == name && !*failed {
					*failed = true
					return apierrors.NewInternalError(errors.New("data plane unavailable"))
				}
				return cl.Create(ctx, obj, opts...)
			},
		}
	}

	t.Run("failed Started defers Succeeded to the retry", func(t *testing.T) {
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		startedName := deliveryEventName(dc, reasonDeploymentStarted, "")

		failed := false
		cl := fake.NewClientBuilder().WithInterceptorFuncs(failCreateFor(startedName, &failed)).Build()

		statuses := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}

		err := r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil)
		if err == nil {
			t.Fatal("expected the Started emission failure to be returned so the reconcile requeues")
		}

		if events := listDeliveryEvents(t, cl); len(events) != 0 {
			t.Fatalf("expected no events after a failed Started, got %d (%s)",
				len(events), events[0].Reason)
		}
		if d := release.Status.Delivery; d.StartedAt != nil || d.SucceededAt != nil {
			t.Errorf("no markers should be set after a failed Started: startedAt=%v succeededAt=%v",
				d.StartedAt, d.SucceededAt)
		}

		// The retry emits both phases, in order.
		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil); err != nil {
			t.Fatalf("retry returned %v, want nil", err)
		}
		events := listDeliveryEvents(t, cl)
		if len(events) != 2 {
			t.Fatalf("expected Started and Succeeded after the retry, got %d", len(events))
		}
		if findEventByReason(events, reasonDeploymentStarted) == nil {
			t.Error("expected DeploymentStarted after the retry")
		}
		if findEventByReason(events, reasonDeploymentSucceeded) == nil {
			t.Error("expected DeploymentSucceeded after the retry")
		}
	})

	t.Run("failed Succeeded leaves Started recorded and does not mark success", func(t *testing.T) {
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		succeededName := deliveryEventName(dc, reasonDeploymentSucceeded, "")

		failed := false
		cl := fake.NewClientBuilder().WithInterceptorFuncs(failCreateFor(succeededName, &failed)).Build()

		statuses := []openchoreov1alpha1.RenderedManifestStatus{
			manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
		}

		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, statuses, nil); err == nil {
			t.Fatal("expected the Succeeded emission failure to be returned")
		}
		if d := release.Status.Delivery; d.StartedAt == nil {
			t.Error("Started succeeded, so its marker must be kept for the retry")
		} else if d.SucceededAt != nil {
			t.Error("SucceededAt must not be set when the event was not written")
		}
	})
}

// TestRestoreLostFailureEpisode covers the ordering hazard where DeploymentFailed
// reaches the data plane but the status update carrying its marker does not. The
// episode is genuinely open, but nothing in status says so, and a release that
// returns to healthy would otherwise never emit DeploymentRecovered -- leaving the
// failure open forever with no recovery to measure against it.
func TestRestoreLostFailureEpisode(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}

	degraded := []openchoreov1alpha1.RenderedManifestStatus{
		manifestStatus("deployment", openchoreov1alpha1.HealthStatusDegraded),
	}
	healthy := []openchoreov1alpha1.RenderedManifestStatus{
		manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
	}

	t.Run("recovers an episode whose marker was lost before the healthy transition", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, degraded, nil); err != nil {
			t.Fatalf("degraded reconcile: %v", err)
		}
		if release.Status.Delivery.FailedAt == nil {
			t.Fatal("expected a DeploymentFailed marker after the degraded reconcile")
		}
		failedEvent := findEventByReason(listDeliveryEvents(t, cl), reasonDeploymentFailed)
		if failedEvent == nil {
			t.Fatal("expected a DeploymentFailed event")
		}

		// The status write carrying FailedAt/FailureEpisode is lost; the Event
		// itself already reached the data plane and survives.
		release.Status.Delivery.FailedAt = nil
		release.Status.Delivery.FailureEpisode = 0

		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, healthy, nil); err != nil {
			t.Fatalf("healthy reconcile: %v", err)
		}

		events := listDeliveryEvents(t, cl)
		if findEventByReason(events, reasonDeploymentRecovered) == nil {
			t.Fatal("expected DeploymentRecovered after the lost marker was restored")
		}
		if findEventByReason(events, reasonDeploymentSucceeded) == nil {
			t.Error("expected DeploymentSucceeded on the healthy transition")
		}

		d := release.Status.Delivery
		if d.FailureEpisode != 1 {
			t.Errorf("failureEpisode = %d, want the restored episode 1", d.FailureEpisode)
		}
		if d.RecoveredAt == nil {
			t.Error("expected RecoveredAt to be set")
		}
		// The restored marker must carry the original failure time, or the recovery
		// duration a consumer derives starts at the repair instead of the outage.
		if d.FailedAt == nil {
			t.Fatal("expected FailedAt to be restored")
		}
		if !d.FailedAt.Time.Equal(failedEvent.FirstTimestamp.Time) {
			t.Errorf("restored failedAt = %v, want the event's timestamp %v",
				d.FailedAt.Time, failedEvent.FirstTimestamp.Time)
		}
	})

	t.Run("recovered event closes the episode exactly once", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, degraded, nil); err != nil {
			t.Fatalf("degraded reconcile: %v", err)
		}
		release.Status.Delivery.FailedAt = nil
		release.Status.Delivery.FailureEpisode = 0

		for i := range 3 {
			if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, healthy, nil); err != nil {
				t.Fatalf("healthy reconcile %d: %v", i, err)
			}
		}

		var recovered int
		for _, e := range listDeliveryEvents(t, cl) {
			if e.Reason == reasonDeploymentRecovered {
				recovered++
			}
		}
		if recovered != 1 {
			t.Errorf("emitted %d DeploymentRecovered events, want exactly 1", recovered)
		}
	})

	t.Run("healthy rollout with no failure history emits no recovery", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)

		if err := r.reconcileDeliveryEvents(ctx, cl, release, dc, healthy, nil); err != nil {
			t.Fatalf("healthy reconcile: %v", err)
		}

		events := listDeliveryEvents(t, cl)
		if findEventByReason(events, reasonDeploymentRecovered) != nil {
			t.Error("a rollout that never failed must not emit DeploymentRecovered")
		}
		if d := release.Status.Delivery; d.FailedAt != nil || d.FailureEpisode != 0 {
			t.Errorf("no failure markers expected: failedAt=%v episode=%d", d.FailedAt, d.FailureEpisode)
		}
	})
}

// TestEmitDeliveryEventsWrapsError pins the context on the error that reaches the
// reconcile boundary. controller-runtime reports the request, not which rollout
// was being emitted for, so the rollout identity has to travel on the error.
func TestEmitDeliveryEventsWrapsError(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deployment := makeDeliveryDeployment()
	desired := []*unstructured.Unstructured{deployment}
	statuses := []openchoreov1alpha1.RenderedManifestStatus{
		manifestStatus("deployment", openchoreov1alpha1.HealthStatusHealthy),
	}

	t.Run("no delivery context is a no-op", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		release := makeDeliveryRelease()
		if err := r.emitDeliveryEvents(ctx, cl, release, nil, statuses, nil); err != nil {
			t.Fatalf("emitDeliveryEvents with no context returned %v, want nil", err)
		}
		if events := listDeliveryEvents(t, cl); len(events) != 0 {
			t.Errorf("expected no events, got %d", len(events))
		}
	})

	t.Run("emission failure is wrapped with the rollout and still unwraps", func(t *testing.T) {
		release := makeDeliveryRelease()
		dc := deliveryContextFor(release, desired)
		inner := apierrors.NewInternalError(errors.New("data plane unavailable"))

		cl := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object,
				opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.Event); ok {
					return inner
				}
				return cl.Create(ctx, obj, opts...)
			},
		}).Build()

		err := r.emitDeliveryEvents(ctx, cl, release, dc, statuses, nil)
		if err == nil {
			t.Fatal("expected an error from a failed emission")
		}
		if !strings.Contains(err.Error(), dc.rolloutID) {
			t.Errorf("error %q does not name the rollout %q", err, dc.rolloutID)
		}
		if !errors.Is(err, inner) {
			t.Errorf("wrapped error must still unwrap to the data plane failure, got %v", err)
		}
	})
}
