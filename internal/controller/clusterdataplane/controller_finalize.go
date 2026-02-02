// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
)

// ClusterDataPlaneCleanupFinalizer is the finalizer that is used to clean up clusterdataplane resources.
const ClusterDataPlaneCleanupFinalizer = "openchoreo.dev/clusterdataplane-cleanup"

// ensureFinalizer ensures that the finalizer is added to the clusterdataplane.
// The first return value indicates whether the finalizer was added to the clusterdataplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) (bool, error) {
	// If the clusterdataplane is being deleted, no need to add the finalizer
	if !clusterDataPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(clusterDataPlane, ClusterDataPlaneCleanupFinalizer) {
		return true, r.Update(ctx, clusterDataPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("clusterdataplane", clusterDataPlane.Name)

	if !controllerutil.ContainsFinalizer(clusterDataPlane, ClusterDataPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ClusterDataPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterDataPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterDataPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the ClusterDataPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, clusterDataPlane)
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(clusterDataPlane, ClusterDataPlaneCleanupFinalizer) {
		if err := r.Update(ctx, clusterDataPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized clusterdataplane")
	return ctrl.Result{}, nil
}
