// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package servicebinding

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

const (
	// serviceClassNameIndex is the field index name for service class reference
	serviceClassNameIndex = "spec.className"
)

// setupServiceClassRefIndex sets up the field index for service class references
func (r *Reconciler) setupServiceClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.ServiceBinding{}, serviceClassNameIndex, func(rawObj client.Object) []string {
		serviceBinding := rawObj.(*choreov1.ServiceBinding)
		if serviceBinding.Spec.ClassName == "" {
			return nil
		}
		return []string{serviceBinding.Spec.ClassName}
	})
}

// listServiceBindingsForServiceClass finds all ServiceBindings that reference the given ServiceClass
func (r *Reconciler) listServiceBindingsForServiceClass(ctx context.Context, obj client.Object) []reconcile.Request {
	serviceClass, ok := obj.(*choreov1.ServiceClass)
	if !ok {
		return nil
	}

	serviceBindingList := &choreov1.ServiceBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(serviceClass.Namespace),
		client.MatchingFields{serviceClassNameIndex: serviceClass.Name},
	}

	if err := r.List(ctx, serviceBindingList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(serviceBindingList.Items))
	for i, serviceBinding := range serviceBindingList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: serviceBinding.Namespace,
				Name:      serviceBinding.Name,
			},
		}
	}
	return requests
}
