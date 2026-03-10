// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// dataplaneRefIndexKey is the index key for the dataplane reference
const dataplaneRefIndexKey = ".spec.dataPlaneRef"

// setupDataPlaneRefIndex creates a field index for the dataplane reference in the environments.
func (r *Reconciler) setupDataPlaneRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&openchoreov1alpha1.Environment{},
		dataplaneRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to the appropriate type
			environment, ok := obj.(*openchoreov1alpha1.Environment)
			if !ok {
				return nil
			}

			// Handle nil DataPlaneRef (defaults to "default" DataPlane)
			ref := environment.Spec.DataPlaneRef
			if ref == nil {
				return []string{controller.DefaultPlaneName}
			}

			// Only index namespace-scoped DataPlane references
			// ClusterDataPlane references are handled separately
			if ref.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane {
				return nil
			}

			// Return the name of the DataPlane
			return []string{ref.Name}
		},
	)
}

func (r *Reconciler) GetDataPlaneForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	environment, ok := obj.(*openchoreov1alpha1.Environment)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be an Environment from the Watch
		return nil
	}

	result, err := controller.GetDataPlaneOrClusterDataPlaneOfEnv(ctx, r.Client, environment)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to resolve dataplane for environment",
			"environment", environment.Name, "namespace", environment.Namespace)
		return nil
	}

	// Only enqueue if the result is a namespace-scoped DataPlane (this controller reconciles DataPlane, not ClusterDataPlane)
	if result.DataPlane == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      result.DataPlane.Name,
				Namespace: result.DataPlane.Namespace,
			},
		},
	}
}
