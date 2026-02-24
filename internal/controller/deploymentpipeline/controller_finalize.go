// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// PipelineCleanupFinalizer prevents the DeploymentPipeline from being deleted
	// while Projects that reference it are still finalizing. Projects depend on
	// the DeploymentPipeline to resolve environment names and compute data plane
	// namespace names during their own cleanup.
	PipelineCleanupFinalizer = "openchoreo.dev/deployment-pipeline-cleanup"
)

// finalize removes the finalizer once no Projects reference this DeploymentPipeline.
func (r *Reconciler) finalize(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deploymentPipeline", pipeline.Name)

	if !controllerutil.ContainsFinalizer(pipeline, PipelineCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	hasRefs, err := r.hasReferencingProjects(ctx, pipeline)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check referencing projects: %w", err)
	}

	if hasRefs {
		logger.Info("Waiting for referencing Projects to be deleted before removing finalizer")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if controllerutil.RemoveFinalizer(pipeline, PipelineCleanupFinalizer) {
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized DeploymentPipeline")
	return ctrl.Result{}, nil
}

// hasReferencingProjects checks if any Projects in the same namespace reference this DeploymentPipeline.
func (r *Reconciler) hasReferencingProjects(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (bool, error) {
	projectList := &openchoreov1alpha1.ProjectList{}
	if err := r.List(ctx, projectList,
		client.InNamespace(pipeline.Namespace),
		client.MatchingFields{controller.IndexKeyProjectDeploymentPipelineRef: pipeline.Name},
	); err != nil {
		return false, fmt.Errorf("failed to list projects: %w", err)
	}

	return len(projectList.Items) > 0, nil
}
