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
