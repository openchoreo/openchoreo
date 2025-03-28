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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/labels"
)

// All the watch handlers for the component controller are defined in this file.

// listComponentsForDeploymentTrack is a watch handler that lists the components
// that refers to a given deployment track and makes a reconcile.Request for reconciliation.
func (r *Reconciler) listComponentsForDeploymentTrack(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	deploymentTrack, ok := obj.(*choreov1.DeploymentTrack)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a DeploymentTrack from the Watch
		return nil
	}

	// Gets the component for the deployment track
	component, err := getComponentForDeploymentTrack(ctx, r.Client, deploymentTrack)
	if err != nil {
		// Log the error and return
		logger.Error(err, "Failed to get component for deployment track", "deploymentTrack", deploymentTrack)
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

func getComponentForDeploymentTrack(ctx context.Context, c client.Client, obj client.Object) (*choreov1.Component, error) {
	componentList := &choreov1.ComponentList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: controller.GetOrganizationName(obj),
			labels.LabelKeyProjectName:      controller.GetProjectName(obj),
			labels.LabelKeyComponentName:    controller.GetComponentName(obj),
		},
	}

	if err := c.List(ctx, componentList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	if len(componentList.Items) > 0 {
		return &componentList.Items[0], nil
	}

	return nil, nil
}
