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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// DeploymentTrackCleanupFinalizer is the finalizer that is used to clean up deployment track resources.
	DeploymentTrackCleanupFinalizer = "core.choreo.dev/deploymenttrack-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the deployment track.
// The first return value indicates whether the finalizer was added to the deployment track.
func (r *Reconciler) ensureFinalizer(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) (bool, error) {
	// If the deployment track is being deleted, no need to add the finalizer
	if !deploymentTrack.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(deploymentTrack, DeploymentTrackCleanupFinalizer) {
		return true, r.Update(ctx, deploymentTrack)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, old, deploymentTrack *choreov1.DeploymentTrack) (ctrl.Result, error) {
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
func (r *Reconciler) deleteChildResources(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) (bool, error) {
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

	// Clean up deployable artifacts
	artifactsDeleted, err := r.deleteDeployableArtifactsAndWait(ctx, deploymentTrack)
	if err != nil {
		logger.Error(err, "Failed to delete deployable artifacts")
		return false, err
	}
	if !artifactsDeleted {
		logger.Info("Deployable artifacts are still being deleted", "name", deploymentTrack.Name)
		return false, nil
	}

	// Clean up deployments
	deploymentsDeleted, err := r.deleteDeploymentsAndWait(ctx, deploymentTrack)
	if err != nil {
		logger.Error(err, "Failed to delete deployments")
		return false, err
	}
	if !deploymentsDeleted {
		logger.Info("Deployments are still being deleted", "name", deploymentTrack.Name)
		return false, nil
	}

	logger.Info("All dependent resources are deleted")
	return true, nil
}

// deleteBuildsAndWait deletes builds and waits for them to be fully deleted
func (r *Reconciler) deleteBuildsAndWait(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up builds")

	// Find all Builds owned by this DeploymentTrack
	buildList := &choreov1.BuildList{}
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

// deleteDeployableArtifactsAndWait deletes deployable artifacts and waits for them to be fully deleted
func (r *Reconciler) deleteDeployableArtifactsAndWait(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up deployable artifacts")

	// Find all DeployableArtifacts owned by this DeploymentTrack
	artifactList := &choreov1.DeployableArtifactList{}
	listOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: controller.GetName(deploymentTrack),
		},
	}

	if err := r.List(ctx, artifactList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Deployable artifacts not found. Continuing with deletion.")
			return true, nil
		}
		return false, fmt.Errorf("failed to list deployable artifacts: %w", err)
	}

	// Check if any artifacts still exist
	pendingDeletion := false

	// Process each DeployableArtifact
	for i := range artifactList.Items {
		artifact := &artifactList.Items[i]

		// Skip artifact if there is an owner reference - this will be managed by the build controller
		if len(artifact.OwnerReferences) > 0 {
			continue
		}

		// Check if the artifact is already being deleted
		if !artifact.DeletionTimestamp.IsZero() {
			// Still in the process of being deleted
			pendingDeletion = true
			logger.Info("Deployable artifact is still being deleted", "name", artifact.Name)
			continue
		}

		// If not being deleted, trigger deletion
		logger.Info("Deleting deployable artifact", "name", artifact.Name)
		if err := r.Delete(ctx, artifact); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Deployable artifact already deleted", "name", artifact.Name)
				continue
			}
			return false, fmt.Errorf("failed to delete deployable artifact %s: %w", artifact.Name, err)
		}

		// Mark as pending since we just triggered deletion
		pendingDeletion = true
	}

	// If there are still artifacts being deleted, go to next iteration to check again later
	if pendingDeletion {
		return false, nil
	}

	logger.Info("All deployable artifacts are deleted")
	return true, nil
}

// deleteDeploymentsAndWait deletes deployments and waits for them to be fully deleted
func (r *Reconciler) deleteDeploymentsAndWait(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up deployments")

	// Find all Deployments owned by this DeploymentTrack
	deploymentList := &choreov1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: controller.GetName(deploymentTrack),
		},
	}

	if err := r.List(ctx, deploymentList, listOpts...); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Deployments not found. Continuing with deletion.")
			return true, nil
		}
		return false, fmt.Errorf("failed to list deployments: %w", err)
	}

	pendingDeletion := false

	// Check if any deployments still exist
	if len(deploymentList.Items) > 0 {
		// Process each Deployment
		for i := range deploymentList.Items {
			deployment := &deploymentList.Items[i]

			// Check if the deployment is already being deleted
			if !deployment.DeletionTimestamp.IsZero() {
				// Still in the process of being deleted
				pendingDeletion = true
				logger.Info("Deployment is still being deleted", "name", deployment.Name)
				continue
			}

			// If not being deleted, trigger deletion
			logger.Info("Deleting deployment", "name", deployment.Name)
			if err := r.Delete(ctx, deployment); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Deployment already deleted", "name", deployment.Name)
					continue
				}
				return false, fmt.Errorf("failed to delete deployment %s: %w", deployment.Name, err)
			}

			// Mark as pending since we just triggered deletion
			pendingDeletion = true
		}

		// If there are still deployments being deleted, go to next iteration to check again later
		if pendingDeletion {
			return false, nil
		}
	}

	logger.Info("All deployments are deleted")
	return true, nil
}
