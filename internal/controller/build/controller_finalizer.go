package build

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	argointegrations "github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes/ci/argo"
	"github.com/choreo-idp/choreo/internal/controller/build/resources"
)

const (
	// CleanUpFinalizer is used to ensure proper cleanup of data plane resources before a Build resource is deleted.
	CleanUpFinalizer = "core.choreo.dev/build-cleanup"
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
func (r *Reconciler) finalize(ctx context.Context, oldBuild *choreov1.Build, build *choreov1.Build) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(build, CleanUpFinalizer) {
		return ctrl.Result{}, nil
	}

	// Delete Workflow resource
	if err := r.deleteWorkflow(ctx, build); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete workflow resource: %w", err)
		}
	}

	// Delete DeployableArtifact if it exists
	if meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(ConditionDeployableArtifactCreated), metav1.ConditionTrue) {
		needsRequeue, err := r.deleteDeployableArtifact(ctx, build)
		if err != nil {
			return ctrl.Result{}, err
		} else if needsRequeue {
			return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
		}
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

// deleteWorkflow deletes the workflow resource.
func (r *Reconciler) deleteWorkflow(ctx context.Context, build *choreov1.Build) error {
	buildCtx := &integrations.BuildContext{Build: build}
	workflowHandler := argointegrations.NewWorkflowHandler(r.Client)
	return workflowHandler.Delete(ctx, buildCtx)
}

// deleteDeployableArtifact attempts to delete the DeployableArtifact and determines whether requeueing is needed.
func (r *Reconciler) deleteDeployableArtifact(ctx context.Context, build *choreov1.Build) (bool, error) {
	deployableArtifact := resources.MakeDeployableArtifact(build)
	existingArtifact := &choreov1.DeployableArtifact{}

	// Check if the DeployableArtifact exists
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(deployableArtifact), existingArtifact); err != nil {
		if apierrors.IsNotFound(err) {
			// Artifact does not exist, no need to requeue
			return false, nil
		}
		// Unexpected error
		return true, fmt.Errorf("failed to check deployable artifact: %w", err)
	}

	// If artifact is pending deletion (has DeletionTimestamp), update condition and requeue
	if !existingArtifact.DeletionTimestamp.IsZero() {
		meta.SetStatusCondition(&build.Status.Conditions, NewArtifactRemainingCondition(build.Generation))
		r.recorder.Event(build, corev1.EventTypeWarning, "DeployableArtifactPendingDeletion",
			"Deployable artifact is pending deletion due to finalizer. Build deletion is blocked.")
		return true, nil
	}

	// Attempt to delete the DeployableArtifact
	if err := r.Client.Delete(ctx, deployableArtifact); err != nil {
		if apierrors.IsNotFound(err) {
			// Artifact already deleted, no need to requeue
			return false, nil
		}
		// Other errors require requeueing
		return true, fmt.Errorf("failed to delete deployable artifact: %w", err)
	}

	// Deletion initiated successfully, requeue to check if it gets finalized
	return true, nil
}
