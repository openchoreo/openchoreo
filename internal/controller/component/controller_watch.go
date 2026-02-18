// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// componentTypeIndex is the field index name for componentType reference
	componentTypeIndex = "spec.componentType"
	// traitsIndex is the field index name for traits used
	traitsIndex = "spec.traits"
	// workflowIndex is the field index name for workflow reference
	workflowIndex = "spec.workflow.name"
	// workloadOwnerIndex is the field index name for workload owner references
	workloadOwnerIndex = "spec.owner"
	// releaseBindingIndex is the field index name for ReleaseBinding owner fields and environment
	releaseBindingIndex = "spec.owner.projectName/spec.owner.componentName/spec.environment"
	// componentWorkflowRunOwnerIndex is the field index name for ComponentWorkflowRun owner references
	componentWorkflowRunOwnerIndex = "spec.owner.componentName"
)

// makeReleaseBindingIndexKey creates the index key for ReleaseBinding lookups.
func makeReleaseBindingIndexKey(projectName, componentName, environment string) string {
	return fmt.Sprintf("%s/%s/%s", projectName, componentName, environment)
}

// setupComponentTypeRefIndex sets up the field index for componentType references.
// Index key format: "{kind}:{name}" (e.g., "ComponentType:deployment/web-app")
func (r *Reconciler) setupComponentTypeRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		componentTypeIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			return []string{string(comp.Spec.ComponentType.Kind) + ":" + comp.Spec.ComponentType.Name}
		})
}

// setupTraitsRefIndex sets up the field index for trait references
func (r *Reconciler) setupTraitsRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		traitsIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			traitNames := []string{}
			for _, trait := range comp.Spec.Traits {
				traitNames = append(traitNames, trait.Name)
			}
			return traitNames
		})
}

// setupWorkflowRefIndex sets up the field index for workflow references
func (r *Reconciler) setupWorkflowRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		workflowIndex, func(obj client.Object) []string {
			comp := obj.(*openchoreov1alpha1.Component)
			if comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
				return []string{}
			}
			return []string{comp.Spec.Workflow.Name}
		})
}

// setupWorkloadOwnerIndex sets up the field index for workload owner references
func (r *Reconciler) setupWorkloadOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Workload{},
		workloadOwnerIndex, func(obj client.Object) []string {
			workload := obj.(*openchoreov1alpha1.Workload)
			// Create a composite key: projectName/componentName
			ownerKey := fmt.Sprintf("%s/%s",
				workload.Spec.Owner.ProjectName,
				workload.Spec.Owner.ComponentName)
			return []string{ownerKey}
		})
}

// setupReleaseBindingIndex registers an index for ReleaseBinding by owner fields and environment.
func (r *Reconciler) setupReleaseBindingIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		releaseBindingIndex, func(obj client.Object) []string {
			releaseBinding := obj.(*openchoreov1alpha1.ReleaseBinding)
			key := makeReleaseBindingIndexKey(
				releaseBinding.Spec.Owner.ProjectName,
				releaseBinding.Spec.Owner.ComponentName,
				releaseBinding.Spec.Environment,
			)
			return []string{key}
		})
}

// setupComponentReleaseOwnerIndex registers an index for ComponentRelease by owner component name.
// This enables efficient lookup of ComponentReleases owned by a specific Component.
func (r *Reconciler) setupComponentReleaseOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentRelease{},
		"spec.owner.componentName", func(obj client.Object) []string {
			release := obj.(*openchoreov1alpha1.ComponentRelease)
			if release.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{release.Spec.Owner.ComponentName}
		})
}

// setupComponentWorkflowRunOwnerIndex registers an index for ComponentWorkflowRun by owner component name.
// This enables efficient lookup of ComponentWorkflowRuns owned by a specific Component.
func (r *Reconciler) setupComponentWorkflowRunOwnerIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentWorkflowRun{},
		componentWorkflowRunOwnerIndex, func(obj client.Object) []string {
			workflowRun := obj.(*openchoreov1alpha1.ComponentWorkflowRun)
			if workflowRun.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{workflowRun.Spec.Owner.ComponentName}
		})
}

