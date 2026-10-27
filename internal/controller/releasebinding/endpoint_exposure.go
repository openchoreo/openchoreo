// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// endpointVisibility returns each workload endpoint's effective visibility:
// the deprecated workload field together with the visibilities of every
// Endpoint in the project that routes to it. A component's network policy
// admits traffic from these scopes, so an Endpoint can reach a workload
// endpoint whose workload declares no visibility of its own.
//
// It covers every Endpoint, deployed to this environment or not: opening the
// policy only lets the scope's sources in, and keying it on deployment would
// make the route work only after a second reconcile.
func (r *Reconciler) endpointVisibility(
	ctx context.Context,
	namespace, projectName, componentName string,
	endpoints map[string]openchoreov1alpha1.WorkloadEndpoint,
) (map[string][]openchoreov1alpha1.EndpointVisibility, error) {
	result := make(map[string][]openchoreov1alpha1.EndpointVisibility, len(endpoints))
	for name, ep := range endpoints {
		result[name] = slices.Clone(ep.Visibility) //nolint:staticcheck // deprecated field still supported for backward compatibility
	}
	if len(endpoints) == 0 {
		return result, nil
	}

	var list openchoreov1alpha1.EndpointList
	if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list Endpoints: %w", err)
	}
	for i := range list.Items {
		ep := &list.Items[i]
		if ep.Spec.Owner.ProjectName != projectName || !ep.DeletionTimestamp.IsZero() {
			continue
		}
		for _, ref := range ep.Spec.BackendRefs {
			if ref.Kind != openchoreov1alpha1.EndpointBackendKindComponent || ref.Component == nil ||
				ref.Component.Name != componentName {
				continue
			}
			if _, ok := endpoints[ref.Component.Endpoint]; !ok {
				continue
			}
			for _, v := range ep.Spec.Visibility {
				if !slices.Contains(result[ref.Component.Endpoint], v) {
					result[ref.Component.Endpoint] = append(result[ref.Component.Endpoint], v)
				}
			}
		}
	}
	for name := range result {
		slices.Sort(result[name])
	}
	return result, nil
}

// findReleaseBindingsForEndpoint enqueues the ReleaseBindings of every
// component an Endpoint routes to, so their network policies follow the
// Endpoint's visibility. Update events map both the old and the new object,
// which also releases components the Endpoint no longer routes to.
func (r *Reconciler) findReleaseBindingsForEndpoint(ctx context.Context, obj client.Object) []ctrl.Request {
	ep, ok := obj.(*openchoreov1alpha1.Endpoint)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var requests []ctrl.Request
	for _, ref := range ep.Spec.BackendRefs {
		if ref.Kind != openchoreov1alpha1.EndpointBackendKindComponent || ref.Component == nil ||
			seen[ref.Component.Name] {
			continue
		}
		seen[ref.Component.Name] = true

		var bindings openchoreov1alpha1.ReleaseBindingList
		if err := r.List(ctx, &bindings,
			client.InNamespace(ep.Namespace),
			client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: ref.Component.Name}); err != nil {
			continue
		}
		for _, binding := range bindings.Items {
			if binding.Spec.Owner.ProjectName != ep.Spec.Owner.ProjectName {
				continue
			}
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace},
			})
		}
	}
	return requests
}
