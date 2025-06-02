// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// dataPlaneRefIndexKey is the field index key in the environment that
	// points to a data plane reference.
	dataPlaneRefIndexKey = "spec.dataPlaneRef"
)

// setupDataPlaneRefIndex creates a field index for the data plane reference in environments.
func (r *Reconciler) setupDataPlaneRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.Environment{},
		dataPlaneRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to an Environment
			env := obj.(*choreov1.Environment)
			// Return the data plane reference
			return []string{env.Spec.DataPlaneRef}
		},
	)
}

// find and return all endpoints that belong to a dataplane
func (r *Reconciler) listEndpointsForDataplane(ctx context.Context, obj client.Object) []reconcile.Request {
	dp, ok := obj.(*choreov1.DataPlane)
	if !ok {
		return nil
	}

	envList := &choreov1.EnvironmentList{}
	if err := r.List(
		ctx,
		envList,
		client.MatchingFields{
			dataPlaneRefIndexKey: dp.Name,
		},
	); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(envList.Items))
	for _, env := range envList.Items {
		epList := &choreov1.EndpointList{}
		if err := r.List(ctx, epList, client.MatchingLabels{
			labels.LabelKeyEnvironmentName: env.Name,
		}); err != nil {
			return nil
		}
		for _, ep := range epList.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      ep.Name,
					Namespace: ep.Namespace,
				},
			})
		}
	}
	return requests
}

// find and return all endpoints that belong to a dataplane
func (r *Reconciler) listEndpointsForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	env, ok := obj.(*choreov1.Environment)
	if !ok {
		return nil
	}

	epList := &choreov1.EndpointList{}
	if err := r.List(
		ctx,
		epList,
		client.MatchingLabels{
			labels.LabelKeyEnvironmentName: env.Name,
		},
	); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(epList.Items))
	for i, ep := range epList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      ep.Name,
				Namespace: ep.Namespace,
			},
		}
	}

	return requests
}
