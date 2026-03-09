// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// DataPlaneCleanupFinalizer is the finalizer that is used to clean up dataplane resources.
const DataPlaneCleanupFinalizer = "openchoreo.dev/dataplane-cleanup"

// ensureFinalizer ensures that the finalizer is added to the dataplane.
// The first return value indicates whether the finalizer was added to the dataplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, dataPlane *openchoreov1alpha1.DataPlane) (bool, error) {
	// If the dataplane is being deleted, no need to add the finalizer
	if !dataPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, dataPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, old, dataPlane *openchoreov1alpha1.DataPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)

	if !controllerutil.ContainsFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Block deletion if the dataplane is still referenced by any environment.
	refCount, err := r.countReferencingEnvironments(ctx, dataPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check environment references: %w", err)
	}
	if refCount > 0 {
		msg := fmt.Sprintf("Deletion blocked: dataplane is still referenced by %d environment(s)", refCount)
		logger.Info(msg)
		if meta.SetStatusCondition(&dataPlane.Status.Conditions, NewDeletionBlockedCondition(dataPlane.Generation, msg)) {
			if err := controller.UpdateStatusConditions(ctx, r.Client, old, dataPlane); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Mark the condition as finalizing and return so that the dataplane will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&dataPlane.Status.Conditions, NewDataPlaneFinalizingCondition(dataPlane.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, dataPlane); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Notify gateway of DataPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, dataPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "DataPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the DataPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, dataPlane)
	}

	// Remove the finalizer once no environments reference this dataplane
	if controllerutil.RemoveFinalizer(dataPlane, DataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, dataPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized dataplane")
	return ctrl.Result{}, nil
}

// countReferencingEnvironments returns the number of environments in the same namespace that reference this dataplane.
func (r *Reconciler) countReferencingEnvironments(ctx context.Context, dataPlane *openchoreov1alpha1.DataPlane) (int, error) {
	environmentsList := &openchoreov1alpha1.EnvironmentList{}
	if err := r.List(ctx, environmentsList,
		client.InNamespace(dataPlane.Namespace),
		client.MatchingFields{
			dataplaneRefIndexKey: dataPlane.Name,
		},
	); err != nil {
		return 0, fmt.Errorf("failed to list environments: %w", err)
	}

	return len(environmentsList.Items), nil
}
