// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

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
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Service{}, workloadNameIndex, func(rawObj client.Object) []string {
		service := rawObj.(*openchoreov1alpha1.Service)
		if service.Spec.WorkloadName == "" {
			return nil
		}
		return []string{service.Spec.WorkloadName}
	})
}

// listServicesForWorkload finds all Services that reference the given Workload
func (r *Reconciler) listServicesForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	workload, ok := obj.(*openchoreov1alpha1.Workload)
	if !ok {
		return nil
	}

	serviceList := &openchoreov1alpha1.ServiceList{}
	listOpts := []client.ListOption{
		client.InNamespace(workload.Namespace),
		client.MatchingFields{workloadNameIndex: workload.Name},
	}

	if err := r.List(ctx, serviceList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(serviceList.Items))
	for i, service := range serviceList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: service.Namespace,
				Name:      service.Name,
			},
		}
	}
	return requests
}
