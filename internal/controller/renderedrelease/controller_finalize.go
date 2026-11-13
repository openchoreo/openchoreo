// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// DataPlaneCleanupFinalizer is the finalizer that is used to clean up the data plane resources.
	DataPlaneCleanupFinalizer = "openchoreo.dev/dataplane-cleanup"
	// ObsPlaneCleanupFinalizer is the finalizer that is used to clean up the observability plane resources.
	ObsPlaneCleanupFinalizer = "openchoreo.dev/obsplane-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the Release.
// The first return value indicates whether the finalizer was added to the Release.
func (r *Reconciler) ensureFinalizer(ctx context.Context, release *openchoreov1alpha1.RenderedRelease) (bool, error) {
	// If the Release is being deleted, no need to add the finalizer
	if !release.DeletionTimestamp.IsZero() {
		return false, nil
	}

	finalizer := DataPlaneCleanupFinalizer
	if release.Spec.TargetPlane == targetPlaneObservabilityPlane {
		finalizer = ObsPlaneCleanupFinalizer
	}

	if controllerutil.AddFinalizer(release, finalizer) {
		return true, r.Update(ctx, release)
	}

	return false, nil
}

// finalizeRetryInterval is how long finalization waits before its next pass: after
// marking the release as finalizing, and while deleted resources are still
// disappearing from the plane.
const finalizeRetryInterval = 5 * time.Second

// markFinalizing persists the Finalizing condition the first time finalization
// runs, and reports whether it wrote it. After a write the caller returns and
// comes back after finalizeRetryInterval. The watch drops the controller's own
// status writes, so the write does not trigger the next pass; and continuing in
// this pass is not safe, because UpdateStatusConditions writes a copy built from
// old, which leaves release with a stale resourceVersion for the finalizer
// removal at the end.
//
// The condition is only added when absent. The cleanup failure paths reuse the
// Finalizing type with reason CleanupFailed; resetting it here on every pass
// flipped it between CleanupFailed and CleanupInProgress and wrote status twice
// per retry.
func (r *Reconciler) markFinalizing(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease) (bool, error) {
	if meta.FindStatusCondition(release.Status.Conditions, string(ConditionFinalizing)) != nil {
		return false, nil
	}
	meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseFinalizingCondition(release.Generation))
	return true, controller.UpdateStatusConditions(ctx, r.Client, old, release)
}

// cleanupPending requeues while deleted resources are still present on the plane.
// This pass got through its deletes, so a CleanupFailed reason left by an earlier
// pass no longer holds; it goes back to CleanupInProgress, since markFinalizing
// leaves an existing condition alone.
func (r *Reconciler) cleanupPending(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease, remaining int) (ctrl.Result, error) {
	log.FromContext(ctx).WithValues("release", release.Name).
		Info("Resource deletion is still pending, retrying...", "remainingResources", remaining)
	if meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseFinalizingCondition(release.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, release); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: finalizeRetryInterval}, nil
}

// finalize dispatches to the appropriate plane-specific cleanup based on targetPlane.
func (r *Reconciler) finalize(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease) (ctrl.Result, error) {
	if release.Spec.TargetPlane == targetPlaneObservabilityPlane {
		return r.finalizeObsPlane(ctx, old, release)
	}
	return r.finalizeDataPlane(ctx, old, release)
}

