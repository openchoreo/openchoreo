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

package component

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// All the watch handlers for the component controller are defined in this file.

// listComponentsForDeploymentTrack is a watch handler that lists the components
// that refers to a given deployment track and makes a reconcile.Request for reconciliation.
func (r *Reconciler) listComponentsForDeploymentTrack(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	logger.Info("In watch for DeploymentTrack in Component")

	deploymentTrack, ok := obj.(*choreov1.DeploymentTrack)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a DeploymentTrack from the Watch
		return nil
	}

	// Gets the component for the deployment track
	component, err := controller.GetComponent(ctx, r.Client, deploymentTrack)
	if err != nil {
		if errors.Is(err, &controller.HierarchyNotFoundError{}) {
			logger.Error(err, "Hierarchy not found for deploymentTrack", "deploymentTrack", deploymentTrack)
			return nil
		}

		// Log the error and return
		logger.Error(err, "Failed to get deployment track for deploymentTrack", "deploymentTrack ", deploymentTrack)
		return nil
	}

	if component == nil {
		return nil
	}

	requests := make([]reconcile.Request, 1)
	requests[0] = reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: component.Namespace,
			Name:      component.Name,
		},
	}

	// Enqueue the component if the deployment track is updated
	return requests
}
