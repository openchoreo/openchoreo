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

package deploymenttrack

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

// listDeploymentTrackForChild is a watch handler that lists the deployment tracks
// that refers to a given build, deployable artifact or deployment and makes a reconcile.Request for reconciliation.
func (r *Reconciler) listDeploymentTrackForChild(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	build, ok := obj.(*choreov1.Build)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a Build from the Watch
		return nil
	}

	deploymentTrack, err := getDeploymentTrackForObject(ctx, r.Client, build)
	if err != nil {
		// Log the error and return
		logger.Error(err, "Failed to get component for deployment track", "deploymentTrack", build)
		return nil
	}

	if deploymentTrack == nil {
		return nil
	}

	requests := make([]reconcile.Request, 1)
	requests[0] = reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: deploymentTrack.Namespace,
			Name:      deploymentTrack.Name,
		},
	}

	// Enqueue the deploymentTrack if the build is updated
	return requests
}

func getDeploymentTrackForObject(ctx context.Context, c client.Client, obj client.Object) (*choreov1.DeploymentTrack, error) {
	deploymentTrackList := &choreov1.DeploymentTrackList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName:    controller.GetOrganizationName(obj),
			labels.LabelKeyProjectName:         controller.GetProjectName(obj),
			labels.LabelKeyComponentName:       controller.GetComponentName(obj),
			labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(obj),
		},
	}

	if err := c.List(ctx, deploymentTrackList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list deploymentTracks: %w", err)
	}

	if len(deploymentTrackList.Items) > 0 {
		return &deploymentTrackList.Items[0], nil
	}

	return nil, nil
}
