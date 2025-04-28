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

package dataplane

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// DataPlaneCleanupFinalizer is the finalizer that is used to clean up dataplane resources.
const DataPlaneCleanupFinalizer = "core.choreo.dev/dataplane-cleanup"

// ensureFinalizer ensures that the finalizer is added to the dataplane.
// The first return value indicates whether the finalizer was added to the dataplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, dataPlane *choreov1.DataPlane) (bool, error) {
	// If the dataplane is being deleted, no need to add the finalizer
	if !dataPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, dataPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, old, dataPlane *choreov1.DataPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)

	if !controllerutil.ContainsFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Mark the condition as finalizing and return so that the dataplane will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&dataPlane.Status.Conditions, NewDataPlaneFinalizingCondition(dataPlane.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, dataPlane); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Perform cleanup logic for referenced environments
	artifactsDeleted, err := r.deleteEnvironmnetsAndWait(ctx, dataPlane)
	if err != nil {
		logger.Error(err, "Failed to delete environments")
		return ctrl.Result{}, err
	}
	if !artifactsDeleted {
		logger.Info("Environments are still being deleted", "name", dataPlane.Name)
		return ctrl.Result{}, nil
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, dataPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized dataplane")
	return ctrl.Result{}, nil
}

// deleteEnvironmnetsAndWait deletes referenced deployments and waits for them to be fully deleted
func (r *Reconciler) deleteEnvironmnetsAndWait(ctx context.Context, dataPlane *choreov1.DataPlane) (bool, error) {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)
	logger.Info("Cleaning up environments")

	// Find all Environments referred to by this Dataplane
	environmentsList := &choreov1.EnvironmentList{}
	listOpts := []client.ListOption{
		client.InNamespace(dataPlane.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: controller.GetOrganizationName(dataPlane),
		},
		client.MatchingFields{
			dataplaneRefIndexKey: dataPlane.Name,
		},
	}

	if err := r.List(ctx, environmentsList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Environments not found. Continuing with deletion.")
			return true, nil
		}
		return false, fmt.Errorf("failed to list environments: %w", err)
	}

	pendingDeletion := false

	// Check if any environmnets still exist
	if len(environmentsList.Items) > 0 {
		// Process each Environment
		for i := range environmentsList.Items {
			environment := &environmentsList.Items[i]

			// Check if the environment is already being deleted
			if !environment.DeletionTimestamp.IsZero() {
				// Still in the process of being deleted
				pendingDeletion = true
				logger.Info("Environment is still being deleted", "name", environment.Name)
				continue
			}

			// If not being deleted, trigger deletion
			logger.Info("Deleting environment", "name", environment.Name)
			if err := r.Delete(ctx, environment); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Environment already deleted", "name", environment.Name)
					continue
				}
				return false, fmt.Errorf("failed to delete environment %s: %w", environment.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still deployments being deleted, go to next iteration to check again later
		if pendingDeletion {
			return false, nil
		}
	}

	logger.Info("All environments are deleted")
	return true, nil
}
