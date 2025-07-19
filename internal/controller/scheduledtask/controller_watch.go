// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// workloadNameIndex is the field index name for workload reference
	workloadNameIndex = "spec.workloadName"
)

// setupWorkloadRefIndex sets up the field index for workload references
func (r *Reconciler) setupWorkloadRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ScheduledTask{}, workloadNameIndex, func(rawObj client.Object) []string {
		scheduledTask := rawObj.(*openchoreov1alpha1.ScheduledTask)
		if scheduledTask.Spec.WorkloadName == "" {
			return nil
		}
		return []string{scheduledTask.Spec.WorkloadName}
	})
}

// listScheduledTasksForWorkload finds all ScheduledTasks that reference the given Workload
func (r *Reconciler) listScheduledTasksForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	workload, ok := obj.(*openchoreov1alpha1.Workload)
	if !ok {
		return nil
	}

	scheduledTaskList := &openchoreov1alpha1.ScheduledTaskList{}
	listOpts := []client.ListOption{
		client.InNamespace(workload.Namespace),
		client.MatchingFields{workloadNameIndex: workload.Name},
	}

	if err := r.List(ctx, scheduledTaskList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(scheduledTaskList.Items))
	for i, scheduledTask := range scheduledTaskList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: scheduledTask.Namespace,
				Name:      scheduledTask.Name,
			},
		}
	}
	return requests
}
