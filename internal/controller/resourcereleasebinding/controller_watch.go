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
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// resourceReleaseRefIndex indexes ResourceReleaseBinding by
	// spec.resourceRelease so the ResourceRelease watch mapper can answer
	// "which bindings pin this release?" with a single namespaced lookup.
	resourceReleaseRefIndex = "spec.resourceRelease"

	// environmentRefIndex indexes ResourceReleaseBinding by spec.environment
	// so the Environment and (Cluster)DataPlane watch mappers can answer
	// "which bindings target this environment?" with a single namespaced lookup.
	environmentRefIndex = "spec.environment"
)

// indexResourceReleaseRef extracts the spec.resourceRelease key from a
// binding. Bindings with an unset pin index to an empty key (filtered out
// in the mapper).
func indexResourceReleaseRef(obj client.Object) []string {
	binding := obj.(*openchoreov1alpha1.ResourceReleaseBinding)
	return []string{binding.Spec.ResourceRelease}
}

// indexEnvironmentRef extracts the spec.environment key from a binding.
func indexEnvironmentRef(obj client.Object) []string {
	binding := obj.(*openchoreov1alpha1.ResourceReleaseBinding)
	return []string{binding.Spec.Environment}
}

// setupResourceReleaseRefIndex registers the field index used by the
// ResourceRelease watch mapper.
func (r *Reconciler) setupResourceReleaseRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ResourceReleaseBinding{},
		resourceReleaseRefIndex, indexResourceReleaseRef)
}

// setupEnvironmentRefIndex registers the field index used by the
// Environment and (Cluster)DataPlane watch mappers.
func (r *Reconciler) setupEnvironmentRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ResourceReleaseBinding{},
		environmentRefIndex, indexEnvironmentRef)
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

// listResourceReleaseBindingsForEnvironment returns reconcile requests
// for ResourceReleaseBindings in the same namespace that target the supplied
// Environment via spec.environment. Create wakes bindings stuck on
// EnvironmentNotFound. Spec updates re-render the bindings too, since the
// Environment's gateway settings feed the render context.
func (r *Reconciler) listResourceReleaseBindingsForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	environment := obj.(*openchoreov1alpha1.Environment)

	var bindings openchoreov1alpha1.ResourceReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(environment.Namespace),
		client.MatchingFields{environmentRefIndex: environment.Name},
	); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list ResourceReleaseBindings by environment ref",
			"environment", environment.Name, "namespace", environment.Namespace)
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

// resourceReleaseBindingsForDataPlaneRef returns reconcile requests for the
// ResourceReleaseBindings whose target Environment references the given data
// plane (kind+name), so bindings stuck on DataPlaneNotFound are retried once the
// data plane appears. An empty namespace scans Environments cluster-wide (used
// for ClusterDataPlane). Environments with no dataPlaneRef resolve through the
// "default" DataPlane / ClusterDataPlane fallback, so they match any data plane
// named "default". This can enqueue a binding whose Environment actually
// resolves to the other kind; the extra reconcile is idempotent (it re-renders
// the same RenderedRelease).
func (r *Reconciler) resourceReleaseBindingsForDataPlaneRef(
	ctx context.Context, namespace string, kind openchoreov1alpha1.DataPlaneRefKind, name string,
) []reconcile.Request {
	var envs openchoreov1alpha1.EnvironmentList
	var listOpts []client.ListOption
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if err := r.List(ctx, &envs, listOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Environments for data plane change",
			"kind", kind, "dataPlane", name, "namespace", namespace)
		return nil
	}

	var requests []reconcile.Request
	for i := range envs.Items {
		env := &envs.Items[i]
		ref := env.Spec.DataPlaneRef
		if ref == nil {
			if name != controller.DefaultPlaneName {
				continue
			}
		} else if ref.Kind != kind || ref.Name != name {
			continue
		}
		requests = append(requests, r.listResourceReleaseBindingsForEnvironment(ctx, env)...)
	}
	return requests
}

// listResourceReleaseBindingsForDataPlane re-enqueues the bindings whose target
// Environment references this namespace-scoped DataPlane (or falls back to it
// as "default"; see resourceReleaseBindingsForDataPlaneRef).
func (r *Reconciler) listResourceReleaseBindingsForDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	dp := obj.(*openchoreov1alpha1.DataPlane)
	return r.resourceReleaseBindingsForDataPlaneRef(ctx, dp.Namespace, openchoreov1alpha1.DataPlaneRefKindDataPlane, dp.Name)
}

// listResourceReleaseBindingsForClusterDataPlane re-enqueues the bindings, cluster-wide,
// whose target Environment references this ClusterDataPlane (or falls back to
// it as "default"; see resourceReleaseBindingsForDataPlaneRef).
func (r *Reconciler) listResourceReleaseBindingsForClusterDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	cdp := obj.(*openchoreov1alpha1.ClusterDataPlane)
	return r.resourceReleaseBindingsForDataPlaneRef(ctx, "", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, cdp.Name)
}
