// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// findEnvironmentsForDeploymentPipeline maps a DeploymentPipeline change to the Environments
// it references in its PromotionPaths. This ensures that when a DeploymentPipeline is deleted
// or its environment references change, the affected Environments are reconciled so that
// finalization can proceed once the reference is removed.
//
// When an environment reference is removed from a pipeline, the removed environment is no longer
// in the current PromotionPaths. To handle this, we also find environments in the namespace that
// are pending deletion and may have been unblocked by this pipeline change.
func (r *Reconciler) findEnvironmentsForDeploymentPipeline(ctx context.Context, obj client.Object) []reconcile.Request {
	pipeline, ok := obj.(*openchoreov1alpha1.DeploymentPipeline)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx).V(1)

	// Collect all environments currently referenced by this pipeline.
	envNames := make(map[string]struct{})
	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef != "" {
			envNames[path.SourceEnvironmentRef] = struct{}{}
		}
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name != "" {
				envNames[target.Name] = struct{}{}
			}
		}
	}

	// Also find environments that are pending deletion in this namespace.
	// These may have been blocked by this pipeline and could now proceed.
	var envList openchoreov1alpha1.EnvironmentList
	if err := r.List(ctx, &envList,
		client.InNamespace(pipeline.Namespace),
	); err != nil {
		logger.Error(err, "Failed to list environments for DeploymentPipeline watch",
			"deploymentPipeline", pipeline.Name)
	} else {
		for i := range envList.Items {
			if !envList.Items[i].DeletionTimestamp.IsZero() {
				envNames[envList.Items[i].Name] = struct{}{}
			}
		}
	}

	if len(envNames) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(envNames))
	for name := range envNames {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: pipeline.Namespace,
				Name:      name,
			},
		})
	}

	logger.Info("DeploymentPipeline change detected, enqueueing Environments",
		"deploymentPipeline", pipeline.Name, "environmentCount", len(requests))

	return requests
}
