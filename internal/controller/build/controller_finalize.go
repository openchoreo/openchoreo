// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

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
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	argointegrations "github.com/openchoreo/openchoreo/internal/controller/build/integrations/kubernetes/ci/argo"
	"github.com/openchoreo/openchoreo/internal/controller/build/resources"
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
func (r *Reconciler) finalize(ctx context.Context, oldBuild, build *choreov1.Build) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(build, CleanUpFinalizer) {
		return ctrl.Result{}, nil
	}

	// Mark the build condition as finalizing and return so that the component will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&build.Status.Conditions, NewBuildFinalizingCondition(build.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
	}

	bpClient, err := r.getBPClient(ctx, build)
	logger := log.FromContext(ctx)
	if err != nil {
		logger.Error(err, "Error getting build plane client for finalizing")
	} else {
		// Delete Workflow resource if build plane exists
		if err := deleteWorkflow(ctx, build, bpClient); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete workflow resource")
			}
		}
	}

	// Delete DeployableArtifact if it exists
	if meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(ConditionDeployableArtifactCreated), metav1.ConditionTrue) {
		err := r.deleteDeployableArtifact(ctx, build)
		if err != nil {
			return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
		}
		if meta.FindStatusCondition(build.Status.Conditions, string(ConditionDeployableArtifactReferencesRemaining)) != nil {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
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
func deleteWorkflow(ctx context.Context, build *choreov1.Build, dpClient client.Client) error {
	buildCtx := &integrations.BuildContext{Build: build}
	workflowHandler := argointegrations.NewWorkflowHandler(dpClient)
	return workflowHandler.Delete(ctx, buildCtx)
}

// deleteDeployableArtifact attempts to delete the DeployableArtifact and determines whether requeueing is needed.
func (r *Reconciler) deleteDeployableArtifact(ctx context.Context, build *choreov1.Build) error {
	deployableArtifact := resources.MakeDeployableArtifact(build)
	existingArtifact := &choreov1.DeployableArtifact{}

	// Check if the DeployableArtifact exists
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(deployableArtifact), existingArtifact); err != nil {
		if apierrors.IsNotFound(err) {
			// Artifact does not exist, no need to requeue
			meta.RemoveStatusCondition(&build.Status.Conditions, string(ConditionDeployableArtifactReferencesRemaining))
			return nil
		}
		// Unexpected error
		return fmt.Errorf("failed to check deployable artifact: %w", err)
	}

	// If artifact is pending deletion, update condition and let the next cycle handle it
	if !existingArtifact.DeletionTimestamp.IsZero() {
		meta.SetStatusCondition(&build.Status.Conditions, NewArtifactRemainingCondition(build.Generation))
		r.recorder.Event(build, corev1.EventTypeWarning, "DeployableArtifactPendingDeletion",
			"Deployable artifact is pending deletion due to finalizer. Build deletion is blocked.")
		// Return nil instead of error to indicate the process is happening normally
		return nil
	}

	// Attempt to delete the DeployableArtifact
	if err := r.Client.Delete(ctx, deployableArtifact); err != nil {
		if apierrors.IsNotFound(err) {
			// Artifact already deleted, no need to requeue
			meta.RemoveStatusCondition(&build.Status.Conditions, string(ConditionDeployableArtifactReferencesRemaining))
			return nil
		}
		// Other errors require requeueing
		return fmt.Errorf("failed to delete deployable artifact: %w", err)
	}

	// Deletion initiated successfully, set condition so that the normal reconciliation cycle continues
	meta.SetStatusCondition(&build.Status.Conditions, NewArtifactRemainingCondition(build.Generation))
	return nil
}
