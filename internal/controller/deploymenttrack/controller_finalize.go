// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymenttrack

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
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
	// DeploymentTrackCleanupFinalizer is the finalizer that is used to clean up deployment track resources.
	DeploymentTrackCleanupFinalizer = "openchoreo.dev/deploymenttrack-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the deployment track.
// The first return value indicates whether the finalizer was added to the deployment track.
func (r *Reconciler) ensureFinalizer(ctx context.Context, deploymentTrack *openchoreov1alpha1.DeploymentTrack) (bool, error) {
	// If the deployment track is being deleted, no need to add the finalizer
	if !deploymentTrack.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(deploymentTrack, DeploymentTrackCleanupFinalizer) {
		return true, r.Update(ctx, deploymentTrack)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, old, deploymentTrack *openchoreov1alpha1.DeploymentTrack) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)

	if !controllerutil.ContainsFinalizer(deploymentTrack, DeploymentTrackCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the deployment condition as finalizing and return so that the deployment will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&deploymentTrack.Status.Conditions, NewDeploymentTrackFinalizingCondition(deploymentTrack.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Perform cleanup logic for dependent resources
	complete, err := r.deleteChildResources(ctx, deploymentTrack)
	if err != nil {
		logger.Error(err, "Failed to clean up child resources")
		return ctrl.Result{}, err
	}

	// If deletion is still in progress, check in next cycle
	if !complete {
		logger.Info("Child resources are still being deleted, will retry")
		return ctrl.Result{}, nil
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(deploymentTrack, DeploymentTrackCleanupFinalizer) {
		if err := r.Update(ctx, deploymentTrack); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized deployment track")
	return ctrl.Result{}, nil
}

// deleteChildResources cleans up any resources that are dependent on this DeploymentTrack
// Returns a boolean indicating if all resources are deleted and an error if something unexpected occurred
func (r *Reconciler) deleteChildResources(ctx context.Context, deploymentTrack *openchoreov1alpha1.DeploymentTrack) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)

	// Clean up builds
	buildsDeleted, err := r.deleteBuildsAndWait(ctx, deploymentTrack)
	if err != nil {
		logger.Error(err, "Failed to delete builds")
		return false, err
	}
	if !buildsDeleted {
		logger.Info("Builds are still being deleted", "name", deploymentTrack.Name)
		return false, nil
	}

	logger.Info("All dependent resources are deleted")
	return true, nil
}

// deleteBuildsAndWait deletes builds and waits for them to be fully deleted
func (r *Reconciler) deleteBuildsAndWait(ctx context.Context, deploymentTrack *openchoreov1alpha1.DeploymentTrack) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up builds")

	// Find all Builds owned by this DeploymentTrack
	buildList := &openchoreov1alpha1.BuildList{}
	listOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: controller.GetName(deploymentTrack),
		},
	}

	if err := r.List(ctx, buildList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Builds not found. Continuing with deletion.")
			return true, nil
		}
		return false, fmt.Errorf("failed to list builds: %w", err)
	}

	pendingDeletion := false

	// Check if any builds still exist
	if len(buildList.Items) > 0 {
		// Process each Build
		for i := range buildList.Items {
			build := &buildList.Items[i]

			// Check if the build is already being deleted
			if !build.DeletionTimestamp.IsZero() {
				// Still in the process of being deleted
				pendingDeletion = true
				logger.Info("Build is still being deleted", "name", build.Name)
				continue
			}

			// If not being deleted, trigger deletion
			logger.Info("Deleting build", "name", build.Name)
			if err := r.Delete(ctx, build); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Build already deleted", "name", build.Name)
					continue
				}
				return false, fmt.Errorf("failed to delete build %s: %w", build.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still builds being deleted, go to next iteration to check again later
		if pendingDeletion {
			return false, nil
		}
	}

	logger.Info("All builds are deleted")
	return true, nil
}
