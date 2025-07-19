// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplication

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
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.WebApplication{}, workloadNameIndex, func(rawObj client.Object) []string {
		webApplication := rawObj.(*openchoreov1alpha1.WebApplication)
		if webApplication.Spec.WorkloadName == "" {
			return nil
		}
		return []string{webApplication.Spec.WorkloadName}
	})
}

// listWebApplicationsForWorkload finds all WebApplications that reference the given Workload
func (r *Reconciler) listWebApplicationsForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	workload, ok := obj.(*openchoreov1alpha1.Workload)
	if !ok {
		return nil
	}

	webApplicationList := &openchoreov1alpha1.WebApplicationList{}
	listOpts := []client.ListOption{
		client.InNamespace(workload.Namespace),
		client.MatchingFields{workloadNameIndex: workload.Name},
	}

	if err := r.List(ctx, webApplicationList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(webApplicationList.Items))
	for i, webApplication := range webApplicationList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: webApplication.Namespace,
				Name:      webApplication.Name,
			},
		}
	}
	return requests
}
