// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// resourceReleaseRefIndex indexes ResourceReleaseBinding by
	// spec.resourceRelease so the ResourceRelease watch mapper can answer
	// "which bindings pin this release?" with a single namespaced lookup.
	resourceReleaseRefIndex = "spec.resourceRelease"

	// ownerProjectNameIndex indexes ResourceReleaseBinding by
	// spec.owner.projectName so the Project watch mapper can find the
	// bindings of a project with a single namespaced lookup.
	ownerProjectNameIndex = "spec.owner.projectName"
)

// indexResourceReleaseRef extracts the spec.resourceRelease key from a
// binding. Bindings with an unset pin index to an empty key (filtered out
// in the mapper).
func indexResourceReleaseRef(obj client.Object) []string {
	binding := obj.(*openchoreov1alpha1.ResourceReleaseBinding)
	return []string{binding.Spec.ResourceRelease}
}

// setupResourceReleaseRefIndex registers the field index used by the
// ResourceRelease watch mapper.
func (r *Reconciler) setupResourceReleaseRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ResourceReleaseBinding{},
		resourceReleaseRefIndex, indexResourceReleaseRef)
}

// indexOwnerProjectName extracts the spec.owner.projectName key from a binding.
func indexOwnerProjectName(obj client.Object) []string {
	binding := obj.(*openchoreov1alpha1.ResourceReleaseBinding)
	return []string{binding.Spec.Owner.ProjectName}
}

// setupOwnerProjectNameIndex registers the field index used by the Project
// watch mapper.
func (r *Reconciler) setupOwnerProjectNameIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ResourceReleaseBinding{},
		ownerProjectNameIndex, indexOwnerProjectName)
}

// listResourceReleaseBindingsForResourceRelease returns reconcile requests
// for ResourceReleaseBindings in the same namespace that pin the supplied
// ResourceRelease via spec.resourceRelease. ResourceReleases are immutable,
// so only Create and Delete events reach this mapper:
//   - Create wakes bindings that were authored before the release existed
//     (GitOps applies binding + release together, binding lands first).
//   - Delete surfaces ResourceReleaseNotFound on the binding immediately
//     instead of waiting for the next cache resync.
func (r *Reconciler) listResourceReleaseBindingsForResourceRelease(ctx context.Context, obj client.Object) []reconcile.Request {
	release := obj.(*openchoreov1alpha1.ResourceRelease)
	if release.Name == "" {
		return nil
	}

	var bindings openchoreov1alpha1.ResourceReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(release.Namespace),
		client.MatchingFields{resourceReleaseRefIndex: release.Name},
	); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list ResourceReleaseBindings by resourceRelease ref",
			"resourceRelease", release.Name, "namespace", release.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, len(bindings.Items))
	for i := range bindings.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      bindings.Items[i].Name,
				Namespace: bindings.Items[i].Namespace,
			},
		}
	}
	return requests
}

// listResourceReleaseBindingsForProject returns reconcile requests for
// ResourceReleaseBindings in the same namespace whose spec.owner.projectName
// matches the supplied Project. A binding that hit ProjectNotFound returns
// without a requeue, so this watch is what wakes it when the Project is
// created later (GitOps applies the Project and the binding from different
// sources, in any order). The ResourceRelease watch does not cover this: the
// Resource controller cuts the release without checking that the Project
// exists.
func (r *Reconciler) listResourceReleaseBindingsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project := obj.(*openchoreov1alpha1.Project)
	if project.Name == "" {
		return nil
	}

	var bindings openchoreov1alpha1.ResourceReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(project.Namespace),
		client.MatchingFields{ownerProjectNameIndex: project.Name},
	); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list ResourceReleaseBindings by owner project",
			"project", project.Name, "namespace", project.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, len(bindings.Items))
	for i := range bindings.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      bindings.Items[i].Name,
				Namespace: bindings.Items[i].Namespace,
			},
		}
	}
	return requests
}
