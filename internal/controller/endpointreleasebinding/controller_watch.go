// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// listBindingsForReleaseBinding re-enqueues the EndpointReleaseBindings that
// route to the ReleaseBinding's component in its environment, so a backend
// being deployed, gaining an address, or being undeployed is picked up.
// Bindings are narrowed to the same project and environment, then to those
// whose pinned release names the component, so a component's status churn
// does not wake every Endpoint in the project.
func (r *Reconciler) listBindingsForReleaseBinding(ctx context.Context, obj client.Object) []reconcile.Request {
	rb := obj.(*openchoreov1alpha1.ReleaseBinding)
	logger := log.FromContext(ctx)

	bindings := &openchoreov1alpha1.EndpointReleaseBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(rb.Namespace)); err != nil {
		logger.Error(err, "Failed to list EndpointReleaseBindings for ReleaseBinding watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if b.Spec.Owner.ProjectName != rb.Spec.Owner.ProjectName ||
			b.Spec.Environment != rb.Spec.Environment ||
			b.Spec.EndpointRelease == "" {
			continue
		}
		release := &openchoreov1alpha1.EndpointRelease{}
		if err := r.Get(ctx, types.NamespacedName{Name: b.Spec.EndpointRelease, Namespace: b.Namespace}, release); err != nil {
			// A missing release is reported by the binding's own reconcile.
			continue
		}
		if routesToComponent(release, rb.Spec.Owner.ComponentName) {
			requests = append(requests, requestFor(b))
		}
	}
	return requests
}

// listBindingsForReleaseLine re-enqueues the EndpointReleaseBindings, in every
// environment, whose pinned release routes to the line, so a backend reported
// as ReleaseLineNotFound recovers when the line is created, and the reverse
// when it is deleted.
func (r *Reconciler) listBindingsForReleaseLine(ctx context.Context, obj client.Object) []reconcile.Request {
	line := obj.(*openchoreov1alpha1.ReleaseLine)
	logger := log.FromContext(ctx)

	bindings := &openchoreov1alpha1.EndpointReleaseBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(line.Namespace)); err != nil {
		logger.Error(err, "Failed to list EndpointReleaseBindings for ReleaseLine watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if b.Spec.Owner.ProjectName != line.Spec.Owner.ProjectName || b.Spec.EndpointRelease == "" {
			continue
		}
		release := &openchoreov1alpha1.EndpointRelease{}
		if err := r.Get(ctx, types.NamespacedName{Name: b.Spec.EndpointRelease, Namespace: b.Namespace}, release); err != nil {
			continue
		}
		if routesToReleaseLine(release, line.Spec.Owner.ComponentName, line.Name) {
			requests = append(requests, requestFor(b))
		}
	}
	return requests
}

// routesToReleaseLine reports whether any Component backend of the release
// names the component's line.
func routesToReleaseLine(release *openchoreov1alpha1.EndpointRelease, componentName, line string) bool {
	for _, ref := range release.Spec.BackendRefs {
		if ref.Component != nil && ref.Component.Name == componentName && ref.Component.ReleaseLine == line {
			return true
		}
	}
	return false
}

// routesToComponent reports whether any Component backend of the release names
// the component, on any of its lines.
func routesToComponent(release *openchoreov1alpha1.EndpointRelease, componentName string) bool {
	for _, ref := range release.Spec.BackendRefs {
		if ref.Component != nil && ref.Component.Name == componentName {
			return true
		}
	}
	return false
}

// listBindingsForEndpointRelease re-enqueues the bindings pinned to the
// release, waking a binding authored before the release it names was cut.
func (r *Reconciler) listBindingsForEndpointRelease(ctx context.Context, obj client.Object) []reconcile.Request {
	release := obj.(*openchoreov1alpha1.EndpointRelease)
	return r.ownedBindings(ctx, release.Namespace, release.Spec.Owner.EndpointName,
		func(b *openchoreov1alpha1.EndpointReleaseBinding) bool {
			return b.Spec.EndpointRelease == release.Name
		})
}

// listBindingsForEndpoint re-enqueues the Endpoint's bindings.
func (r *Reconciler) listBindingsForEndpoint(ctx context.Context, obj client.Object) []reconcile.Request {
	ep := obj.(*openchoreov1alpha1.Endpoint)
	return r.ownedBindings(ctx, ep.Namespace, ep.Name, nil)
}

