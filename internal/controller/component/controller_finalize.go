// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

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
	// ComponentCleanupFinalizer is the finalizer that is used to clean up component resources.
	ComponentCleanupFinalizer = "openchoreo.dev/component-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the component.
// The first return value indicates whether the finalizer was added to the component.
func (r *Reconciler) ensureFinalizer(ctx context.Context, component *openchoreov1alpha1.Component) (bool, error) {
	// If the component is being deleted, no need to add the finalizer
	if !component.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(component, ComponentCleanupFinalizer) {
		return true, r.Update(ctx, component)
	}

	return false, nil
}

// finalize cleans up the resources associated with the component.
func (r *Reconciler) finalize(ctx context.Context, old, component *openchoreov1alpha1.Component) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("component", component.Name)

	if !controllerutil.ContainsFinalizer(component, ComponentCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the component condition as finalizing and return so that the component will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&component.Status.Conditions, NewComponentFinalizingCondition(component.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, component)
	}

	// Perform cleanup logic for deployment tracks
	artifactsDeleted, err := r.deleteDeploymentTracksAndWait(ctx, component)
	if err != nil {
		logger.Error(err, "Failed to delete deployment tracks")
		return ctrl.Result{}, err
	}
	if !artifactsDeleted {
		logger.Info("Deployment tracks are still being deleted", "name", component.Name)
		return ctrl.Result{}, nil
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(component, ComponentCleanupFinalizer) {
		if err := r.Update(ctx, component); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized component")
	return ctrl.Result{}, nil
}

// deleteDeploymentTracksAndWait cleans up any resources that are dependent on this Component
func (r *Reconciler) deleteDeploymentTracksAndWait(ctx context.Context, component *openchoreov1alpha1.Component) (bool, error) {
	logger := log.FromContext(ctx).WithValues("component", component.Name)
	logger.Info("Cleaning up dependent resources")

	// Find all DeploymentTracks owned by this Component using the component label
	deploymentTrackList := &openchoreov1alpha1.DeploymentTrackList{}
	listOpts := []client.ListOption{
		client.InNamespace(component.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: controller.GetOrganizationName(component),
			labels.LabelKeyProjectName:      controller.GetProjectName(component),
			labels.LabelKeyComponentName:    controller.GetName(component),
		},
	}

	if err := r.List(ctx, deploymentTrackList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			// The DeploymentTrack resource may have been deleted since it triggered the reconcile
			logger.Info("Deployment track not found. Ignoring since it must either be deleted or no deployment tracks have been created.")
			return true, nil
		}

		// It's a real error
		return false, fmt.Errorf("failed to list deployment tracks: %w", err)
	}

	pendingDeletion := false
	// Check if any deployment tracks still exist
	if len(deploymentTrackList.Items) > 0 {
		// Process each DeploymentTrack
		for i := range deploymentTrackList.Items {
			deploymentTrack := &deploymentTrackList.Items[i]

			// Check if the deployment track is already being deleted
			if !deploymentTrack.DeletionTimestamp.IsZero() {
				// Still in the process of being deleted
				pendingDeletion = true
				logger.Info("Deployment track is still being deleted", "name", deploymentTrack.Name)
				continue
			}

			// If not being deleted, trigger deletion
			logger.Info("Deleting deployment track", "name", deploymentTrack.Name)
			if err := r.Delete(ctx, deploymentTrack); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Deployment track already deleted", "name", deploymentTrack.Name)
					continue
				}
				return false, fmt.Errorf("failed to delete deployment track %s: %w", deploymentTrack.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still tracks being deleted, go to next iteration to check again later
		if pendingDeletion {
			return false, nil
		}
	}

	logger.Info("All deployment tracks are deleted")
	return true, nil
}
