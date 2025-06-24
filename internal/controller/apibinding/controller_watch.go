// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apibinding

// import (
//	"context"
//
//	ctrl "sigs.k8s.io/controller-runtime"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/reconcile"
//
//	choreov1 "github.com/openchoreo/openchoreo/api/v1"
// )

/*
const (
	// apiClassNameIndex is the field index name for API class reference
	// TODO: Update this when APIBinding spec is properly defined
	apiClassNameIndex = "spec.className"
)
*/

// setupAPIClassRefIndex sets up the field index for API class references
// TODO: Implement when APIBinding spec includes API class reference
/*
func (r *Reconciler) setupAPIClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.APIBinding{}, apiClassNameIndex, func(rawObj client.Object) []string {
		// apiBinding := rawObj.(*choreov1.APIBinding)
		// TODO: Return the actual API class name when spec is defined
		// if apiBinding.Spec.ClassName == "" {
		//     return nil
		// }
		// return []string{apiBinding.Spec.ClassName}
		return nil
	})
}
*/

// listAPIBindingsForAPIClass finds all APIBindings that reference the given APIClass
// TODO: Implement when APIBinding spec includes API class reference
/*
func (r *Reconciler) listAPIBindingsForAPIClass(ctx context.Context, obj client.Object) []reconcile.Request {
	apiClass, ok := obj.(*choreov1.APIClass)
	if !ok {
		return nil
	}

	apiBindingList := &choreov1.APIBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(apiClass.Namespace),
		client.MatchingFields{apiClassNameIndex: apiClass.Name},
	}

	if err := r.List(ctx, apiBindingList, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(apiBindingList.Items))
	for i, apiBinding := range apiBindingList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: apiBinding.Namespace,
				Name:      apiBinding.Name,
			},
		}
	}
	return requests
}
*/
