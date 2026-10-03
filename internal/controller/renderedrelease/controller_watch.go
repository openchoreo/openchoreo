// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconcile resolves the target plane client from the RenderedRelease's
// EnvironmentName at apply time (see getDPClient/getOPClient), but this
// controller only watches RenderedRelease itself. Repointing a DataPlane
// (or ClusterDataPlane) to a different physical cluster leaves the rendered
// manifests byte-identical, so the owning ProjectReleaseBinding's re-render
// is a no-op and never produces an event here. Without the handlers below,
// the release only picks up the new plane client on its next periodic
// requeue (or never, if Spec.Interval disables it), leaving the namespace
// and workloads never (re)created on the new cluster. This closes that loop.

// releasesForDataPlane re-enqueues every RenderedRelease in the DataPlane's
// namespace whose referenced Environment uses this DataPlane.
func (r *Reconciler) releasesForDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	dp := obj.(*openchoreov1alpha1.DataPlane)
	return r.releasesForReferencedEnvironments(ctx, dp.Namespace, func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind != openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == dp.Name
	})
}

// releasesForClusterDataPlane re-enqueues every RenderedRelease (cluster-wide)
// whose referenced Environment uses this ClusterDataPlane.
func (r *Reconciler) releasesForClusterDataPlane(ctx context.Context, obj client.Object) []reconcile.Request {
	cdp := obj.(*openchoreov1alpha1.ClusterDataPlane)
	return r.releasesForReferencedEnvironments(ctx, "", func(env *openchoreov1alpha1.Environment) bool {
		ref := env.Spec.DataPlaneRef
		return ref != nil &&
			ref.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane &&
			ref.Name == cdp.Name
	})
}

// releasesForReferencedEnvironments lists Environments in `namespace` (or
// cluster-wide if empty) whose spec matches the predicate, then enqueues
// every RenderedRelease whose spec.environmentName points to one of those
// envs in the env's own namespace.
func (r *Reconciler) releasesForReferencedEnvironments(
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
		log.FromContext(ctx).Error(err, "Failed to list Environments for RenderedRelease watch")
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

	releases := &openchoreov1alpha1.RenderedReleaseList{}
	if err := r.List(ctx, releases, envListOpts...); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list RenderedReleases for RenderedRelease watch")
		return nil
	}
	var requests []reconcile.Request
	for i := range releases.Items {
		rel := &releases.Items[i]
		if _, ok := matched[nsenv{namespace: rel.Namespace, name: rel.Spec.EnvironmentName}]; ok {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: rel.Name, Namespace: rel.Namespace},
			})
		}
	}
	return requests
}
