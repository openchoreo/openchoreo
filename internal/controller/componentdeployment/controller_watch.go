// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"context"

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

// listComponentDeploymentForSnapshot enqueues ComponentDeployment that correspond to the given ComponentEnvSnapshot.
func (r *Reconciler) listComponentDeploymentForSnapshot(ctx context.Context, obj client.Object) []reconcile.Request {
	snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)

	var componentDeploymentList openchoreov1alpha1.ComponentDeploymentList
	if err := r.List(ctx, &componentDeploymentList,
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{
			componentDeploymentComponentIndex:   snapshot.Spec.Owner.ComponentName,
			componentDeploymentEnvironmentIndex: snapshot.Spec.Environment,
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
