// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// PipelineCleanupFinalizer is the finalizer for DeploymentPipeline cleanup.
	PipelineCleanupFinalizer = "openchoreo.dev/deployment-pipeline-cleanup"
)

// finalize removes the finalizer to allow the DeploymentPipeline to be deleted.
// It is the user's responsibility to remove project references before deleting a pipeline.
func (r *Reconciler) finalize(ctx context.Context, pipeline *openchoreov1alpha1.DeploymentPipeline) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("deploymentPipeline", pipeline.Name)

	if !controllerutil.ContainsFinalizer(pipeline, PipelineCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	if controllerutil.RemoveFinalizer(pipeline, PipelineCleanupFinalizer) {
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized DeploymentPipeline")
	return ctrl.Result{}, nil
}
