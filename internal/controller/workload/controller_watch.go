// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

const (
	// workloadClassNameIndex is the field index name for workload class reference
	workloadClassNameIndex = "spec.workloadTemplateSpec.className"
)

// setupWorkloadClassRefIndex sets up the field index for workload class references
func (r *Reconciler) setupWorkloadClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.Workload{}, workloadClassNameIndex, func(rawObj client.Object) []string {
		workload := rawObj.(*choreov1.Workload)
		if workload.Spec.WorkloadTemplateSpec.ClassName == "" {
			return nil
		}
		return []string{workload.Spec.WorkloadTemplateSpec.ClassName}
	})
}

// listWorkloadsForWorkloadClass finds all Workloads that reference the given WorkloadClass
func (r *Reconciler) listWorkloadsForWorkloadClass(ctx context.Context, obj client.Object) []reconcile.Request {
	workloadClass, ok := obj.(*choreov1.WorkloadClass)
	if !ok {
		return nil
	}

	workloadList := &choreov1.WorkloadList{}
	listOpts := []client.ListOption{
		client.InNamespace(workloadClass.Namespace),
		client.MatchingFields{workloadClassNameIndex: workloadClass.Name},
	}

	if err := r.List(ctx, workloadList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(workloadList.Items))
	for i, workload := range workloadList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: workload.Namespace,
				Name:      workload.Name,
			},
		}
	}
	return requests
}
