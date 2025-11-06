// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

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
	// componentTypeIndex is the field index name for componentType reference
	componentTypeIndex = "spec.componentType"
	// addonsIndex is the field index name for addons used
	addonsIndex = "spec.addons"
	// workloadOwnerIndex is the field index name for workload owner references
	workloadOwnerIndex = "spec.owner"
)

// setupComponentTypeRefIndex sets up the field index for componentType references
func (r *Reconciler) setupComponentTypeRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		componentTypeIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			if comp.Spec.ComponentType == "" {
				return []string{}
			}
			return []string{comp.Spec.ComponentType}
		})
}

// setupAddonsRefIndex sets up the field index for addon references
func (r *Reconciler) setupAddonsRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		addonsIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			addonNames := []string{}
			for _, addon := range comp.Spec.Addons {
				addonNames = append(addonNames, addon.Name)
			}
			return addonNames
		})
}

// setupWorkloadOwnerIndex sets up the field index for workload owner references
func (r *Reconciler) setupWorkloadOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Workload{},
		workloadOwnerIndex, func(obj client.Object) []string {
			workload := obj.(*openchoreov1alpha1.Workload)
			// Create a composite key: projectName/componentName
			ownerKey := fmt.Sprintf("%s/%s",
				workload.Spec.Owner.ProjectName,
				workload.Spec.Owner.ComponentName)
			return []string{ownerKey}
		})
}

// listComponentsForComponentType returns reconcile requests for all Components using this ComponentTypeDefinition
func (r *Reconciler) listComponentsForComponentType(ctx context.Context, obj client.Object) []reconcile.Request {
	ctd := obj.(*openchoreov1alpha1.ComponentTypeDefinition)

	// Find all components using this ComponentTypeDefinition
	// ComponentType format: {workloadType}/{ctdName}
	componentType := fmt.Sprintf("%s/%s", ctd.Spec.WorkloadType, ctd.Name)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{componentTypeIndex: componentType}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for ComponentTypeDefinition", "componentTypeDefinition", ctd.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsUsingAddon returns reconcile requests for all Components using this Addon
func (r *Reconciler) listComponentsUsingAddon(ctx context.Context, obj client.Object) []reconcile.Request {
	addon := obj.(*openchoreov1alpha1.Addon)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{addonsIndex: addon.Name}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for Addon", "addon", addon.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsForWorkload returns reconcile requests for the Component owning this Workload
func (r *Reconciler) listComponentsForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	workload := obj.(*openchoreov1alpha1.Workload)

	// Use the owner reference from workload spec to find the owning component
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      workload.Spec.Owner.ComponentName,
			Namespace: workload.Namespace,
		},
	}}
}
