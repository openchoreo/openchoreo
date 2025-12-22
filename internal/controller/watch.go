// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Shared field index keys for use across controllers.
// These constants ensure consistency when multiple controllers need to use the same field index.
const (
	// IndexKeyReleaseBindingOwnerComponentName indexes ReleaseBinding by owner component name.
	IndexKeyReleaseBindingOwnerComponentName = "releasebinding.spec.owner.componentName"

	// IndexKeyComponentOwnerProjectName indexes Component by owner project name.
	IndexKeyComponentOwnerProjectName = "component.spec.owner.projectName"

	// IndexKeyProjectDeploymentPipelineRef indexes Project by deploymentPipelineRef.
	IndexKeyProjectDeploymentPipelineRef = "project.spec.deploymentPipelineRef"
)

// SetupSharedIndexes registers field indexes that are shared across multiple controllers.
// This must be called before any controllers are set up with the manager.
func SetupSharedIndexes(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		IndexKeyReleaseBindingOwnerComponentName, func(obj client.Object) []string {
			binding := obj.(*openchoreov1alpha1.ReleaseBinding)
			if binding.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{binding.Spec.Owner.ComponentName}
		}); err != nil {
		return fmt.Errorf("failed to setup ReleaseBinding owner index: %w", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		IndexKeyComponentOwnerProjectName, func(obj client.Object) []string {
			component := obj.(*openchoreov1alpha1.Component)
			if component.Spec.Owner.ProjectName == "" {
				return nil
			}
			return []string{component.Spec.Owner.ProjectName}
		}); err != nil {
		return fmt.Errorf("failed to setup Component owner project index: %w", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Project{},
		IndexKeyProjectDeploymentPipelineRef, func(obj client.Object) []string {
			project := obj.(*openchoreov1alpha1.Project)
			if project.Spec.DeploymentPipelineRef == "" {
				return nil
			}
			return []string{project.Spec.DeploymentPipelineRef}
		}); err != nil {
		return fmt.Errorf("failed to setup Project deploymentPipelineRef index: %w", err)
	}

	return nil
}

// HierarchyWatchHandler is a function that creates a watch handler for a specific hierarchy.
// It can be used to watch from parent object for child object updates.
// The hierarchyFunc should return the target object that is being watched given the source object.
func HierarchyWatchHandler[From client.Object, To client.Object](
	c client.Client,
	hierarchyFunc HierarchyFunc[To],
) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		fromObj, ok := obj.(From)
		if !ok {
			return nil
		}

		toObj, err := hierarchyFunc(ctx, c, fromObj)
		if err != nil {
			return nil
		}

		return []reconcile.Request{{
			NamespacedName: client.ObjectKey{
				Namespace: toObj.GetNamespace(),
				Name:      toObj.GetName(),
			},
		}}
	}
}
