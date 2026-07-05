// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	controllerpkg "github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// projectTypeRefIndex indexes Project by its (Cluster)ProjectType
	// reference. Index key format: "{kind}:{name}" — e.g. "ProjectType:foo",
	// "ClusterProjectType:standard". Mirrors resource.resourceTypeRefIndex.
	projectTypeRefIndex = "spec.type"
)

// findProjectForComponent maps a Component to its owner Project
func (r *Reconciler) findProjectForComponent(ctx context.Context, obj client.Object) []ctrl.Request {
	component := obj.(*openchoreov1alpha1.Component)
	if component.Spec.Owner.ProjectName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: client.ObjectKey{
			Name:      component.Spec.Owner.ProjectName,
			Namespace: component.Namespace,
		},
	}}
}

// findProjectForProjectReleaseBinding maps a ProjectReleaseBinding to its
// owner Project via spec.owner.projectName. Deliberately not an Owns watch:
// externally authored bindings (console, occ, API, GitOps) carry no
// OwnerReference, but the Project controller must still reconcile to seed
// their empty projectRelease pins.
func (r *Reconciler) findProjectForProjectReleaseBinding(ctx context.Context, obj client.Object) []ctrl.Request {
	binding := obj.(*openchoreov1alpha1.ProjectReleaseBinding)
	if binding.Spec.Owner.ProjectName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: client.ObjectKey{
			Name:      binding.Spec.Owner.ProjectName,
			Namespace: binding.Namespace,
		},
	}}
}

// indexProjectTypeRef extracts the (Cluster)ProjectType reference key from a
// Project. Exposed as a package-level value so tests can pass it to
// fake.NewClientBuilder().WithIndex.
func indexProjectTypeRef(obj client.Object) []string {
	project := obj.(*openchoreov1alpha1.Project)
	kind := projectTypeKind(project.Spec.Type.Kind)
	return []string{string(kind) + ":" + project.Spec.Type.Name}
}

// setupProjectTypeRefIndex registers the field index used by the watch mappers.
func (r *Reconciler) setupProjectTypeRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Project{},
		projectTypeRefIndex, indexProjectTypeRef)
}

// listProjectsForProjectType returns reconcile requests for Projects in the
// same namespace as the given ProjectType that reference it via
// spec.type.{Kind=ProjectType, Name=pt.Name}.
func (r *Reconciler) listProjectsForProjectType(ctx context.Context, obj client.Object) []reconcile.Request {
	pt := obj.(*openchoreov1alpha1.ProjectType)
	indexKey := string(openchoreov1alpha1.ProjectTypeRefKindProjectType) + ":" + pt.Name
	return r.requestsForProjectTypeIndexKey(ctx, indexKey, client.InNamespace(pt.Namespace))
}

// listProjectsForClusterProjectType returns reconcile requests for Projects
// across all namespaces that reference the given ClusterProjectType via
// spec.type.{Kind=ClusterProjectType, Name=cpt.Name}.
func (r *Reconciler) listProjectsForClusterProjectType(ctx context.Context, obj client.Object) []reconcile.Request {
	cpt := obj.(*openchoreov1alpha1.ClusterProjectType)
	indexKey := string(openchoreov1alpha1.ProjectTypeRefKindClusterProjectType) + ":" + cpt.Name
	return r.requestsForProjectTypeIndexKey(ctx, indexKey)
}

// listProjectsForDeploymentPipeline returns reconcile requests for Projects
// in the same namespace as the given DeploymentPipeline that reference it
// via spec.deploymentPipelineRef.name. Drives re-reconcile (and binding
// fan-out updates) when PEs add/remove environments on the pipeline.
func (r *Reconciler) listProjectsForDeploymentPipeline(ctx context.Context, obj client.Object) []reconcile.Request {
	pipeline := obj.(*openchoreov1alpha1.DeploymentPipeline)
	var projects openchoreov1alpha1.ProjectList
	if err := r.List(ctx, &projects,
		client.InNamespace(pipeline.Namespace),
		client.MatchingFields{controllerpkg.IndexKeyProjectDeploymentPipelineRef: pipeline.Name}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Projects by deploymentPipelineRef",
			"deploymentPipeline", pipeline.Name)
		return nil
	}
	requests := make([]reconcile.Request, len(projects.Items))
	for i, p := range projects.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{Name: p.Name, Namespace: p.Namespace},
		}
	}
	return requests
}

// requestsForProjectTypeIndexKey lists Projects by the projectTypeRefIndex and
// converts them to reconcile.Requests. Extra ListOptions (e.g.
// client.InNamespace) scope the lookup further.
func (r *Reconciler) requestsForProjectTypeIndexKey(ctx context.Context, indexKey string, opts ...client.ListOption) []reconcile.Request {
	listOpts := append([]client.ListOption{client.MatchingFields{projectTypeRefIndex: indexKey}}, opts...)

	var projects openchoreov1alpha1.ProjectList
	if err := r.List(ctx, &projects, listOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Projects by type ref", "indexKey", indexKey)
		return nil
	}

	requests := make([]reconcile.Request, len(projects.Items))
	for i, p := range projects.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{Name: p.Name, Namespace: p.Namespace},
		}
	}
	return requests
}
