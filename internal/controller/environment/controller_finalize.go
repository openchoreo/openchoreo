// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

const (
	// EnvCleanupFinalizer is the finalizer that is used to clean up the environment.
	EnvCleanupFinalizer = "openchoreo.dev/environment-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the environment.
// The first return value indicates whether the finalizer was added to the environment.
func (r *Reconciler) ensureFinalizer(ctx context.Context, environment *openchoreov1alpha1.Environment) (bool, error) {
	if !environment.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(environment, EnvCleanupFinalizer) {
		return true, r.Update(ctx, environment)
	}

	return false, nil
}

// finalize cleans up the resources associated with the environment.
// The finalization flow is:
//  1. Check if the environment is referenced by any DeploymentPipeline — if so, block deletion.
//  2. Wait for all ReleaseBindings that reference this environment to be gone.
//  3. Delete the data plane namespaces associated with the environment.
//  4. Wait for namespace deletion to complete.
//  5. Remove the finalizer to allow garbage collection.
func (r *Reconciler) finalize(ctx context.Context, old, environment *openchoreov1alpha1.Environment) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("environment", environment.Name)
	if !controllerutil.ContainsFinalizer(environment, EnvCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Step 1: Block deletion if the environment is referenced by a DeploymentPipeline.
	referencingPipeline, err := r.findReferencingPipeline(ctx, environment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check deployment pipeline references: %w", err)
	}
	if referencingPipeline != "" {
		msg := fmt.Sprintf("Deletion blocked: environment is referenced by DeploymentPipeline %q", referencingPipeline)
		logger.Info(msg)
		if meta.SetStatusCondition(&environment.Status.Conditions, NewDeletionBlockedCondition(environment.Generation, msg)) {
			if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Requeue to re-evaluate once the pipeline reference is removed
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Mark the environment condition as finalizing.
	if meta.SetStatusCondition(&environment.Status.Conditions, NewEnvironmentFinalizingCondition(environment.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 2: Delete all release bindings referencing this environment and wait for them to be gone.
	pendingCount, err := r.deleteAndCountReleaseBindings(ctx, environment)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pendingCount > 0 {
		msg := fmt.Sprintf("Deleting %d release binding(s)", pendingCount)
		logger.Info(msg)
		if meta.SetStatusCondition(&environment.Status.Conditions, NewReleaseBindingsPendingCondition(environment.Generation, msg)) {
			if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Step 4 & 5: Delete data plane namespaces and wait for them to be gone.
	// If the DataPlane is already gone, skip — the namespaces are assumed to be cleaned up with it.
	// getDPClient handles both namespace-scoped DataPlane and cluster-scoped ClusterDataPlane refs.
	dpClient, err := r.getDPClient(ctx, environment)
	if err != nil {
		if isDataPlaneNotFoundError(err) {
			if skip, skipErr := r.shouldSkipCleanupForMissingDataPlane(ctx, environment); skipErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to verify data plane during finalization: %w", skipErr)
			} else if !skip {
				return ctrl.Result{}, fmt.Errorf("failed to get data plane client during finalization: %w", err)
			}
			logger.Info("DataPlane not found during finalization, skipping namespace cleanup")
			return r.removeFinalizer(ctx, environment)
		}
		// When no explicit dataPlaneRef is set and neither a default DataPlane nor
		// a default ClusterDataPlane exists, there is nothing to clean up.
		if environment.Spec.DataPlaneRef == nil {
			logger.Info("No data plane reference and no defaults found during finalization, skipping namespace cleanup")
			return r.removeFinalizer(ctx, environment)
		}
		logger.Error(err, "Error getting DP client")
		return ctrl.Result{}, err
	}

	// The namespace handler only needs Environment from EnvironmentContext;
	// DataPlane is not accessed during finalization cleanup.
	envCtx := &dataplane.EnvironmentContext{
		Environment: environment,
	}

	resourceHandlers := r.makeExternalResourceHandlers(dpClient)
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		exists, err := resourceHandler.GetCurrentState(ctx, envCtx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check existence of external resource %s: %w", resourceHandler.Name(), err)
		}
		if exists == nil {
			continue
		}

		pendingDeletion = true
		if err := resourceHandler.Delete(ctx, envCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	if pendingDeletion {
		logger.Info("Waiting for data plane namespace deletion")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Step 6: All cleanup complete — remove finalizer.
	return r.removeFinalizer(ctx, environment)
}

// removeFinalizer removes the cleanup finalizer from the environment.
func (r *Reconciler) removeFinalizer(ctx context.Context, environment *openchoreov1alpha1.Environment) (ctrl.Result, error) {
	if controllerutil.RemoveFinalizer(environment, EnvCleanupFinalizer) {
		if err := r.Update(ctx, environment); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// shouldSkipCleanupForMissingDataPlane determines whether namespace cleanup can be
// safely skipped when the DataPlane lookup has already failed with a not-found error.
//
// For environments referencing a namespaced DataPlane, cleanup can always be skipped
// because the DataPlane is truly gone along with its namespaces.
//
// For environments referencing a ClusterDataPlane, we must verify the cluster-scoped
// resource is also gone — GetDataPlaneByEnvironment returns HierarchyNotFoundError
// for ClusterDataPlane refs even when the ClusterDataPlane exists, because it only
// handles namespaced DataPlanes. Skipping cleanup in that case would leak namespaces.
//
// Returns (true, nil) if cleanup can be skipped, (false, nil) if ClusterDataPlane
// still exists, or (false, err) if the lookup itself failed.
func (r *Reconciler) shouldSkipCleanupForMissingDataPlane(ctx context.Context, env *openchoreov1alpha1.Environment) (bool, error) {
	// Non-ClusterDataPlane refs: the DataPlane is genuinely gone, safe to skip.
	if env.Spec.DataPlaneRef == nil ||
		env.Spec.DataPlaneRef.Kind != openchoreov1alpha1.DataPlaneRefKindClusterDataPlane {
		return true, nil
	}

	// ClusterDataPlane ref: verify the cluster-scoped resource is also missing.
	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	if err := r.Get(ctx, client.ObjectKey{Name: env.Spec.DataPlaneRef.Name}, cdp); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("failed to look up ClusterDataPlane %q: %w", env.Spec.DataPlaneRef.Name, err)
	}
	// ClusterDataPlane exists — do not skip cleanup.
	return false, nil
}

// isDataPlaneNotFoundError checks if the error is due to the DataPlane not being found.
func isDataPlaneNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for HierarchyNotFoundError (returned by GetDataPlaneByEnvironment)
	if controller.IgnoreHierarchyNotFoundError(err) == nil {
		return true
	}
	// Check for standard Kubernetes not-found error (wrapped in some paths)
	if apierrors.IsNotFound(err) {
		return true
	}
	return false
}

// findReferencingPipeline returns the name of the first DeploymentPipeline that references
// the given environment in its promotionPaths. Returns empty string if none found.
func (r *Reconciler) findReferencingPipeline(ctx context.Context, environment *openchoreov1alpha1.Environment) (string, error) {
	pipelineList := &openchoreov1alpha1.DeploymentPipelineList{}
	if err := r.List(ctx, pipelineList,
		client.InNamespace(environment.Namespace),
		client.MatchingFields{controller.IndexKeyDeploymentPipelineEnvironmentRef: environment.Name},
	); err != nil {
		return "", fmt.Errorf("failed to list deployment pipelines: %w", err)
	}

	if len(pipelineList.Items) > 0 {
		return pipelineList.Items[0].Name, nil
	}

	return "", nil
}

// deleteAndCountReleaseBindings deletes all release bindings that reference this environment
// and returns the count of those still present (pending deletion or not yet deleted).
func (r *Reconciler) deleteAndCountReleaseBindings(ctx context.Context, environment *openchoreov1alpha1.Environment) (int, error) {
	releaseBindingList := &openchoreov1alpha1.ReleaseBindingList{}
	if err := r.List(ctx, releaseBindingList, client.InNamespace(environment.Namespace)); err != nil {
		return 0, fmt.Errorf("failed to list release bindings: %w", err)
	}

	count := 0
	for i := range releaseBindingList.Items {
		rb := &releaseBindingList.Items[i]
		if rb.Spec.Environment != environment.Name {
			continue
		}
		count++
		if rb.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, rb); err != nil && !apierrors.IsNotFound(err) {
				return 0, fmt.Errorf("failed to delete release binding %s: %w", rb.Name, err)
			}
		}
	}

	return count, nil
}
