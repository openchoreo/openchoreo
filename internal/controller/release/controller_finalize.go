// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// DataPlaneCleanupFinalizer is the finalizer that is used to clean up the data plane resources.
	DataPlaneCleanupFinalizer = "core.choreo.dev/dataplane-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the Release.
// The first return value indicates whether the finalizer was added to the Release.
func (r *Reconciler) ensureFinalizer(ctx context.Context, release *choreov1.Release) (bool, error) {
	// If the Release is being deleted, no need to add the finalizer
	if !release.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(release, DataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, release)
	}

	return false, nil
}

// finalize cleans up the data plane resources associated with the Release.
func (r *Reconciler) finalize(ctx context.Context, old, release *choreov1.Release) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(release, DataPlaneCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// STEP 1: Set finalizing status condition and return to persist it
	// Mark the Release condition as finalizing and return so that the Release will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&release.Status.Conditions, NewReleaseFinalizingCondition(release.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, release); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// STEP 2: Get dataplane client and find all managed resources
	dpClient, err := r.getDPClient(ctx, release.Spec.EnvironmentName)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to get dataplane client for finalization: %w", err)
	}

	// STEP 3: List all live resources we manage (use empty desired resources since we want to delete everything)
	var emptyDesiredResources []*unstructured.Unstructured
	gvks := findAllKnownGVKs(emptyDesiredResources, release.Status.Resources)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, dpClient, release, gvks)
	if err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewReleaseCleanupFailedCondition(release.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, release); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to list live resources for cleanup: %w", err)
	}

	// STEP 4: Delete all live resources (since we want to delete everything, all live resources are "stale")
	if err := r.deleteResources(ctx, dpClient, liveResources); err != nil {
		meta.SetStatusCondition(&release.Status.Conditions, NewReleaseCleanupFailedCondition(release.Generation, err))
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
