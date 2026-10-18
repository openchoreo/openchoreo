// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// endpointTypeRefIndex indexes Endpoint by its (Cluster)EndpointType reference.
	// Index key format: "{kind}:{name}" — e.g. "EndpointType:http",
	// "ClusterEndpointType:gateway-route". Mirrors Resource's resourceTypeRefIndex.
	endpointTypeRefIndex = "spec.type"

	// backendComponentIndex indexes Endpoint by the components its Component
	// backends reference. Index key format: "{projectName}/{componentName}", the
	// same key a Workload's owner produces, so a Workload change maps straight
	// to the Endpoints routing to it.
	backendComponentIndex = "spec.backendRefs.component"
)

// indexEndpointTypeRef extracts the (Cluster)EndpointType reference key from an
// Endpoint.
func indexEndpointTypeRef(obj client.Object) []string {
	ep := obj.(*openchoreov1alpha1.Endpoint)
	kind := resolvedKind(ep.Spec.Type.Kind)
	return []string{string(kind) + ":" + ep.Spec.Type.Name}
}

// indexBackendComponents extracts one "{projectName}/{componentName}" key per
// distinct component an Endpoint routes to. Backends resolve in the Endpoint's
// own project.
func indexBackendComponents(obj client.Object) []string {
	ep := obj.(*openchoreov1alpha1.Endpoint)
	seen := map[string]struct{}{}
	var keys []string
	for _, ref := range ep.Spec.BackendRefs {
		if ref.Component == nil {
			continue
		}
		key := backendComponentKey(ep.Spec.Owner.ProjectName, ref.Component.Name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func backendComponentKey(projectName, componentName string) string {
	return projectName + "/" + componentName
}

// setupEndpointTypeRefIndex registers the field index used by the type watch mappers.
func (r *Reconciler) setupEndpointTypeRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Endpoint{},
		endpointTypeRefIndex, indexEndpointTypeRef)
}

// setupBackendComponentIndex registers the field index used by the Workload watch mapper.
func (r *Reconciler) setupBackendComponentIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Endpoint{},
		backendComponentIndex, indexBackendComponents)
}

// listEndpointsForEndpointType returns reconcile requests for Endpoints in the
// same namespace as the given EndpointType that reference it.
func (r *Reconciler) listEndpointsForEndpointType(ctx context.Context, obj client.Object) []reconcile.Request {
	et := obj.(*openchoreov1alpha1.EndpointType)
	indexKey := string(openchoreov1alpha1.EndpointTypeRefKindEndpointType) + ":" + et.Name
	return r.requestsForIndex(ctx, endpointTypeRefIndex, indexKey, client.InNamespace(et.Namespace))
}

// listEndpointsForClusterEndpointType returns reconcile requests for Endpoints
// across all namespaces that reference the given ClusterEndpointType.
func (r *Reconciler) listEndpointsForClusterEndpointType(ctx context.Context, obj client.Object) []reconcile.Request {
	cet := obj.(*openchoreov1alpha1.ClusterEndpointType)
	indexKey := string(openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType) + ":" + cet.Name
	return r.requestsForIndex(ctx, endpointTypeRefIndex, indexKey)
}

// listEndpointsForWorkload returns reconcile requests for Endpoints in the
// Workload's namespace that route to the Workload's component, so adding or
// removing a workload endpoint re-validates the Endpoints that name it.
func (r *Reconciler) listEndpointsForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	w := obj.(*openchoreov1alpha1.Workload)
	indexKey := backendComponentKey(w.Spec.Owner.ProjectName, w.Spec.Owner.ComponentName)
	return r.requestsForIndex(ctx, backendComponentIndex, indexKey, client.InNamespace(w.Namespace))
}

// requestsForIndex lists Endpoints by the given field index and converts them
// to reconcile.Requests. Extra ListOptions (e.g. client.InNamespace) scope the
// lookup further.
func (r *Reconciler) requestsForIndex(ctx context.Context, index, key string, opts ...client.ListOption) []reconcile.Request {
	listOpts := append([]client.ListOption{client.MatchingFields{index: key}}, opts...)

	var endpoints openchoreov1alpha1.EndpointList
	if err := r.List(ctx, &endpoints, listOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Endpoints by index", "index", index, "key", key)
		return nil
	}

	requests := make([]reconcile.Request, len(endpoints.Items))
	for i, ep := range endpoints.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ep.Name, Namespace: ep.Namespace},
		}
	}
	return requests
}
