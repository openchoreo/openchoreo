/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package endpoint

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	logger := log.FromContext(ctx).WithValues("endpoint", ep.Name)
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

	dpClient, err := r.getDPClient(ctx, epCtx.Environment)
	if err != nil {
		logger.Error(err, "Error getting DP client")
		return ctrl.Result{}, err
	}

	resourceHandlers := r.makeExternalResourceHandlers(dpClient)
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		exists, err := resourceHandler.GetCurrentState(ctx, epCtx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get current state of external resource %s: %w", resourceHandler.Name(), err)
		}

		if exists != nil {
			pendingDeletion = true
			// Trigger deletion of the resource as it is still exists
			if err := resourceHandler.Delete(ctx, epCtx); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
			}
		}
	}

	// Requeue the reconcile loop if there are still resources pending deletion
	if pendingDeletion {
		logger.Info("endpoint deletion is still pending as the dependent resource deletion pending.. retrying..")
		return ctrl.Result{Requeue: true}, nil
	}

	// Remove the finalizer after all the data plane resources are cleaned up
	if controllerutil.RemoveFinalizer(ep, choreov1.EndpointDeletionFinalizer) {
		if err := r.Update(ctx, ep); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
