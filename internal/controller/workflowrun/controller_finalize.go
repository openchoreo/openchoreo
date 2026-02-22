// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// WorkflowRunCleanupFinalizer is the finalizer used to clean up build plane resources.
	WorkflowRunCleanupFinalizer = "openchoreo.dev/workflowrun-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the WorkflowRun.
// Returns true if the finalizer was added (indicating the caller should return early).
func (r *Reconciler) ensureFinalizer(ctx context.Context, cwRun *openchoreodevv1alpha1.WorkflowRun) (bool, error) {
	// If already being deleted, no need to add finalizer
	if !cwRun.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(cwRun, WorkflowRunCleanupFinalizer) {
		return true, r.Update(ctx, cwRun)
	}

	return false, nil
}

// finalize cleans up the build plane resources associated with the WorkflowRun.
func (r *Reconciler) finalize(ctx context.Context, cwRun *openchoreodevv1alpha1.WorkflowRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cwRun, WorkflowRunCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Get build plane client (supports both BuildPlane and ClusterBuildPlane)
	buildPlaneResult, err := controller.ResolveBuildPlane(ctx, r.Client, cwRun)
	if err != nil {
		// If build plane doesn't exist, we can't clean up - remove finalizer anyway
		if controller.IgnoreHierarchyNotFoundError(err) == nil || errors.IsNotFound(err) {
			logger.Info("BuildPlane not found, removing finalizer without cleanup", "error", err)
			return r.removeFinalizer(ctx, cwRun)
		}
		return ctrl.Result{Requeue: true}, err
	}
	if buildPlaneResult == nil {
		logger.Info("No build plane found, removing finalizer without cleanup")
		return r.removeFinalizer(ctx, cwRun)
	}

	bpClient, err := r.getBuildPlaneClient(buildPlaneResult)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Delete additional resources from status.Resources
	if cwRun.Status.Resources != nil {
		for _, res := range *cwRun.Status.Resources {
			if err := r.deleteResource(ctx, bpClient, res); err != nil {
				if !errors.IsNotFound(err) {
					logger.Error(err, "failed to delete resource", "name", res.Name, "namespace", res.Namespace, "kind", res.Kind)
					return ctrl.Result{Requeue: true}, nil
				}
			}
			logger.Info("deleted resource", "name", res.Name, "namespace", res.Namespace, "kind", res.Kind)
		}
	}

	// Delete the run resource from status.RunReference
	if cwRun.Status.RunReference != nil && cwRun.Status.RunReference.Name != "" {
		if err := r.deleteResource(ctx, bpClient, *cwRun.Status.RunReference); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "failed to delete run resource",
					"name", cwRun.Status.RunReference.Name,
					"namespace", cwRun.Status.RunReference.Namespace)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}

	return r.removeFinalizer(ctx, cwRun)
}

// deleteResource deletes a single resource from the build plane using the ResourceReference.
func (r *Reconciler) deleteResource(ctx context.Context, bpClient client.Client, ref openchoreodevv1alpha1.ResourceReference) error {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return fmt.Errorf("failed to parse API version %q: %w", ref.APIVersion, err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    ref.Kind,
	})

	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, obj); err != nil {
		return err
	}

	return bpClient.Delete(ctx, obj)
}

// removeFinalizer removes the finalizer from the WorkflowRun.
func (r *Reconciler) removeFinalizer(ctx context.Context, cwRun *openchoreodevv1alpha1.WorkflowRun) (ctrl.Result, error) {
	if controllerutil.RemoveFinalizer(cwRun, WorkflowRunCleanupFinalizer) {
		if err := r.Update(ctx, cwRun); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
