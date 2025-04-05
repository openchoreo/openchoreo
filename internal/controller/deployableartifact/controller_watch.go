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

package deployableartifact

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// All the watch handlers for the deployable artifact controller are defined in this file.

// listDeployableArtifactForDeployment is a watch handler that lists the deployable artifacts
// that refer to a given deployment and makes a reconcile.Request for reconciliation.
// It is used to trigger reconciliation when the deployment changes.
func (r *Reconciler) listDeployableArtifactForDeployment(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	deployment, ok := obj.(*choreov1.Deployment)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a Deployment from the Watch
		return nil
	}

	deployableArtifactName := deployment.Spec.DeploymentArtifactRef
	if deployableArtifactName == "" {
		logger.Info("Deployment does not have a deploymentArtifactRef, skipping reconciliation")
		return nil
	}

	// Fetch the deployable artifact instance
	deployableArtifact := &choreov1.DeployableArtifact{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      deployableArtifactName,
	}, deployableArtifact); err != nil {
		logger.Error(err, "Failed to get deployable artifact for deployment", "deployment", deployment)
		return nil
	}

	requests := make([]reconcile.Request, 1)
	requests[0] = reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: deployableArtifact.Namespace,
			Name:      deployableArtifact.Name,
		},
	}

	// Enqueue the deployable artifact if the deployment is updated
	return requests
}
