// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ComponentFinalizer is the finalizer that ensures owned resources are deleted before Component
	ComponentFinalizer = "openchoreo.dev/component-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the Component.
// The first return value indicates whether the finalizer was added to the Component.
func (r *Reconciler) ensureFinalizer(ctx context.Context, comp *openchoreov1alpha1.Component) (bool, error) {
	// If the Component is being deleted, no need to add the finalizer
	if !comp.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(comp, ComponentFinalizer) {
		return true, r.Update(ctx, comp)
	}

	return false, nil
}

// finalize cleans up the resources associated with the Component.
func (r *Reconciler) finalize(ctx context.Context, old, comp *openchoreov1alpha1.Component) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	if !controllerutil.ContainsFinalizer(comp, ComponentFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the component condition as finalizing and return so that the component will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&comp.Status.Conditions, NewComponentFinalizingCondition(comp.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, comp)
	}

	// Delete all owned resources in a single reconcile
	hasReleases, err := r.hasOwnedComponentReleases(ctx, comp)
	if err != nil {
		logger.Error(err, "Failed to check for owned ComponentReleases")
		return ctrl.Result{}, err
	}

	hasBindings, err := r.hasOwnedReleaseBindings(ctx, comp)
	if err != nil {
		logger.Error(err, "Failed to check for owned ReleaseBindings")
		return ctrl.Result{}, err
	}

	hasWorkloads, err := r.hasOwnedWorkloads(ctx, comp)
	if err != nil {
		logger.Error(err, "Failed to check for owned Workloads")
		return ctrl.Result{}, err
	}

	hasWorkflowRuns, err := r.hasOwnedWorkflowRuns(ctx, comp)
	if err != nil {
		logger.Error(err, "Failed to check for owned WorkflowRuns")
		return ctrl.Result{}, err
	}

	// Requeue if any children still exist
	if hasReleases || hasBindings || hasWorkloads || hasWorkflowRuns {
		logger.Info("Waiting for owned resources to be deleted")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// All children are deleted - remove the finalizer
	if controllerutil.RemoveFinalizer(comp, ComponentFinalizer) {
		if err := r.Update(ctx, comp); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized Component")
	return ctrl.Result{}, nil
}

// hasOwnedComponentReleases checks if any ComponentReleases owned by this Component still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedComponentReleases(ctx context.Context, comp *openchoreov1alpha1.Component) (bool, error) {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	// List ComponentReleases owned by this Component using field index
	releaseList := &openchoreov1alpha1.ComponentReleaseList{}
	if err := r.List(ctx, releaseList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{"spec.owner.componentName": comp.Name}); err != nil {
		return false, fmt.Errorf("failed to list component releases: %w", err)
	}

	if len(releaseList.Items) == 0 {
		logger.Info("All ComponentReleases are deleted")
		return false, nil
	}

	// Delete all ComponentReleases owned by this Component
	logger.Info("Deleting owned ComponentReleases", "count", len(releaseList.Items))
	for i := range releaseList.Items {
		release := &releaseList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, release)); err != nil {
			return false, fmt.Errorf("failed to delete component release %s: %w", release.Name, err)
		}
	}

	return true, nil
}

// hasOwnedReleaseBindings checks if any ReleaseBindings owned by this Component still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedReleaseBindings(ctx context.Context, comp *openchoreov1alpha1.Component) (bool, error) {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	// List ReleaseBindings owned by this Component using shared field index
	bindingList := &openchoreov1alpha1.ReleaseBindingList{}
	if err := r.List(ctx, bindingList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: comp.Name}); err != nil {
		return false, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if len(bindingList.Items) == 0 {
		logger.Info("All ReleaseBindings are deleted")
		return false, nil
	}

	// Delete all ReleaseBindings owned by this Component
	logger.Info("Deleting owned ReleaseBindings", "count", len(bindingList.Items))
	for i := range bindingList.Items {
		binding := &bindingList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, binding)); err != nil {
			return false, fmt.Errorf("failed to delete release binding %s: %w", binding.Name, err)
		}
	}

	return true, nil
}

// hasOwnedWorkloads checks if any Workloads owned by this Component still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedWorkloads(ctx context.Context, comp *openchoreov1alpha1.Component) (bool, error) {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	// List Workloads owned by this Component using field index
	// The workloadOwnerIndex uses composite key: projectName/componentName
	ownerKey := fmt.Sprintf("%s/%s", comp.Spec.Owner.ProjectName, comp.Name)
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := r.List(ctx, workloadList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{workloadOwnerIndex: ownerKey}); err != nil {
		return false, fmt.Errorf("failed to list workloads: %w", err)
	}

	if len(workloadList.Items) == 0 {
		logger.Info("All Workloads are deleted")
		return false, nil
	}

	// Delete all Workloads owned by this Component
	logger.Info("Deleting owned Workloads", "count", len(workloadList.Items))
	for i := range workloadList.Items {
		workload := &workloadList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, workload)); err != nil {
			return false, fmt.Errorf("failed to delete workload %s: %w", workload.Name, err)
		}
	}

	return true, nil
}

// hasOwnedWorkflowRuns checks if any WorkflowRuns owned by this Component still exist,
// and deletes them if they exist.
func (r *Reconciler) hasOwnedWorkflowRuns(ctx context.Context, comp *openchoreov1alpha1.Component) (bool, error) {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	// List WorkflowRuns owned by this Component using field index.
	workflowRunList := &openchoreov1alpha1.WorkflowRunList{}
	if err := r.List(ctx, workflowRunList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{workflowRunOwnerIndex: comp.Name}); err != nil {
		return false, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	if len(workflowRunList.Items) == 0 {
		logger.Info("All WorkflowRuns are deleted")
		return false, nil
	}

	// Delete all WorkflowRuns owned by this Component.
	logger.Info("Deleting owned WorkflowRuns", "count", len(workflowRunList.Items))
	for i := range workflowRunList.Items {
		workflowRun := &workflowRunList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, workflowRun)); err != nil {
			return false, fmt.Errorf("failed to delete workflow run %s: %w", workflowRun.Name, err)
		}
	}

	return true, nil
}
