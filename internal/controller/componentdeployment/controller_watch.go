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
	// componentDeploymentCompositeIndex indexes ComponentDeployment by project, component name, and environment (composite key)
	componentDeploymentCompositeIndex = "projectComponentEnvironmentComposite"
	// snapshotOwnerIndex indexes ComponentEnvSnapshot by owner fields and environment
	snapshotOwnerIndex = "snapshotOwnerComposite"
)

// buildProjectComponentEnvironmentKey creates the shared composite key format project/component/environment.
func buildProjectComponentEnvironmentKey(project, component, environment string) string {
	return fmt.Sprintf("%s/%s/%s", project, component, environment)
}

// setupComponentDeploymentCompositeIndex registers a composite index for ComponentDeployment by project, component name, and environment.
func (r *Reconciler) setupComponentDeploymentCompositeIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentDeployment{},
		componentDeploymentCompositeIndex, func(obj client.Object) []string {
			componentDeployment := obj.(*openchoreov1alpha1.ComponentDeployment)
			compositeKey := buildProjectComponentEnvironmentKey(
				componentDeployment.Spec.Owner.ProjectName,
				componentDeployment.Spec.Owner.ComponentName,
				componentDeployment.Spec.Environment,
			)
			return []string{compositeKey}
		})
}

// setupSnapshotOwnerIndex registers an index for ComponentEnvSnapshot by owner fields and environment.
func (r *Reconciler) setupSnapshotOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentEnvSnapshot{},
		snapshotOwnerIndex, func(obj client.Object) []string {
			snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)
			project := snapshot.Spec.Owner.ProjectName
			component := snapshot.Spec.Owner.ComponentName
			environment := snapshot.Spec.Environment
			ownerKey := buildProjectComponentEnvironmentKey(project, component, environment)
			return []string{ownerKey}
		})
}

// listComponentDeploymentForSnapshot enqueues ComponentDeployment that correspond to the given ComponentEnvSnapshot.
func (r *Reconciler) listComponentDeploymentForSnapshot(ctx context.Context, obj client.Object) []reconcile.Request {
	snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)
	project := snapshot.Spec.Owner.ProjectName
	component := snapshot.Spec.Owner.ComponentName
	environment := snapshot.Spec.Environment
	compositeKey := buildProjectComponentEnvironmentKey(project, component, environment)

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
