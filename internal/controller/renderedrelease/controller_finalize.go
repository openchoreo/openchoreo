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

	// STEP 1: Set finalizing status condition and return to persist it
	// Mark the Release condition as finalizing and return so that the Release will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseFinalizingCondition(release.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, release); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
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
		logger := log.FromContext(ctx).WithValues("release", release.Name)
		logger.Info("Resource deletion is still pending, retrying...", "remainingResources", len(liveResources))
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
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

	// STEP 1: Set finalizing status condition and return to persist it
	if meta.SetStatusCondition(&release.Status.Conditions, NewRenderedReleaseFinalizingCondition(release.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, release); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
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
		logger := log.FromContext(ctx).WithValues("release", release.Name)
		logger.Info("Resource deletion is still pending, retrying...", "remainingResources", len(liveResources))
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
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
