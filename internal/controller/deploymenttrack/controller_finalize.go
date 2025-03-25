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

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/labels"
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
	finalizingCond := meta.FindStatusCondition(deploymentTrack.Status.Conditions, "Finalizing")

	// First reconciliation - set the initial status
	if finalizingCond == nil {
		meta.SetStatusCondition(&deploymentTrack.Status.Conditions, NewDeploymentTrackStartFinalizeCondition(deploymentTrack.Generation))
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Execute appropriate action based on current status
	switch finalizingCond.Reason {
	case string(ReasonDeploymentTrackFinalizingStarted):
		if err := r.deleteBuilds(ctx, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		// Update status and requeue
		meta.SetStatusCondition(&deploymentTrack.Status.Conditions, NewDeploymentTrackCleanBuildsCondition(deploymentTrack.Generation))
		if err := controller.UpdateStatusConditionsWithPatch(ctx, r.Client, old, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case string(ReasonDeploymentTrackDeletingBuilds):
		if err := r.deleteDeployableArtifacts(ctx, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		// Update status and requeue
		meta.SetStatusCondition(&deploymentTrack.Status.Conditions, NewDeploymentTrackCleanDeployableArtifactsCondition(deploymentTrack.Generation))
		if err := controller.UpdateStatusConditionsWithPatch(ctx, r.Client, old, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case string(ReasonDeploymentTrackDeletingDeployableArtifacts):
		if err := r.deleteDeployments(ctx, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		// Update status and requeue
		meta.SetStatusCondition(&deploymentTrack.Status.Conditions, NewDeploymentTrackCleanDeploymentsCondition(deploymentTrack.Generation))
		if err := controller.UpdateStatusConditionsWithPatch(ctx, r.Client, old, deploymentTrack); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case string(ReasonDeploymentTrackDeletingDeployments):
		// Final stage - remove finalizer
		if controllerutil.RemoveFinalizer(deploymentTrack, DeploymentTrackCleanupFinalizer) {
			if err := r.Update(ctx, deploymentTrack); err != nil {
				if errors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteBuilds(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) error {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up builds")

	buildList := &choreov1.BuildList{}
	buildListOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: deploymentTrack.Name,
		},
	}

	if err := r.List(ctx, buildList, buildListOpts...); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to list builds: %w", err)
		}
		// Not found error is okay, it means no builds exist
		logger.Info("No builds found for deletion")
		return nil
	}

	// Process each Build
	for i := range buildList.Items {
		build := &buildList.Items[i]

		// Only process if not already being deleted
		if build.DeletionTimestamp.IsZero() {
			logger.Info("Deleting build", "name", build.Name)
			if err := r.Delete(ctx, build); err != nil {
				if !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete build %s: %w", build.Name, err)
				}
				logger.Info("Build already deleted", "name", build.Name)
			}
		}
	}
	return nil
}

func (r *Reconciler) deleteDeployableArtifacts(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) error {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up deployableArtifacts")

	deployableArtifactList := &choreov1.DeployableArtifactList{}
	artifactListOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: deploymentTrack.Name,
		},
	}

	if err := r.List(ctx, deployableArtifactList, artifactListOpts...); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to list deployable artifacts: %w", err)
		}
		// Not found error is okay, it means no deployable artifacts exist
		logger.Info("No deployable artifacts found for deletion")
		return nil
	}

	// Process each DeployableArtifact, skipping those with owner references
	for i := range deployableArtifactList.Items {
		artifact := &deployableArtifactList.Items[i]

		// Skip artifact if there is an owner references - this will be managed by the build controller
		if len(artifact.OwnerReferences) > 0 {
			continue
		}

		// Only process if not already being deleted
		if artifact.DeletionTimestamp.IsZero() {
			logger.Info("Deleting deployable artifact", "name", artifact.Name)
			if err := r.Delete(ctx, artifact); err != nil {
				if !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete deployable artifact %s: %w", artifact.Name, err)
				}
				logger.Info("Deployable artifact already deleted", "name", artifact.Name)
			}
		}
	}
	return nil
}

func (r *Reconciler) deleteDeployments(ctx context.Context, deploymentTrack *choreov1.DeploymentTrack) error {
	logger := log.FromContext(ctx).WithValues("deploymentTrack", deploymentTrack.Name)
	logger.Info("Cleaning up deployments")

	deploymentList := &choreov1.DeploymentList{}
	deploymentListOpts := []client.ListOption{
		client.InNamespace(deploymentTrack.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deploymentTrack),
			labels.LabelKeyProjectName:         controller.GetProjectName(deploymentTrack),
			labels.LabelKeyComponentName:       controller.GetComponentName(deploymentTrack),
			labels.LabelKeyDeploymentTrackName: deploymentTrack.Name,
		},
	}

	if err := r.List(ctx, deploymentList, deploymentListOpts...); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to list deployments: %w", err)
		}
		// Not found error is okay, it means no deployments exist
		logger.Info("No deployments found for deletion")
		return nil
	}

	// Process each Deployment
	for i := range deploymentList.Items {
		deployment := &deploymentList.Items[i]

		// Only process if not already being deleted
		if deployment.DeletionTimestamp.IsZero() {
			logger.Info("Deleting deployment", "name", deployment.Name)
			if err := r.Delete(ctx, deployment); err != nil {
				if !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete deployment %s: %w", deployment.Name, err)
				}
				logger.Info("Deployment already deleted", "name", deployment.Name)
			}
		}
	}
	return nil
}
