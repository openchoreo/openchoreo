// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// componentDeploymentComponentIndex indexes ComponentDeployment by component name
	componentDeploymentComponentIndex = "spec.owner.componentName"
	// componentDeploymentEnvironmentIndex indexes ComponentDeployment by environment
	componentDeploymentEnvironmentIndex = "spec.environment"
	// componentDeploymentCompositeIndex indexes ComponentDeployment by component name and environment (composite key)
	componentDeploymentCompositeIndex = "componentEnvironmentComposite"
	// snapshotOwnerIndex indexes ComponentEnvSnapshot by owner fields and environment
	snapshotOwnerIndex = "snapshotOwnerComposite"
)

// setupComponentIndex registers an index for ComponentDeployment by component name.
func (r *Reconciler) setupComponentIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentDeployment{},
		componentDeploymentComponentIndex, func(obj client.Object) []string {
			componentDeployment := obj.(*openchoreov1alpha1.ComponentDeployment)
			if componentDeployment.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{componentDeployment.Spec.Owner.ComponentName}
		})
}

// setupEnvironmentIndex registers an index for ComponentDeployment by environment.
func (r *Reconciler) setupEnvironmentIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentDeployment{},
		componentDeploymentEnvironmentIndex, func(obj client.Object) []string {
			componentDeployment := obj.(*openchoreov1alpha1.ComponentDeployment)
			if componentDeployment.Spec.Environment == "" {
				return nil
			}
			return []string{componentDeployment.Spec.Environment}
		})
}

// setupComponentDeploymentCompositeIndex registers a composite index for ComponentDeployment by component name and environment.
func (r *Reconciler) setupComponentDeploymentCompositeIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentDeployment{},
		componentDeploymentCompositeIndex, func(obj client.Object) []string {
			componentDeployment := obj.(*openchoreov1alpha1.ComponentDeployment)
			// Create a composite key: componentName/environment
			compositeKey := fmt.Sprintf("%s/%s",
				componentDeployment.Spec.Owner.ComponentName,
				componentDeployment.Spec.Environment)
			return []string{compositeKey}
		})
}

// setupSnapshotOwnerIndex registers an index for ComponentEnvSnapshot by owner fields and environment.
func (r *Reconciler) setupSnapshotOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentEnvSnapshot{},
		snapshotOwnerIndex, func(obj client.Object) []string {
			snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)
			// Create a composite key: projectName/componentName/environment
			ownerKey := fmt.Sprintf("%s/%s/%s",
				snapshot.Spec.Owner.ProjectName,
				snapshot.Spec.Owner.ComponentName,
				snapshot.Spec.Environment)
			return []string{ownerKey}
		})
}

// listComponentDeploymentForSnapshot enqueues ComponentDeployment that correspond to the given ComponentEnvSnapshot.
func (r *Reconciler) listComponentDeploymentForSnapshot(ctx context.Context, obj client.Object) []reconcile.Request {
	snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)

	// Build composite key: componentName/environment
	compositeKey := fmt.Sprintf("%s/%s",
		snapshot.Spec.Owner.ComponentName,
		snapshot.Spec.Environment)

	var componentDeploymentList openchoreov1alpha1.ComponentDeploymentList
	if err := r.List(ctx, &componentDeploymentList,
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{
			componentDeploymentCompositeIndex: compositeKey,
		}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list ComponentDeployment for ComponentEnvSnapshot", "snapshot", obj.GetName(), "namespace", obj.GetNamespace())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(componentDeploymentList.Items))
	for _, componentDeployment := range componentDeploymentList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      componentDeployment.Name,
				Namespace: componentDeployment.Namespace,
			},
		})
	}
	return requests
}
