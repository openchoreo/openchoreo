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

package project

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

const (
	// ProjectCleanupFinalizer is the finalizer that is used to clean up project resources.
	ProjectCleanupFinalizer = "core.choreo.dev/project-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the project.
// The first return value indicates whether the finalizer was added to the project.
func (r *Reconciler) ensureFinalizer(ctx context.Context, project *choreov1.Project) (bool, error) {
	// If the project is being deleted, no need to add the finalizer
	if !project.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(project, ProjectCleanupFinalizer) {
		return true, r.Update(ctx, project)
	}

	return false, nil
}

// finalize cleans up the resources associated with the project.
func (r *Reconciler) finalize(ctx context.Context, old, project *choreov1.Project) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	if !controllerutil.ContainsFinalizer(project, ProjectCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the project condition as finalizing and return so that the project will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&project.Status.Conditions, NewProjectFinalizingCondition(project.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, project)
	}

	// Perform cleanup logic for deployment tracks
	artifactsDeleted, err := r.deleteComponentsAndWait(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete components")
		return ctrl.Result{}, err
	}
	if !artifactsDeleted {
		logger.Info("Components are still being deleted", "name", project.Name)
		return ctrl.Result{}, nil
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(project, ProjectCleanupFinalizer) {
		if err := r.Update(ctx, project); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized project")
	return ctrl.Result{}, nil
}

// deleteComponentsAndWait cleans up any resources that are dependent on this Project
func (r *Reconciler) deleteComponentsAndWait(ctx context.Context, project *choreov1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)
	logger.Info("Cleaning up dependent resources")

	// Find all Components owned by this Project using the project label
	componentsList := &choreov1.ComponentList{}
	listOpts := []client.ListOption{
		client.InNamespace(project.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: controller.GetOrganizationName(project),
			labels.LabelKeyProjectName:      project.Name,
		},
	}

	if err := r.List(ctx, componentsList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			// The Component resource may have been deleted since it triggered the reconcile
			logger.Info("Component not found. Ignoring since it must either be deleted or no components have been created.")
			return true, nil
		}

		// It's a real error
		return false, fmt.Errorf("failed to list components: %w", err)
	}

	pendingDeletion := false
	// Check if any components still exist
	if len(componentsList.Items) > 0 {
		// Process each Component
		for i := range componentsList.Items {
			component := &componentsList.Items[i]

			// Check if the component is already being deleted
			if !component.DeletionTimestamp.IsZero() {
				// Still in the process of being deleted
				pendingDeletion = true
				logger.Info("Component is still being deleted", "name", component.Name)
				continue
			}

			// If not being deleted, trigger deletion
			logger.Info("Deleting component", "name", component.Name)
			if err := r.Delete(ctx, component); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Component already deleted", "name", component.Name)
					continue
				}
				return false, fmt.Errorf("failed to delete component %s: %w", component.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still components being deleted, go to next iteration to check again later
		if pendingDeletion {
			return false, nil
		}
	}

	logger.Info("All components are deleted")
	return true, nil
}
