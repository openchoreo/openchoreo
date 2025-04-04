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

package watchfilters

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LabelFilter is an interface for creating a filter to
type LabelFilter interface {
	// IsRelevant Determines whether an object is relevant based on labels and owner references.
	IsRelevant(obj client.Object) bool

	// GetReconcileRequests Extracts reconcile requests from a relevant object.
	GetReconcileRequests(ctx context.Context, obj client.Object) []reconcile.Request
}

// BuildPredicates constructs predicates using the LabelWatchFilter interface.
func BuildPredicates(filter LabelFilter) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return filter.IsRelevant(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filter.IsRelevant(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return filter.IsRelevant(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return filter.IsRelevant(e.Object)
		},
	}
}

// BuildMapFunc constructs a handler.MapFunc using the LabelWatchFilter interface.
func BuildMapFunc(filter LabelFilter) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		return filter.GetReconcileRequests(ctx, obj)
	}
}
