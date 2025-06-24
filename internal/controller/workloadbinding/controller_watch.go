// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workloadbinding

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

const (
	// workloadClassNameIndex is the field index name for workload class reference
	workloadClassNameIndex = "spec.workloadSpec.workloadTemplateSpec.className"
)

// setupWorkloadClassRefIndex sets up the field index for workload class references
func (r *Reconciler) setupWorkloadClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.WorkloadBinding{}, workloadClassNameIndex, func(rawObj client.Object) []string {
		workloadBinding := rawObj.(*choreov1.WorkloadBinding)
		if workloadBinding.Spec.WorkloadSpec.WorkloadTemplateSpec.ClassName == "" {
			return nil
		}
		return []string{workloadBinding.Spec.WorkloadSpec.WorkloadTemplateSpec.ClassName}
	})
}

// listWorkloadBindingsForWorkloadClass finds all WorkloadBindings that reference the given WorkloadClass
func (r *Reconciler) listWorkloadBindingsForWorkloadClass(ctx context.Context, obj client.Object) []reconcile.Request {
	workloadClass, ok := obj.(*choreov1.WorkloadClass)
	if !ok {
		return nil
	}

	workloadBindingList := &choreov1.WorkloadBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(workloadClass.Namespace),
		client.MatchingFields{workloadClassNameIndex: workloadClass.Name},
	}

	if err := r.List(ctx, workloadBindingList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(workloadBindingList.Items))
	for i, workloadBinding := range workloadBindingList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: workloadBinding.Namespace,
				Name:      workloadBinding.Name,
			},
		}
	}
	return requests
}
