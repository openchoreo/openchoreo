/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
