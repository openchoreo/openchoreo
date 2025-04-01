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
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
)

// All the watch handlers for the component controller are defined in this file.

// listDeploymentTrackForChild is a watch handler that lists the deployment tracks
// that refers to a given build, deployable artifact or deployment and makes a reconcile.Request for reconciliation.
func (r *Reconciler) listDeploymentTrackForBuild(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	logger.Info("In watch for Build in DT")

	build, ok := obj.(*choreov1.Build)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a Build from the Watch
		return nil
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, build)
	if err != nil {
		if errors.Is(err, &controller.HierarchyNotFoundError{}) {
			logger.Error(err, "Hierarchy not found for build", "build", build)
			return nil
		}

		// Log the error and return
		logger.Error(err, "Failed to get deployment track for build", "build", build)
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

func (r *Reconciler) listDeploymentTrackForDeployableArtifact(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	logger.Info("In watch for DeployableArtifact in DT")

	deployableArtifact, ok := obj.(*choreov1.DeployableArtifact)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a Deployable Artifact from the Watch
		return nil
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, deployableArtifact)
	if err != nil {
		if errors.Is(err, &controller.HierarchyNotFoundError{}) {
			logger.Error(err, "Hierarchy not found for deployableArtifact", "deployableArtifact", deployableArtifact)
			return nil
		}

		// Log the error and return
		logger.Error(err, "Failed to get deployment track for deployableArtifact", "deployableArtifact ", deployableArtifact)
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

func (r *Reconciler) listDeploymentTrackForDeployments(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	logger.Info("In watch for Deployment in DT")

	deployment, ok := obj.(*choreov1.Deployment)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a Deployment from the Watch
		return nil
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, deployment)
	if err != nil {
		if errors.Is(err, &controller.HierarchyNotFoundError{}) {
			logger.Error(err, "Hierarchy not found for deployment", "deployment", deployment)
			return nil
		}

		// Log the error and return
		logger.Error(err, "Failed to get deployment track for deployment", "deployment", deployment)
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
