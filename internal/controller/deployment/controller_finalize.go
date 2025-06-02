// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	"context"
	"fmt"
	"time"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// DataPlaneCleanupFinalizer is the finalizer that is used to clean up the data plane resources.
	DataPlaneCleanupFinalizer = "core.choreo.dev/data-plane-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the deployment.
// The first return value indicates whether the finalizer was added to the deployment.
func (r *Reconciler) ensureFinalizer(ctx context.Context, deployment *choreov1.Deployment) (bool, error) {
	// If the deployment is being deleted, no need to add the finalizer
	if !deployment.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(deployment, DataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, deployment)
	}

	return false, nil
}

// finalize cleans up the data plane resources associated with the deployment.
func (r *Reconciler) finalize(ctx context.Context, old, deployment *choreov1.Deployment) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deployment", deployment.Name)
	if !controllerutil.ContainsFinalizer(deployment, DataPlaneCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the deployment condition as finalizing and return so that the deployment will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&deployment.Status.Conditions, NewDeploymentFinalizingCondition(deployment.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, deployment); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Get the deployment context and delete the data plane resources
	deploymentCtx, err := r.makeDeploymentContext(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct deployment context for finalization: %w", err)
	}

	dpClient, err := r.getDPClient(ctx, deploymentCtx.Environment)
	if err != nil {
		logger.Error(err, "Error getting DP client")
		return ctrl.Result{}, err
	}

	resourceHandlers := r.makeExternalResourceHandlers(dpClient)
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		// Skip the namespace resource as it should not be considered to handle the deletion
		// Otherwise it will become an infinite retry, as the deletion of namespace is not implemented (returning null)
		// and GetCurrentState returns the actual resource.
		if resourceHandler.Name() == "KubernetesNamespace" {
			continue
		}

		// Check if the resource is still being deleted
		exists, err := resourceHandler.GetCurrentState(ctx, deploymentCtx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check existence of external resource %s: %w", resourceHandler.Name(), err)
		}

		if exists == nil {
			continue
		}

		pendingDeletion = true
		// Trigger deletion of the resource as it is still exists
		if err := resourceHandler.Delete(ctx, deploymentCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	// Requeue the reconcile loop if there are still resources pending deletion
	if pendingDeletion {
		logger.Info("endpoint deletion is still pending as the dependent resource deletion pending.. retrying..")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Clean up the endpoints associated with the deployment
	isPending, err := r.cleanupEndpoints(ctx, deployment)

	if err != nil {
		return ctrl.Result{}, err
	}

	if isPending {
		// the next reconcile will be triggered after the pending endpoint/s deleted
		return ctrl.Result{}, nil
	}

	// Remove the finalizer after all the data plane resources are cleaned up
	if controllerutil.RemoveFinalizer(deployment, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// cleanupEndpoints cleans up the endpoints associated with the deployment.
// it will return true, nil if the deletion is still in progress and false, nil if the deletion is completed.
// false, error will be returned if there is an error while deleting the endpoints.
func (r *Reconciler) cleanupEndpoints(ctx context.Context, deployment *choreov1.Deployment) (bool, error) {
	logger := log.FromContext(ctx).WithValues("deployment", deployment.Name)
	logger.Info("Cleaning up the endpoints associated with the deployment")

	// List all the endpoints associated with the deployment
	endpointList := &choreov1.EndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(deployment.Namespace),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(deployment),
			labels.LabelKeyProjectName:         controller.GetProjectName(deployment),
			labels.LabelKeyComponentName:       controller.GetComponentName(deployment),
			labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(deployment),
			labels.LabelKeyDeploymentName:      controller.GetName(deployment),
		},
	}

	if err := r.List(ctx, endpointList, listOpts...); err != nil {
		return false, fmt.Errorf("error listing endpoints: %w", err)
	}

	if len(endpointList.Items) == 0 {
		logger.Info("No endpoints associated with the deployment")
		return false, nil
	}

	for _, endpoint := range endpointList.Items {
		// Check if the endpoint is being already deleting
		if !endpoint.DeletionTimestamp.IsZero() {
			continue
		}

		if err := r.Delete(ctx, &endpoint); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The endpoint is already deleted, no need to retry
				continue
			}
			return false, fmt.Errorf("error deleting endpoint %s: %w", endpoint.Name, err)
		}
	}

	// Reaching this point means the endpoint deletion is either still in progress or has just been initiated.
	// If this is the first deletion attempt, marking the pending deletion as true.
	return true, nil
}
