/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// HierarchyWatchHandler is a function that creates a watch handler for a specific hierarchy.
// It can be used to watch from parent object for child object updates.
// The hierarchyFunc should return the target object that is being watched given the source object.
func HierarchyWatchHandler[From client.Object, To client.Object](
	c client.Client,
	hierarchyFunc HierarchyFunc[To],
) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		fromObj, ok := obj.(From)
		if !ok {
			return nil
		}

		toObj, err := hierarchyFunc(ctx, c, fromObj)
		if err != nil {
			return nil
		}

		return []reconcile.Request{{
			NamespacedName: client.ObjectKey{
				Namespace: toObj.GetNamespace(),
				Name:      toObj.GetName(),
			},
		}}
	}
}
