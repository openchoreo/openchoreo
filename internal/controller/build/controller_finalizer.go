package build

import (
	"context"
	"fmt"
	choreov1 "github.com/choreo-idp/choreo/api/v1"
	argointegrations "github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes/ci/argo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// CleanUpFinalizer is used to ensure proper cleanup of data plane resources before a Build resource is deleted.
	CleanUpFinalizer = "core.choreo.dev/cleanup"
)

// ensureFinalizer ensures that the build resource has the cleanup finalizer.
// Returns true if the finalizer was added, and false if it was already present or not needed.
func (r *Reconciler) ensureFinalizer(ctx context.Context, build *choreov1.Build) (bool, error) {
	// If the build is being deleted, do not add the finalizer
	if !build.DeletionTimestamp.IsZero() {
		return false, nil
	}

	// Attempt to add the finalizer to the build resource
	if controllerutil.AddFinalizer(build, CleanUpFinalizer) {
		// Update the resource to persist the finalizer addition
		return true, r.Update(ctx, build)
	}

	return false, nil
}

// finalize cleans up data plane resources associated with the build before deletion.
// It is invoked when the build resource has the cleanup finalizer.
func (r *Reconciler) finalize(ctx context.Context, build *choreov1.Build) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(build, CleanUpFinalizer) {
		return ctrl.Result{}, nil
	}

	// Construct the build context for resource finalization
	buildCtx, err := r.makeBuildContext(ctx, build)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct build context for finalization: %w", err)
	}

	// Initialize the workflow handler and attempt to delete the workflow resource
	workflowHandler := argointegrations.NewWorkflowHandler(r.Client)
	if err := workflowHandler.Delete(ctx, buildCtx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete workflow resource: %w", err)
	}

	// Remove the finalizer after successful cleanup
	if controllerutil.RemoveFinalizer(build, CleanUpFinalizer) {
		// Update the resource to reflect finalizer removal
		if err := r.Update(ctx, build); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}
