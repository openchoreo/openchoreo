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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// deployableArtifactRefIndexKey is the index key for the deployable artifact reference
const deployableArtifactRefIndexKey = ".spec.deploymentArtifactRef"

// setupDeployableArtifactRefIndex creates a field index for the deployable artifact reference in the deployments.
func (r *Reconciler) setupDeployableArtifactRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.Deployment{},
		deployableArtifactRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to the appropriate type
			deployment, ok := obj.(*choreov1.Deployment)
			if !ok {
				return nil
			}
			// Return the value of the deploymentArtifactRef field
			return []string{deployment.Spec.DeploymentArtifactRef}
		},
	)
}