// listBindingsForEnvironment re-enqueues bindings in the Environment's
// namespace that target it.
func (r *Reconciler) listBindingsForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	env := obj.(*openchoreov1alpha1.Environment)
	return r.bindingsFiltered(ctx, env.Namespace, func(b *openchoreov1alpha1.EndpointReleaseBinding) bool {
		return b.Spec.Environment == env.Name
	})
}

// listBindingsForProject re-enqueues bindings owned by the Project.
func (r *Reconciler) listBindingsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project := obj.(*openchoreov1alpha1.Project)
	return r.bindingsFiltered(ctx, project.Namespace, func(b *openchoreov1alpha1.EndpointReleaseBinding) bool {
		return b.Spec.Owner.ProjectName == project.Name
	})
}

// listBindingsForDataPlane re-enqueues bindings whose Environment uses the
// DataPlane.
func (r *Reconciler) listBindingsForDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	dp := obj.(*openchoreov1alpha1.DataPlane)
	return r.bindingsForReferencedEnvironments(ctx, dp.Namespace, func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind != openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == dp.Name
	})
}

// listBindingsForClusterDataPlane re-enqueues bindings, cluster-wide, whose
// Environment uses the ClusterDataPlane.
func (r *Reconciler) listBindingsForClusterDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	cdp := obj.(*openchoreov1alpha1.ClusterDataPlane)
	return r.bindingsForReferencedEnvironments(ctx, "", func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == cdp.Name
	})
}

// ownedBindings lists the Endpoint's bindings through the owner index,
// optionally narrowed by match.
func (r *Reconciler) ownedBindings(
	ctx context.Context,
	namespace, endpointName string,
	match func(*openchoreov1alpha1.EndpointReleaseBinding) bool,
) []reconcile.Request {
	bindings := &openchoreov1alpha1.EndpointReleaseBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(namespace),
		client.MatchingFields{controller.IndexKeyEndpointReleaseBindingOwnerEndpointName: endpointName}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list EndpointReleaseBindings by owner", "endpoint", endpointName)
		return nil
	}
	var requests []reconcile.Request
	for i := range bindings.Items {
		if match == nil || match(&bindings.Items[i]) {
			requests = append(requests, requestFor(&bindings.Items[i]))
		}
	}
	return requests
}

// bindingsForReferencedEnvironments lists Environments in namespace (or
// cluster-wide if empty) that match, then enqueues every binding targeting one
// of them in the Environment's own namespace.
func (r *Reconciler) bindingsForReferencedEnvironments(
	ctx context.Context,
	namespace string,
	match func(*openchoreov1alpha1.Environment) bool,
) []reconcile.Request {
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	envs := &openchoreov1alpha1.EnvironmentList{}
	if err := r.List(ctx, envs, opts...); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list Environments for EndpointReleaseBinding watch")
		return nil
	}

	matched := map[types.NamespacedName]struct{}{}
	for i := range envs.Items {
		if match(&envs.Items[i]) {
			matched[types.NamespacedName{Namespace: envs.Items[i].Namespace, Name: envs.Items[i].Name}] = struct{}{}
		}
	}
	if len(matched) == 0 {
		return nil
	}

	bindings := &openchoreov1alpha1.EndpointReleaseBindingList{}
	if err := r.List(ctx, bindings, opts...); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list EndpointReleaseBindings for environment watch")
		return nil
	}
	var requests []reconcile.Request
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if _, ok := matched[types.NamespacedName{Namespace: b.Namespace, Name: b.Spec.Environment}]; ok {
			requests = append(requests, requestFor(b))
		}
	}
	return requests
}

// bindingsFiltered lists bindings in namespace that match.
func (r *Reconciler) bindingsFiltered(
	ctx context.Context,
	namespace string,
	match func(*openchoreov1alpha1.EndpointReleaseBinding) bool,
) []reconcile.Request {
	bindings := &openchoreov1alpha1.EndpointReleaseBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list EndpointReleaseBindings")
		return nil
	}
	var requests []reconcile.Request
	for i := range bindings.Items {
		if match(&bindings.Items[i]) {
			requests = append(requests, requestFor(&bindings.Items[i]))
		}
	}
	return requests
}

func requestFor(b *openchoreov1alpha1.EndpointReleaseBinding) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: b.Name, Namespace: b.Namespace}}
}
