// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// resourceTypeRefIndex indexes Resource by its (Cluster)ResourceType reference.
	// Index key format: "{kind}:{name}" — e.g. "ResourceType:mysql",
	// "ClusterResourceType:shared-cache". Mirrors Component's componentTypeIndex.
	resourceTypeRefIndex = "spec.type"
)

// indexResourceTypeRef extracts the (Cluster)ResourceType reference key from a
// Resource. Exposed as a package-level value so tests can pass it to
// fake.NewClientBuilder().WithIndex.
func indexResourceTypeRef(obj client.Object) []string {
	res := obj.(*openchoreov1alpha1.Resource)
	kind := resolvedKind(res.Spec.Type.Kind)
	return []string{string(kind) + ":" + res.Spec.Type.Name}
}

// setupResourceTypeRefIndex registers the field index used by the watch mappers.
func (r *Reconciler) setupResourceTypeRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Resource{},
		resourceTypeRefIndex, indexResourceTypeRef)
}

// listResourcesForResourceType returns reconcile requests for Resources in the
// same namespace as the given ResourceType that reference it via
// spec.type.{Kind=ResourceType, Name=rt.Name}.
func (r *Reconciler) listResourcesForResourceType(ctx context.Context, obj client.Object) []reconcile.Request {
	rt := obj.(*openchoreov1alpha1.ResourceType)
	indexKey := string(openchoreov1alpha1.ResourceTypeRefKindResourceType) + ":" + rt.Name
	return r.requestsForIndexKey(ctx, indexKey, client.InNamespace(rt.Namespace))
}

// listResourcesForClusterResourceType returns reconcile requests for Resources
// across all namespaces that reference the given ClusterResourceType via
// spec.type.{Kind=ClusterResourceType, Name=crt.Name}.
func (r *Reconciler) listResourcesForClusterResourceType(ctx context.Context, obj client.Object) []reconcile.Request {
	crt := obj.(*openchoreov1alpha1.ClusterResourceType)
	indexKey := string(openchoreov1alpha1.ResourceTypeRefKindClusterResourceType) + ":" + crt.Name
	return r.requestsForIndexKey(ctx, indexKey)
}

// requestsForIndexKey lists Resources by the resourceTypeRefIndex and converts
// them to reconcile.Requests. Extra ListOptions (e.g. client.InNamespace) scope
// the lookup further.
func (r *Reconciler) requestsForIndexKey(ctx context.Context, indexKey string, opts ...client.ListOption) []reconcile.Request {
	listOpts := append([]client.ListOption{client.MatchingFields{resourceTypeRefIndex: indexKey}}, opts...)

	var resources openchoreov1alpha1.ResourceList
	if err := r.List(ctx, &resources, listOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Resources by type ref", "indexKey", indexKey)
		return nil
	}

	requests := make([]reconcile.Request, len(resources.Items))
	for i, res := range resources.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{Name: res.Name, Namespace: res.Namespace},
		}
	}
	return requests
}