// finalizeDataPlane cleans up the data plane resources associated with the Release.
func (r *Reconciler) finalizeDataPlane(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(release, DataPlaneCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// STEP 1: Mark the release as finalizing and persist that before any cleanup
	marked, err := r.markFinalizing(ctx, old, release)
	if err != nil {
		return ctrl.Result{}, err
	}
	if marked {
		return ctrl.Result{RequeueAfter: finalizeRetryInterval}, nil
	}

	// STEP 2: Get data plane client
	planeClient, err := r.getDPClient(ctx, release.Namespace, release.Spec.EnvironmentName)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to get dataplane client for finalization: %w", err)
	}

	// STEP 3: List all live resources we manage (use empty desired resources since we want to delete everything)
	var emptyDesiredResources []*unstructured.Unstructured
	gvks := findAllKnownGVKs(emptyDesiredResources, release.Status.Resources, targetPlaneDataPlane)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, planeClient, release, gvks)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to list live resources for cleanup: %w", err)
	}
	// Only what this controller applied is deleted, and only that is waited for below:
	// an object that merely carries our labels (e.g. the Secret ESO creates for an
	// ExternalSecret) belongs to its creator and may legitimately outlive the release.
	// Observed objects are never deleted, even one this release applied earlier.
	liveResources = excludeObserved(appliedByController(liveResources), resolveObservedResources(release.Spec.ObservedResources))

	// STEP 4: Delete all live resources (since we want to delete everything, all live resources are "stale")
	if err := r.deleteResources(ctx, planeClient, liveResources); err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to delete resources during finalization: %w", err)
	}

	// STEP 5: Check if any resources still exist - if so, requeue for retry
	if len(liveResources) > 0 {
		return r.cleanupPending(ctx, old, release, len(liveResources))
	}

	// STEP 6: All resources cleaned up - remove the finalizer
	if controllerutil.RemoveFinalizer(release, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, release); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// finalizeObsPlane cleans up the observability plane resources associated with the release.
func (r *Reconciler) finalizeObsPlane(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease) (ctrl.Result, error) {
	// For backward compatibility, obsplane releases created before this change may still carry
	// DataPlaneCleanupFinalizer — fall back to it so those releases are not permanently stuck.
	activeFinalizer := ObsPlaneCleanupFinalizer
	if !controllerutil.ContainsFinalizer(release, ObsPlaneCleanupFinalizer) {
		if !controllerutil.ContainsFinalizer(release, DataPlaneCleanupFinalizer) {
			return ctrl.Result{}, nil
		}
		activeFinalizer = DataPlaneCleanupFinalizer
	}

	// STEP 1: Mark the release as finalizing and persist that before any cleanup
	marked, err := r.markFinalizing(ctx, old, release)
	if err != nil {
		return ctrl.Result{}, err
	}
	if marked {
		return ctrl.Result{RequeueAfter: finalizeRetryInterval}, nil
	}

	// STEP 2: Get observability plane client
	planeClient, err := r.getOPClient(ctx, release.Namespace, release.Spec.EnvironmentName)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to get observability plane client for finalization: %w", err)
	}

	// STEP 3: List the obs-plane resource types managed by this release
	releaseResourceGVKs := r.intersectObsPlaneGVKs(ctx, release)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, planeClient, release, releaseResourceGVKs)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to list live resources for cleanup: %w", err)
	}
	// As on the data plane: delete, and wait for, only what this controller applied,
	// and never an observed object.
	liveResources = excludeObserved(appliedByController(liveResources), resolveObservedResources(release.Spec.ObservedResources))

	// STEP 4: Delete all live resources
	if err := r.deleteResources(ctx, planeClient, liveResources); err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to delete resources during finalization: %w", err)
	}

	// STEP 5: Check if any resources still exist - if so, requeue for retry
	if len(liveResources) > 0 {
		return r.cleanupPending(ctx, old, release, len(liveResources))
	}

	// STEP 6: All resources cleaned up - remove the finalizer
	if controllerutil.RemoveFinalizer(release, activeFinalizer) {
		if err := r.Update(ctx, release); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// intersectObsPlaneGVKs returns the subset of GVKs from the release status that are in
// wellKnownObservabilityPlaneGVKs
func (r *Reconciler) intersectObsPlaneGVKs(ctx context.Context, release *openchoreov1alpha1.RenderedRelease) []schema.GroupVersionKind {
	seen := make(map[schema.GroupVersionKind]bool)
	var result []schema.GroupVersionKind
	logger := log.FromContext(ctx).WithValues("release", release.Name)

	for _, rs := range release.Status.Resources {
		gvk := schema.GroupVersionKind{Group: rs.Group, Version: rs.Version, Kind: rs.Kind}
		if seen[gvk] {
			continue
		}
		seen[gvk] = true
		if slices.Contains(wellKnownObservabilityPlaneGVKs, gvk) {
			result = append(result, gvk)
		} else {
			logger.Error(fmt.Errorf("GVK not in observability plane allowlist"),
				"resource will not be cleaned up during obs-plane finalization",
				"group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
		}
	}
	return result
}
