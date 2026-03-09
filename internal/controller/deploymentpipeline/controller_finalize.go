// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// ReasonDeletionBlocked is the reason used when a deployment pipeline cannot be
	// deleted because it is still referenced by projects.
	ReasonDeletionBlocked controller.ConditionReason = "DeletionBlocked"

	// DeletionBlockedRequeueInterval is the interval at which the controller re-checks
	// whether referencing projects have been removed.
	DeletionBlockedRequeueInterval = 5 * time.Second
)

// finalize removes the finalizer once no Projects reference this DeploymentPipeline.
func (r *Reconciler) finalize(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deploymentPipeline", pipeline.Name)

	if !controllerutil.ContainsFinalizer(pipeline, PipelineCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	refCount, err := r.countReferencingProjects(ctx, pipeline)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check referencing projects: %w", err)
	}

	if refCount > 0 {
		msg := fmt.Sprintf("Deletion blocked: deployment pipeline is still referenced by %d project(s)", refCount)
		logger.Info(msg)
		if err := controller.UpdateCondition(
			ctx,
			r.Status(),
			pipeline,
			&pipeline.Status.Conditions,
			controller.TypeAvailable,
			metav1.ConditionFalse,
			string(ReasonDeletionBlocked),
			msg,
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status condition: %w", err)
		}
		return ctrl.Result{RequeueAfter: DeletionBlockedRequeueInterval}, nil
	}

	if controllerutil.RemoveFinalizer(pipeline, PipelineCleanupFinalizer) {
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized DeploymentPipeline")
	return ctrl.Result{}, nil
}

// countReferencingProjects returns the number of Projects in the same namespace that reference this DeploymentPipeline.
func (r *Reconciler) countReferencingProjects(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (int, error) {
	projectList := &openchoreov1alpha1.ProjectList{}
	if err := r.List(ctx, projectList,
		client.InNamespace(pipeline.Namespace),
		client.MatchingFields{controller.IndexKeyProjectDeploymentPipelineRef: pipeline.Name},
	); err != nil {
		return 0, fmt.Errorf("failed to list projects: %w", err)
	}

	return len(projectList.Items), nil
}
