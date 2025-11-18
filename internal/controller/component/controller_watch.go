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
	// traitsIndex is the field index name for traits used
	traitsIndex = "spec.traits"
	// workloadOwnerIndex is the field index name for workload owner references
	workloadOwnerIndex = "spec.owner"
	// releaseBindingIndex is the field index name for ReleaseBinding owner fields and environment
	releaseBindingIndex = "spec.owner.projectName/spec.owner.componentName/spec.environment"
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

// setupTraitsRefIndex sets up the field index for trait references
func (r *Reconciler) setupTraitsRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		traitsIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			traitNames := []string{}
			for _, trait := range comp.Spec.Traits {
				traitNames = append(traitNames, trait.Name)
			}
			return traitNames
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

// setupReleaseBindingIndex registers an index for ReleaseBinding by owner fields and environment.
func (r *Reconciler) setupReleaseBindingIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		releaseBindingIndex, func(obj client.Object) []string {
			releaseBinding := obj.(*openchoreov1alpha1.ReleaseBinding)
			project := releaseBinding.Spec.Owner.ProjectName
			component := releaseBinding.Spec.Owner.ComponentName
			environment := releaseBinding.Spec.Environment
			ownerKey := fmt.Sprintf("%s/%s/%s", project, component, environment)
			return []string{ownerKey}
		})
}

// listComponentsForComponentType returns reconcile requests for all Components using this ComponentType
func (r *Reconciler) listComponentsForComponentType(ctx context.Context, obj client.Object) []reconcile.Request {
	ct := obj.(*openchoreov1alpha1.ComponentType)

	// Find all components using this ComponentType
	// ComponentType format: {workloadType}/{ctName}
	componentType := fmt.Sprintf("%s/%s", ct.Spec.WorkloadType, ct.Name)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{componentTypeIndex: componentType}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for ComponentType", "componentType", ct.Name)
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

// listComponentsUsingTrait returns reconcile requests for all Components using this Trait
func (r *Reconciler) listComponentsUsingTrait(ctx context.Context, obj client.Object) []reconcile.Request {
	trait := obj.(*openchoreov1alpha1.Trait)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{traitsIndex: trait.Name}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for Trait", "trait", trait.Name)
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
