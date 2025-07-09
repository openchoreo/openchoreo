// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplicationbinding

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// webApplicationClassNameIndex is the field index name for web application class reference
	webApplicationClassNameIndex = "spec.className"
)

// setupWebApplicationClassRefIndex sets up the field index for web application class references
func (r *Reconciler) setupWebApplicationClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.WebApplicationBinding{}, webApplicationClassNameIndex, func(rawObj client.Object) []string {
		webApplicationBinding := rawObj.(*openchoreov1alpha1.WebApplicationBinding)
		if webApplicationBinding.Spec.ClassName == "" {
			return nil
		}
		return []string{webApplicationBinding.Spec.ClassName}
	})
}

// listWebApplicationBindingsForWebApplicationClass finds all WebApplicationBindings that reference the given WebApplicationClass
func (r *Reconciler) listWebApplicationBindingsForWebApplicationClass(ctx context.Context, obj client.Object) []reconcile.Request {
	webApplicationClass, ok := obj.(*openchoreov1alpha1.WebApplicationClass)
	if !ok {
		return nil
	}

	webApplicationBindingList := &openchoreov1alpha1.WebApplicationBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(webApplicationClass.Namespace),
		client.MatchingFields{webApplicationClassNameIndex: webApplicationClass.Name},
	}

	if err := r.List(ctx, webApplicationBindingList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(webApplicationBindingList.Items))
	for i, webApplicationBinding := range webApplicationBindingList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: webApplicationBinding.Namespace,
				Name:      webApplicationBinding.Name,
			},
		}
	}
	return requests
}
