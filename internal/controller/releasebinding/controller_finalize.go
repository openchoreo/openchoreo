// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// ReleaseBindingFinalizer is the finalizer that ensures Releases are deleted before ReleaseBinding
	ReleaseBindingFinalizer = "openchoreo.dev/releasebinding-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the ReleaseBinding.
// The first return value indicates whether the finalizer was added to the ReleaseBinding.
func (r *Reconciler) ensureFinalizer(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) (bool, error) {
	// If the ReleaseBinding is being deleted, no need to add the finalizer
	if !releaseBinding.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(releaseBinding, ReleaseBindingFinalizer) {
		return true, r.Update(ctx, releaseBinding)
	}

	return false, nil
}

// finalize cleans up the resources associated with the ReleaseBinding.
func (r *Reconciler) finalize(ctx context.Context, old, releaseBinding *openchoreov1alpha1.ReleaseBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("releaseBinding", releaseBinding.Name)

	if !controllerutil.ContainsFinalizer(releaseBinding, ReleaseBindingFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the releaseBinding condition as finalizing and return so that the releaseBinding will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&releaseBinding.Status.Conditions, NewReleaseBindingFinalizingCondition(releaseBinding.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, releaseBinding)
	}

	// Check if any RenderedReleases owned by this ReleaseBinding still exist
	hasRenderedReleases, err := r.hasOwnedRenderedReleases(ctx, releaseBinding)
	if err != nil {
		logger.Error(err, "Failed to check for owned RenderedReleases")
		return ctrl.Result{}, err
	}

	if hasRenderedReleases {
		logger.Info("Waiting for RenderedReleases to be deleted")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// All RenderedReleases are deleted - remove the finalizer
	if controllerutil.RemoveFinalizer(releaseBinding, ReleaseBindingFinalizer) {
		if err := r.Update(ctx, releaseBinding); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized ReleaseBinding")
	return ctrl.Result{}, nil
}

// hasOwnedRenderedReleases checks if any RenderedReleases owned by this ReleaseBinding still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedRenderedReleases(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) (bool, error) {
	logger := log.FromContext(ctx).WithValues("releaseBinding", releaseBinding.Name)

	// List RenderedReleases owned by this ReleaseBinding using label selectors
	matchingLabels := client.MatchingLabels{
		labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
		labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
		labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
	}
	renderedReleaseList := &openchoreov1alpha1.RenderedReleaseList{}
	if err := r.List(ctx, renderedReleaseList,
		client.InNamespace(releaseBinding.Namespace),
		matchingLabels); err != nil {
		return false, fmt.Errorf("failed to list rendered releases: %w", err)
	}

	if len(renderedReleaseList.Items) == 0 {
		return false, nil
	}

	// Delete all RenderedReleases owned by this ReleaseBinding
	logger.Info("Deleting owned RenderedReleases", "count", len(renderedReleaseList.Items))
	if err := r.DeleteAllOf(ctx, &openchoreov1alpha1.RenderedRelease{},
		client.InNamespace(releaseBinding.Namespace),
		matchingLabels); err != nil {
		return false, fmt.Errorf("failed to delete rendered releases: %w", err)
	}

	return true, nil
}
