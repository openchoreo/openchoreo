// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// The reconcile path fetches the Environment, DataPlane (or ClusterDataPlane),
// and Project to render and emit a RenderedRelease. If any of those isn't
// present at reconcile time the binding settles into a "*NotFound"
// Synced=False state. Without watches on those upstream resources nothing
// wakes the binding up when they later land — common during initial install
// where the dataplane chart is applied after the default resources. The
// handlers below close that loop.
//
// ProjectRelease is intentionally NOT watched: it is immutable by design
// (CEL self == oldSelf guard) and the Project controller cuts a new
// ProjectRelease before the ProjectReleaseBindings that pin to it (same
// reconcile pass), so there is no realistic Create event the binding needs
// to observe.

// listBindingsForDataPlane re-enqueues every ProjectReleaseBinding in the
// DataPlane's namespace whose referenced Environment uses this DataPlane.
func (r *Reconciler) listBindingsForDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	dp := obj.(*openchoreov1alpha1.DataPlane)
	return r.bindingsForReferencedEnvironments(ctx, dp.Namespace, func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind != openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == dp.Name
	})
}

// listBindingsForClusterDataPlane re-enqueues every ProjectReleaseBinding
// (cluster-wide) whose referenced Environment uses this ClusterDataPlane.
func (r *Reconciler) listBindingsForClusterDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	cdp := obj.(*openchoreov1alpha1.ClusterDataPlane)
	return r.bindingsForReferencedEnvironments(ctx, "", func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == cdp.Name
	})
}

// listBindingsForEnvironment re-enqueues bindings in the env's namespace
// whose spec.environment matches.
func (r *Reconciler) listBindingsForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	env := obj.(*openchoreov1alpha1.Environment)
	return r.bindingsFiltered(ctx, env.Namespace, func(b *openchoreov1alpha1.ProjectReleaseBinding) bool {
		return b.Spec.Environment == env.Name
	})
}

// listBindingsForProject re-enqueues bindings owned by this Project.
func (r *Reconciler) listBindingsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project := obj.(*openchoreov1alpha1.Project)
	return r.bindingsFiltered(ctx, project.Namespace, func(b *openchoreov1alpha1.ProjectReleaseBinding) bool {
		return b.Spec.Owner.ProjectName == project.Name
	})
}

// bindingsForReferencedEnvironments lists Environments in `namespace` (or
// cluster-wide if empty) whose spec matches the predicate, then enqueues
// every ProjectReleaseBinding whose spec.environment points to one of those
// envs in the env's own namespace.
func (r *Reconciler) bindingsForReferencedEnvironments(
	ctx context.Context,
	namespace string,
	match func(*openchoreov1alpha1.Environment) bool,
) []reconcile.Request {
	envListOpts := []client.ListOption{}
	if namespace != "" {
		envListOpts = append(envListOpts, client.InNamespace(namespace))
	}
	envs := &openchoreov1alpha1.EnvironmentList{}
	if err := r.List(ctx, envs, envListOpts...); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list Environments for binding watch")
		return nil
	}

	type nsenv struct {
		namespace string
		name      string
	}
	matched := make(map[nsenv]struct{})
	for i := range envs.Items {
		e := &envs.Items[i]
		if match(e) {
			matched[nsenv{namespace: e.Namespace, name: e.Name}] = struct{}{}
		}
	}
	if len(matched) == 0 {
		return nil
	}

	bindings := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, bindings, envListOpts...); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list ProjectReleaseBindings for binding watch")
		return nil
	}
	var requests []reconcile.Request
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if _, ok := matched[nsenv{namespace: b.Namespace, name: b.Spec.Environment}]; ok {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: b.Name, Namespace: b.Namespace},
			})
		}
	}
	return requests
}

// bindingsFiltered lists ProjectReleaseBindings in `namespace` whose spec
// matches the predicate.
func (r *Reconciler) bindingsFiltered(
	ctx context.Context,
	namespace string,
	match func(*openchoreov1alpha1.ProjectReleaseBinding) bool,
) []reconcile.Request {
	bindings := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list ProjectReleaseBindings")
		return nil
	}
	var requests []reconcile.Request
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if match(b) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: b.Name, Namespace: b.Namespace},
			})
		}
	}
	return requests
}
