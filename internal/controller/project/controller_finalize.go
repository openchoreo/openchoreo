// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	k8sintegrations "github.com/openchoreo/openchoreo/internal/controller/project/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

const (
	// ProjectCleanupFinalizer is the finalizer that is used to clean up project resources.
	ProjectCleanupFinalizer = "openchoreo.dev/project-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the project.
// The first return value indicates whether the finalizer was added to the project.
func (r *Reconciler) ensureFinalizer(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	// If the project is being deleted, no need to add the finalizer
	if !project.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(project, ProjectCleanupFinalizer) {
		return true, r.Update(ctx, project)
	}

	return false, nil
}

// finalize cleans up the resources associated with the project.
func (r *Reconciler) finalize(ctx context.Context, old, project *openchoreov1alpha1.Project) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	if !controllerutil.ContainsFinalizer(project, ProjectCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the project condition as finalizing and return so that the project will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	cond := meta.FindStatusCondition(project.Status.Conditions, string(ConditionFinalizing))
	if cond == nil || cond.Status != metav1.ConditionTrue {
		if meta.SetStatusCondition(&project.Status.Conditions, NewProjectFinalizingCondition(project.Generation)) {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, project)
		}
	}

	// Perform cleanup logic for deployment tracks
	artifactsDeleted, err := r.deleteChildAndLinkedResources(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete child resources")
		// If there was an error deleting the child resources, we should not remove the finalizer
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// If deletion is still in progress, check in next cycle
	if !artifactsDeleted {
		logger.Info("Child resources are still being deleted", "name", project.Name)
		if controller.MarkTrueCondition(project, ConditionFinalizing, ReasonProjectFinalizing, "Waiting for child resources to be deleted") {
			return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, old, project, time.Second*5)
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(project, ProjectCleanupFinalizer) {
		if err := r.Update(ctx, project); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized project")
	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteChildAndLinkedResources(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// Clean up components
	componentsDeleted, err := r.deleteComponentsAndWait(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete components")
		return false, err
	}

	// Clean up resources
	resourcesDeleted, err := r.deleteResourcesAndWait(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete resources")
		return false, err
	}

	// Clean up project release bindings
	bindingsDeleted, err := r.deleteProjectReleaseBindingsAndWait(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete project release bindings")
		return false, err
	}

	if !componentsDeleted || !resourcesDeleted || !bindingsDeleted {
		logger.Info("Children are still being deleted", "name", project.Name)
		return false, nil
	}

	// At this point all control plane resource from components downwards should be deleted
	// Also all dataplane resources from deployments in the project should be deleted
	// Now we can delete the dataplane namespaces
	externalResourcesDeleted, err := r.deleteExternalResourcesAndWait(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to delete external resources")
		return false, err
	}
	if !externalResourcesDeleted {
		logger.Info("External resources are still being deleted", "name", project.Name)
		return false, nil
	}

	logger.Info("All dependent resources are deleted")
	return true, nil
}

// deleteComponentsAndWait checks if any Components owned by this Project still exist,
// and deletes them if they exist.
func (r *Reconciler) deleteComponentsAndWait(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// List Components owned by this Project using shared field index
	componentsList := &openchoreov1alpha1.ComponentList{}
	if err := r.List(ctx, componentsList,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controller.IndexKeyComponentOwnerProjectName: project.Name}); err != nil {
		return false, fmt.Errorf("failed to list components: %w", err)
	}

	if len(componentsList.Items) == 0 {
		logger.Info("All components are deleted")
		return true, nil
	}

	// Delete all Components owned by this Project
	logger.Info("Deleting owned Components", "count", len(componentsList.Items))
	for i := range componentsList.Items {
		component := &componentsList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, component)); err != nil {
			return false, fmt.Errorf("failed to delete component %s: %w", component.Name, err)
		}
	}

	return false, nil
}

// deleteResourcesAndWait checks if any Resources owned by this Project still exist,
// and deletes them if they exist.
func (r *Reconciler) deleteResourcesAndWait(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// List Resources owned by this Project using shared field index
	resourcesList := &openchoreov1alpha1.ResourceList{}
	if err := r.List(ctx, resourcesList,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controller.IndexKeyResourceOwnerProjectName: project.Name}); err != nil {
		return false, fmt.Errorf("failed to list resources: %w", err)
	}

	if len(resourcesList.Items) == 0 {
		logger.Info("All resources are deleted")
		return true, nil
	}

	// Delete all Resources owned by this Project
	logger.Info("Deleting owned Resources", "count", len(resourcesList.Items))
	for i := range resourcesList.Items {
		resource := &resourcesList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, resource)); err != nil {
			return false, fmt.Errorf("failed to delete resource %s: %w", resource.Name, err)
		}
	}

	return false, nil
}

// deleteProjectReleaseBindingsAndWait checks if any ProjectReleaseBindings of this
// Project still exist, and deletes them if they exist. Bindings are matched by
// spec.owner.projectName regardless of author or OwnerReference: externally
// authored bindings (console, occ, API, GitOps) carry no owner reference, and a
// binding without its project is meaningless (the owner tuple is immutable).
// Deleting a binding tears down its downstream state via the K8s GC walking the
// binding's RenderedRelease to the applied manifests.
func (r *Reconciler) deleteProjectReleaseBindingsAndWait(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	bindingsList := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, bindingsList,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controller.IndexKeyProjectReleaseBindingOwner: project.Name}); err != nil {
		return false, fmt.Errorf("failed to list project release bindings: %w", err)
	}

	if len(bindingsList.Items) == 0 {
		logger.Info("All project release bindings are deleted")
		return true, nil
	}

	logger.Info("Deleting ProjectReleaseBindings", "count", len(bindingsList.Items))
	for i := range bindingsList.Items {
		binding := &bindingsList.Items[i]
		if err := client.IgnoreNotFound(r.Delete(ctx, binding)); err != nil {
			return false, fmt.Errorf("failed to delete project release binding %s: %w", binding.Name, err)
		}
	}

	return false, nil
}

// deleteExternalResourcesAndWait cleans up any resources that are dependent on this Project
func (r *Reconciler) deleteExternalResourcesAndWait(ctx context.Context, project *openchoreov1alpha1.Project) (bool, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// Create the project context for external resource deletions
	// This will include the deployment pipeline and the environments
	projectCtx, err := r.makeProjectContext(ctx, project)
	if err != nil {
		return false, fmt.Errorf("failed to construct project context for finalization: %w", err)
	}

	// Delete dataplane resources
	resourceHandlers := r.makeExternalResourceHandlers()
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		// Check if the namespaces are still being deleted
		exists, err := resourceHandler.GetCurrentState(ctx, projectCtx)
		if err != nil {
			return false, fmt.Errorf("failed to check existence of external resource %s: %w", resourceHandler.Name(), err)
		}

		if exists == nil {
			continue
		}

		pendingDeletion = true
		// Trigger deletion of the resource as it still exists
		if err := resourceHandler.Delete(ctx, projectCtx); err != nil {
			return false, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	// Requeue the reconcile loop if there are still resources pending deletion
	if pendingDeletion {
		logger.Info("endpoint deletion is still pending as the dependent resource deletion pending.. retrying..")
		return false, nil
	}

	logger.Info("All dataplane resources are deleted")
	return true, nil
}

func (r *Reconciler) makeExternalResourceHandlers() []dataplane.ResourceHandler[dataplane.ProjectContext] {
	return []dataplane.ResourceHandler[dataplane.ProjectContext]{
		k8sintegrations.NewNamespaceHandler(r.Client),
	}
}
