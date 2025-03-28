/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package component

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/labels"
)

const (
	// ComponentCleanupFinalizer is the finalizer that is used to clean up component resources.
	ComponentCleanupFinalizer = "core.choreo.dev/component-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the component.
// The first return value indicates whether the finalizer was added to the component.
func (r *Reconciler) ensureFinalizer(ctx context.Context, component *choreov1.Component) (bool, error) {
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
func (r *Reconciler) finalize(ctx context.Context, old, component *choreov1.Component) (ctrl.Result, error) {
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
	if err := r.deleteDeploymentTracksAndWait(ctx, component); err != nil {
		logger.Info("Waiting for dependent resources to be deleted", "error", err.Error())
		// Return with requeue to check again later
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
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
func (r *Reconciler) deleteDeploymentTracksAndWait(ctx context.Context, component *choreov1.Component) error {
	logger := log.FromContext(ctx).WithValues("component", component.Name)
	logger.Info("Cleaning up dependent resources")

	// Find all DeploymentTracks owned by this Component using the component label
	deploymentTrackList := &choreov1.DeploymentTrackList{}
	listOpts := []client.ListOption{
		client.InNamespace(component.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: controller.GetOrganizationName(component),
			labels.LabelKeyProjectName:      controller.GetProjectName(component),
			labels.LabelKeyComponentName:    component.Name,
		},
	}

	if err := r.List(ctx, deploymentTrackList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			// The DeploymentTrack resource may have been deleted since it triggered the reconcile
			logger.Info("Deployment track not found. Ignoring since it must either be deleted or no deployment tracks have been created.")
			return nil
		}

		// It's a real error
		return fmt.Errorf("failed to list deployment tracks: %w", err)
	}

	// Check if any deployment tracks still exist
	if len(deploymentTrackList.Items) > 0 {
		pendingDeletion := false

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
				return fmt.Errorf("failed to delete deployment track %s: %w", deploymentTrack.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still tracks being deleted, requeue to check again later
		if pendingDeletion {
			return fmt.Errorf("waiting for deployment tracks to be fully deleted")
		}
	}

	logger.Info("All deployment tracks are deleted")
	return nil
}
