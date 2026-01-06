// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// DataPlaneCleanupFinalizer is the finalizer that is used to clean up the data plane resources.
	DataPlaneCleanupFinalizer = "openchoreo.dev/dataplane-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the RenderedRelease.
// The first return value indicates whether the finalizer was added to the RenderedRelease.
func (r *Reconciler) ensureFinalizer(ctx context.Context, renderedRelease *openchoreov1alpha1.RenderedRelease) (bool, error) {
	// If the RenderedRelease is being deleted, no need to add the finalizer
	if !renderedRelease.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(renderedRelease, DataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, renderedRelease)
	}

	return false, nil
}

// finalize cleans up the target plane (dataplane or observabilityplane) resources associated with the RenderedRelease.
func (r *Reconciler) finalize(ctx context.Context, old, renderedRelease *openchoreov1alpha1.RenderedRelease) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(renderedRelease, DataPlaneCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// STEP 1: Set finalizing status condition and return to persist it
	// Mark the RenderedRelease condition as finalizing and return so that the RenderedRelease will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&renderedRelease.Status.Conditions, NewRenderedReleaseFinalizingCondition(renderedRelease.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, renderedRelease); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// STEP 2: Get plane client (dataplane or observabilityplane) and find all managed resources
	targetPlane := renderedRelease.Spec.TargetPlane
	if targetPlane == "" {
		targetPlane = "dataplane" // Default to dataplane if not specified
	}

	var planeClient client.Client
	var err error
	switch targetPlane {
	case "observabilityplane":
		planeClient, err = r.getOPClient(ctx, renderedRelease.Namespace, renderedRelease.Spec.EnvironmentName)
		if err != nil {
			meta.SetStatusCondition(&renderedRelease.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(renderedRelease.Generation, err))
			if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, renderedRelease); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, fmt.Errorf("failed to get observability plane client for finalization: %w", err)
		}
	case "dataplane":
		fallthrough
	default:
		planeClient, err = r.getDPClient(ctx, renderedRelease.Namespace, renderedRelease.Spec.EnvironmentName)
		if err != nil {
			meta.SetStatusCondition(&renderedRelease.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(renderedRelease.Generation, err))
			if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, renderedRelease); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, fmt.Errorf("failed to get dataplane client for finalization: %w", err)
		}
	}

	// STEP 3: List all live resources we manage (use empty desired resources since we want to delete everything)
	var emptyDesiredResources []*unstructured.Unstructured
	gvks := findAllKnownGVKs(emptyDesiredResources, renderedRelease.Status.Resources)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, planeClient, renderedRelease, gvks)
	if err != nil {
		meta.SetStatusCondition(&renderedRelease.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(renderedRelease.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, renderedRelease); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to list live resources for cleanup: %w", err)
	}

	// STEP 4: Delete all live resources (since we want to delete everything, all live resources are "stale")
	if err := r.deleteResources(ctx, planeClient, liveResources); err != nil {
		meta.SetStatusCondition(&renderedRelease.Status.Conditions, NewRenderedReleaseCleanupFailedCondition(renderedRelease.Generation, err))
		if updateErr := controller.UpdateStatusConditions(ctx, r.Client, old, renderedRelease); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, fmt.Errorf("failed to delete resources during finalization: %w", err)
	}

	// STEP 5: Check if any resources still exist - if so, requeue for retry
	if len(liveResources) > 0 {
		logger := log.FromContext(ctx).WithValues("renderedrelease", renderedRelease.Name)
		logger.Info("Resource deletion is still pending, retrying...", "remainingResources", len(liveResources))
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// STEP 6: All resources cleaned up - remove the finalizer
	if controllerutil.RemoveFinalizer(renderedRelease, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, renderedRelease); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
