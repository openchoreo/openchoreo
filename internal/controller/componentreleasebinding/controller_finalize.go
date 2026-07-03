// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

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
	// ComponentReleaseBindingFinalizer is the finalizer that ensures Releases are deleted before ComponentReleaseBinding
	ComponentReleaseBindingFinalizer = "openchoreo.dev/componentreleasebinding-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the ComponentReleaseBinding.
// The first return value indicates whether the finalizer was added to the ComponentReleaseBinding.
func (r *Reconciler) ensureFinalizer(ctx context.Context, releaseBinding *openchoreov1alpha1.ComponentReleaseBinding) (bool, error) {
	// If the ComponentReleaseBinding is being deleted, no need to add the finalizer
	if !releaseBinding.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(releaseBinding, ComponentReleaseBindingFinalizer) {
		return true, r.Update(ctx, releaseBinding)
	}

	return false, nil
}

// finalize cleans up the resources associated with the ComponentReleaseBinding.
func (r *Reconciler) finalize(ctx context.Context, old, releaseBinding *openchoreov1alpha1.ComponentReleaseBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("releaseBinding", releaseBinding.Name)

	if !controllerutil.ContainsFinalizer(releaseBinding, ComponentReleaseBindingFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the releaseBinding condition as finalizing and return so that the releaseBinding will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&releaseBinding.Status.Conditions, NewComponentReleaseBindingFinalizingCondition(releaseBinding.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, releaseBinding)
	}

	// Check if any Releases owned by this ComponentReleaseBinding still exist
	hasReleases, err := r.hasOwnedReleases(ctx, releaseBinding)
	if err != nil {
		logger.Error(err, "Failed to check for owned Releases")
		return ctrl.Result{}, err
	}

	if hasReleases {
		logger.Info("Waiting for Releases to be deleted")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// All Releases are deleted - remove the finalizer
	if controllerutil.RemoveFinalizer(releaseBinding, ComponentReleaseBindingFinalizer) {
		if err := r.Update(ctx, releaseBinding); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized ComponentReleaseBinding")
	return ctrl.Result{}, nil
}

// hasOwnedReleases checks if any Releases owned by this ComponentReleaseBinding still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedReleases(ctx context.Context, releaseBinding *openchoreov1alpha1.ComponentReleaseBinding) (bool, error) {
	logger := log.FromContext(ctx).WithValues("releaseBinding", releaseBinding.Name)

	// List Releases owned by this ComponentReleaseBinding using label selectors
	matchingLabels := client.MatchingLabels{
		labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
		labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
		labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
	}
	releaseList := &openchoreov1alpha1.RenderedReleaseList{}
	if err := r.List(ctx, releaseList,
		client.InNamespace(releaseBinding.Namespace),
		matchingLabels); err != nil {
		return false, fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releaseList.Items) == 0 {
		return false, nil
	}

	// Delete all Releases owned by this ComponentReleaseBinding
	logger.Info("Deleting owned Releases", "count", len(releaseList.Items))
	if err := r.DeleteAllOf(ctx, &openchoreov1alpha1.RenderedRelease{},
		client.InNamespace(releaseBinding.Namespace),
		matchingLabels); err != nil {
		return false, fmt.Errorf("failed to delete releases: %w", err)
	}

	return true, nil
}
