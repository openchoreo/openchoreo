// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointv2

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

const (
	// endpointClassNameIndex is the field index name for endpoint class reference
	endpointClassNameIndex = "spec.className"
)

// setupEndpointClassRefIndex sets up the field index for endpoint class references
func (r *Reconciler) setupEndpointClassRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &choreov1.EndpointV2{}, endpointClassNameIndex, func(rawObj client.Object) []string {
		endpointv2 := rawObj.(*choreov1.EndpointV2)
		if endpointv2.Spec.ClassName == "" {
			return nil
		}
		return []string{endpointv2.Spec.ClassName}
	})
}

// listEndpointV2sForEndpointClass finds all EndpointV2s that reference the given EndpointClass
func (r *Reconciler) listEndpointV2sForEndpointClass(ctx context.Context, obj client.Object) []reconcile.Request {
	endpointClass, ok := obj.(*choreov1.EndpointClass)
	if !ok {
		return nil
	}

	endpointV2List := &choreov1.EndpointV2List{}
	listOpts := []client.ListOption{
		client.InNamespace(endpointClass.Namespace),
		client.MatchingFields{endpointClassNameIndex: endpointClass.Name},
	}

	if err := r.List(ctx, endpointV2List, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(endpointV2List.Items))
	for i, endpointv2 := range endpointV2List.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: endpointv2.Namespace,
				Name:      endpointv2.Name,
			},
		}
	}
	return requests
}
