// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

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
