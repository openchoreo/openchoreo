// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// PipelineCleanupFinalizer prevents the DeploymentPipeline from being deleted
	// before referencing Projects have had their deploymentPipelineRef cleared.
	PipelineCleanupFinalizer = "openchoreo.dev/deployment-pipeline-cleanup"
)

// finalize clears the deploymentPipelineRef from all referencing Projects,
// then removes the finalizer so the DeploymentPipeline can be deleted.
func (r *Reconciler) finalize(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deploymentPipeline", pipeline.Name)

	if !controllerutil.ContainsFinalizer(pipeline, PipelineCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	if err := r.clearReferencingProjects(ctx, pipeline); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to clear referencing projects: %w", err)
	}

	if controllerutil.RemoveFinalizer(pipeline, PipelineCleanupFinalizer) {
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized DeploymentPipeline")
	return ctrl.Result{}, nil
}

// clearReferencingProjects finds all Projects in the same namespace that reference
// this DeploymentPipeline and sets their DeploymentPipelineRef to nil.
func (r *Reconciler) clearReferencingProjects(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) error {
	logger := log.FromContext(ctx).WithValues("deploymentPipeline", pipeline.Name)

	projectList := &openchoreov1alpha1.ProjectList{}
	if err := r.List(ctx, projectList,
		client.InNamespace(pipeline.Namespace),
		client.MatchingFields{controller.IndexKeyProjectDeploymentPipelineRef: pipeline.Name},
	); err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	for i := range projectList.Items {
		project := &projectList.Items[i]
		logger.Info("Clearing deploymentPipelineRef from project", "project", project.Name)
		project.Spec.DeploymentPipelineRef = nil
		if err := r.Update(ctx, project); err != nil {
			return fmt.Errorf("failed to clear deploymentPipelineRef on project %s: %w", project.Name, err)
		}
	}

	return nil
}
