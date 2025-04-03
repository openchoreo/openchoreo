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

package endpoint

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ensureFinalizer ensures that the finalizer is added to the endpoint.
func (r *Reconciler) ensureFinalizer(ctx context.Context, ep *choreov1.Endpoint) error {
	// If the deployment is being deleted, no need to add the finalizer
	if !ep.DeletionTimestamp.IsZero() {
		return nil
	}
	if controllerutil.AddFinalizer(ep, choreov1.EndpointDeletionFinalizer) {
		return r.Update(ctx, ep)
	}
	return nil
}

// finalize cleans up the data plane resources associated with the endpoint.
func (r *Reconciler) finalize(ctx context.Context, old, ep *choreov1.Endpoint) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ep, choreov1.EndpointDeletionFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the endpoint condition as finalizing and return so that the deployment will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&ep.Status.Conditions, EndpointTerminatingCondition(ep.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, ep); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Get the endpoint context and delete the data plane resources
	epCtx, err := r.makeEndpointContext(ctx, ep)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct endpoint context for finalization: %w", err)
	}

	resourceHandlers := r.makeExternalResourceHandlers()
	for _, resourceHandler := range resourceHandlers {
		if err := resourceHandler.Delete(ctx, epCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	// Remove the finalizer after all the data plane resources are cleaned up
	if controllerutil.RemoveFinalizer(ep, choreov1.EndpointDeletionFinalizer) {
		if err := r.Update(ctx, ep); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
