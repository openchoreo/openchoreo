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

package deployment

import (
	"context"
	"errors"
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
	k8sintegrations "github.com/openchoreo/openchoreo/internal/controller/deployment/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// DataPlaneCleanupFinalizer is the finalizer that is used to clean up the data plane resources.
	DataPlaneCleanupFinalizer = "core.choreo.dev/data-plane-cleanup"
)

var ErrEndpointDeletionWait = errors.New("some endpoints are still finalizing, retry later")

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

	resourceHandlers := r.makeExternalResourceHandlers()
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		// Skip the namespace resource as it should not be considered to handle the deletion
		if resourceHandler.Name() == k8sintegrations.NamespaceHandlerName {
			continue
		}
		// Attempt to delete the resource
		if err := resourceHandler.Delete(ctx, deploymentCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}

		// Check if the resource is still being deleted
		exists, err := resourceHandler.GetCurrentState(ctx, deploymentCtx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check existence of external resource %s: %w", resourceHandler.Name(), err)
		}
		if exists != nil {
			pendingDeletion = true
		}
	}

	// Requeue the reconcile loop if there are still resources pending deletion
	if pendingDeletion {
		logger.Info("endpoint deletion is still pending as the dependent resource deletion pending.. retrying..")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Clean up the endpoints associated with the deployment
	if err := r.cleanupEndpoints(ctx, deployment); err != nil {
		if errors.Is(err, ErrEndpointDeletionWait) {
			// this means the endpoint deletion is still in progress. So, we need to retry later.
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	// Remove the finalizer after all the data plane resources are cleaned up
	if controllerutil.RemoveFinalizer(deployment, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) cleanupEndpoints(ctx context.Context, deployment *choreov1.Deployment) error {
	logger := log.FromContext(ctx).WithValues("deployment", deployment.Name)
	logger.Info("Cleaning up the endpoints associated with the deployment")

	// List all the endpoints associated with the deployment
	endpointList := &choreov1.EndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(deployment.Namespace),
		client.MatchingLabels{
			labels.LabelKeyDeploymentName: deployment.Name,
		},
	}

	if err := r.List(ctx, endpointList, listOpts...); err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info("No endpoints associated with the environment")
			return nil
		}
		return fmt.Errorf("error listing endpoints: %w", err)
	}

	pendingDeletion := false

	for _, endpoint := range endpointList.Items {
		if err := r.Delete(ctx, &endpoint); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The endpoint is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error deleting endpoint %s: %w", endpoint.Name, err)
		}

		// Get the resource back to check if the resource still exists
		if err := r.Get(ctx, client.ObjectKeyFromObject(&endpoint), &endpoint); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The endpoint is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error getting endpoint %s: %w", endpoint.Name, err)
		}

		// marking blocked as true as the endpoint has still not deleted.
		pendingDeletion = true
	}

	// If at least one endpoint is blocked, signal that we need to retry later
	if pendingDeletion {
		return ErrEndpointDeletionWait
	}

	return nil
}
