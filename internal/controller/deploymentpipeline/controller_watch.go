// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// findDeploymentPipelineForProject maps a Project change to the DeploymentPipeline it references.
// This ensures the DeploymentPipeline is reconciled when a referring Project is created, updated, or deleted,
// allowing finalization to proceed once the reference is removed.
func (r *Reconciler) findDeploymentPipelineForProject(_ context.Context, obj client.Object) []reconcile.Request {
	project, ok := obj.(*openchoreov1alpha1.Project)
	if !ok {
		return nil
	}

	pipelineRef := project.Spec.DeploymentPipelineRef
	if pipelineRef.Name == "" {
		return nil
	}

	log.Log.V(1).Info("Project change detected, enqueueing DeploymentPipeline",
		"project", project.Name, "deploymentPipeline", pipelineRef.Name)

	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: project.Namespace,
				Name:      pipelineRef.Name,
			},
		},
	}
}
