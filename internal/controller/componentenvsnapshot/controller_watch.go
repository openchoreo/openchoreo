// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentenvsnapshot

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// componentNameIndex is the field index name for component name
	componentNameIndex = "spec.owner.componentName"
	// environmentIndex is the field index name for environment
	environmentIndex = "spec.environment"
)

// setupComponentNameRefIndex sets up the field index for component name references
func (r *Reconciler) setupComponentNameRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentEnvSnapshot{},
		componentNameIndex, func(obj client.Object) []string {
			snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)
			return []string{snapshot.Spec.Owner.ComponentName}
		})
}

// setupEnvironmentRefIndex sets up the field index for environment references
func (r *Reconciler) setupEnvironmentRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentEnvSnapshot{},
		environmentIndex, func(obj client.Object) []string {
			snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)
			return []string{snapshot.Spec.Environment}
		})
}

// listSnapshotsForComponentDeployment returns reconcile requests for all snapshots using this ComponentDeployment
func (r *Reconciler) listSnapshotsForComponentDeployment(ctx context.Context, obj client.Object) []reconcile.Request {
	componentDeployment := obj.(*openchoreov1alpha1.ComponentDeployment)

	// Find all snapshots for this component + environment
	var snapshots openchoreov1alpha1.ComponentEnvSnapshotList
	if err := r.List(ctx, &snapshots,
		client.InNamespace(componentDeployment.Namespace),
		client.MatchingFields{
			componentNameIndex: componentDeployment.Spec.Owner.ComponentName,
			environmentIndex:   componentDeployment.Spec.Environment,
		}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list ComponentEnvSnapshots for ComponentDeployment", "componentDeployment", componentDeployment.Name, "namespace", componentDeployment.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, len(snapshots.Items))
	for i, snapshot := range snapshots.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      snapshot.Name,
				Namespace: snapshot.Namespace,
			},
		}
	}
	return requests
}

// listSnapshotsUsingAddon returns reconcile requests for all snapshots using this Addon
func (r *Reconciler) listSnapshotsUsingAddon(ctx context.Context, obj client.Object) []reconcile.Request {
	addon := obj.(*openchoreov1alpha1.Addon)

	// In Phase 7, we'll use the field index to find snapshots using this addon
	// For now, return empty to avoid errors
	log.FromContext(ctx).Info("Addon changed but snapshot reconciliation not implemented yet",
		"addon", addon.Name)

	return []reconcile.Request{}
}
