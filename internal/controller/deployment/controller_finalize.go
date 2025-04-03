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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
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
	for _, resourceHandler := range resourceHandlers {
		if err := resourceHandler.Delete(ctx, deploymentCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	// Remove the finalizer after all the data plane resources are cleaned up
	if controllerutil.RemoveFinalizer(deployment, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