// listComponentsForComponentType returns reconcile requests for all Components using this ComponentType
func (r *Reconciler) listComponentsForComponentType(ctx context.Context, obj client.Object) []reconcile.Request {
	ct := obj.(*openchoreov1alpha1.ComponentType)

	// Find all components using this ComponentType
	// Index key format: "ComponentType:{workloadType}/{ctName}"
	indexKey := string(openchoreov1alpha1.ComponentTypeRefKindComponentType) + ":" + fmt.Sprintf("%s/%s", ct.Spec.WorkloadType, ct.Name)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.InNamespace(ct.Namespace),
		client.MatchingFields{componentTypeIndex: indexKey}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for ComponentType", "componentType", ct.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsForClusterComponentType returns reconcile requests for all Components using this ClusterComponentType
func (r *Reconciler) listComponentsForClusterComponentType(ctx context.Context, obj client.Object) []reconcile.Request {
	cct := obj.(*openchoreov1alpha1.ClusterComponentType)

	// Find all components using this ClusterComponentType
	// Index key format: "ClusterComponentType:{workloadType}/{cctName}"
	indexKey := string(openchoreov1alpha1.ComponentTypeRefKindClusterComponentType) + ":" + fmt.Sprintf("%s/%s", cct.Spec.WorkloadType, cct.Name)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{componentTypeIndex: indexKey}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for ClusterComponentType", "clusterComponentType", cct.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsUsingTrait returns reconcile requests for all Components using this Trait
func (r *Reconciler) listComponentsUsingTrait(ctx context.Context, obj client.Object) []reconcile.Request {
	trait := obj.(*openchoreov1alpha1.Trait)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.MatchingFields{traitsIndex: trait.Name}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for Trait", "trait", trait.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsForComponentWorkflow returns reconcile requests for all Components using this ComponentWorkflow
func (r *Reconciler) listComponentsForComponentWorkflow(ctx context.Context, obj client.Object) []reconcile.Request {
	workflow := obj.(*openchoreov1alpha1.ComponentWorkflow)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.InNamespace(workflow.Namespace),
		client.MatchingFields{workflowIndex: workflow.Name}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list components for ComponentWorkflow", "workflow", workflow.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsForWorkload returns reconcile requests for the Component owning this Workload
func (r *Reconciler) listComponentsForWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	workload := obj.(*openchoreov1alpha1.Workload)

	// Use the owner reference from workload spec to find the owning component
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      workload.Spec.Owner.ComponentName,
			Namespace: workload.Namespace,
		},
	}}
}

// findComponentsForComponentRelease maps a ComponentRelease to its owner Component
func (r *Reconciler) findComponentsForComponentRelease(ctx context.Context, obj client.Object) []ctrl.Request {
	release := obj.(*openchoreov1alpha1.ComponentRelease)
	if release.Spec.Owner.ComponentName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Name:      release.Spec.Owner.ComponentName,
			Namespace: release.Namespace,
		},
	}}
}

// findComponentsForReleaseBinding maps a ReleaseBinding to its owner Component
func (r *Reconciler) findComponentsForReleaseBinding(ctx context.Context, obj client.Object) []ctrl.Request {
	binding := obj.(*openchoreov1alpha1.ReleaseBinding)
	if binding.Spec.Owner.ComponentName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Name:      binding.Spec.Owner.ComponentName,
			Namespace: binding.Namespace,
		},
	}}
}

// findComponentsForComponentWorkflowRun maps a ComponentWorkflowRun to its owner Component
func (r *Reconciler) findComponentsForComponentWorkflowRun(ctx context.Context, obj client.Object) []ctrl.Request {
	workflowRun := obj.(*openchoreov1alpha1.ComponentWorkflowRun)
	if workflowRun.Spec.Owner.ComponentName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Name:      workflowRun.Spec.Owner.ComponentName,
			Namespace: workflowRun.Namespace,
		},
	}}
}

// listComponentsForProject returns reconcile requests for all Components owned by this Project.
// This ensures Components are re-reconciled when their owning Project changes, particularly when
// the deploymentPipelineRef is added or modified.
func (r *Reconciler) listComponentsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project := obj.(*openchoreov1alpha1.Project)
	logger := ctrl.LoggerFrom(ctx)

	var components openchoreov1alpha1.ComponentList
	if err := r.List(ctx, &components,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controller.IndexKeyComponentOwnerProjectName: project.Name}); err != nil {
		logger.Error(err, "Failed to list components for Project", "project", project.Name)
		return nil
	}

	requests := make([]reconcile.Request, len(components.Items))
	for i, comp := range components.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
	}
	return requests
}

// listComponentsForDeploymentPipeline returns reconcile requests for Components that belong to Projects
// using the changed DeploymentPipeline. Components depend on the DeploymentPipeline's promotion paths
// to determine the root environment for ReleaseBindings, so they need to be re-reconciled when the
// pipeline changes.
func (r *Reconciler) listComponentsForDeploymentPipeline(ctx context.Context, obj client.Object) []reconcile.Request {
	pipeline := obj.(*openchoreov1alpha1.DeploymentPipeline)
	logger := ctrl.LoggerFrom(ctx)

	// Find Projects that reference this DeploymentPipeline
	var projects openchoreov1alpha1.ProjectList
	if err := r.List(ctx, &projects,
		client.InNamespace(pipeline.Namespace),
		client.MatchingFields{controller.IndexKeyProjectDeploymentPipelineRef: pipeline.Name}); err != nil {
		logger.Error(err, "Failed to list projects for DeploymentPipeline", "deploymentPipeline", pipeline.Name)
		return nil
	}

	if len(projects.Items) == 0 {
		return nil
	}

	// For each Project, find its Components
	var requests []reconcile.Request
	for _, project := range projects.Items {
		var components openchoreov1alpha1.ComponentList
		if err := r.List(ctx, &components,
			client.InNamespace(pipeline.Namespace),
			client.MatchingFields{controller.IndexKeyComponentOwnerProjectName: project.Name}); err != nil {
			logger.Error(err, "Failed to list components for Project",
				"project", project.Name, "deploymentPipeline", pipeline.Name)
			continue
		}

		for _, comp := range components.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      comp.Name,
					Namespace: comp.Namespace,
				},
			})
		}
	}
	return requests
}
