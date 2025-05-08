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

package dataplane

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// dataplaneRefIndexKey is the index key for the dataplane reference
const dataplaneRefIndexKey = ".spec.dataPlaneRef"

// setupDataPlaneRefIndex creates a field index for the dataplane reference in the environmentsß.
func (r *Reconciler) setupDataPlaneRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.Environment{},
		dataplaneRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to the appropriate type
			environment, ok := obj.(*choreov1.Environment)
			if !ok {
				return nil
			}
			// Return the value of the dataPlaneRef field
			return []string{environment.Spec.DataPlaneRef}
		},
	)
}

func (r *Reconciler) GetDataPlaneForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	environment, ok := obj.(*choreov1.Environment)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be an Environment from the Watch
		return nil
	}

	dataplane, err := controller.GetDataplaneOfEnv(ctx, r.Client, environment)
	if err != nil {
		// If the dataplane is not found, return an empty request
		if errors.IsNotFound(err) {
			return nil
		}
		// If there is an error other than not found, log it and return an empty request
		log.FromContext(ctx).Error(err, "Failed to get dataplane for environment", "environment", environment.Name)
		return nil
	}

	// If the dataplane is not found, return an empty request
	if dataplane == nil {
		return nil
	}

	// Create a request for the dataplane
	requests := []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      dataplane.Name,
				Namespace: dataplane.Namespace,
			},
		},
	}

	return requests
}
