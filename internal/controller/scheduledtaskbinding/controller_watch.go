// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtaskbinding

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

const (
	// scheduledTaskClassNameIndex is the field index name for scheduled task class reference
	scheduledTaskClassNameIndex = "spec.className"
)

// setupScheduledTaskClassRefIndex sets up the field index for scheduled task class references
func (r *Reconciler) setupScheduledTaskClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.ScheduledTaskBinding{}, scheduledTaskClassNameIndex, func(rawObj client.Object) []string {
		scheduledTaskBinding := rawObj.(*choreov1.ScheduledTaskBinding)
		if scheduledTaskBinding.Spec.ClassName == "" {
			return nil
		}
		return []string{scheduledTaskBinding.Spec.ClassName}
	})
}

// listScheduledTaskBindingsForScheduledTaskClass finds all ScheduledTaskBindings that reference the given ScheduledTaskClass
func (r *Reconciler) listScheduledTaskBindingsForScheduledTaskClass(ctx context.Context, obj client.Object) []reconcile.Request {
	scheduledTaskClass, ok := obj.(*choreov1.ScheduledTaskClass)
	if !ok {
		return nil
	}

	scheduledTaskBindingList := &choreov1.ScheduledTaskBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(scheduledTaskClass.Namespace),
		client.MatchingFields{scheduledTaskClassNameIndex: scheduledTaskClass.Name},
	}

	if err := r.List(ctx, scheduledTaskBindingList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(scheduledTaskBindingList.Items))
	for i, scheduledTaskBinding := range scheduledTaskBindingList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: scheduledTaskBinding.Namespace,
				Name:      scheduledTaskBinding.Name,
			},
		}
	}
	return requests
}
