// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

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
	// ResourceFinalizer ensures owned ResourceReleases are cleaned up and that
	// deletion blocks while ResourceReleaseBindings still reference the Resource.
	// Bindings are deleted externally; their own finalizers enforce retainPolicy.
	ResourceFinalizer = "openchoreo.dev/resource-cleanup"

	// requeueWaitForChildren is how long we wait between reconciles while owned
	// children (bindings or releases) are still being torn down. Matches
	// Component's 5s convention.
	requeueWaitForChildren = 5 * time.Second
)

// ensureFinalizer adds the resource-cleanup finalizer when missing. The first
// return value indicates whether the finalizer was just added (caller should
// requeue to pick up the change).
func (r *Reconciler) ensureFinalizer(ctx context.Context, res *openchoreov1alpha1.Resource) (bool, error) {
	if !res.DeletionTimestamp.IsZero() {
		return false, nil
	}
	if controllerutil.AddFinalizer(res, ResourceFinalizer) {
		return true, r.Update(ctx, res)
	}
	return false, nil
}

// finalize handles the deletion path.
//
//  1. Set the Finalizing condition the first time we observe deletion so users
//     see "deletion in progress" via status.
//  2. Block while ResourceReleaseBindings reference the Resource. Deletion of
//     bindings is externally driven; their own finalizers enforce retainPolicy.
//  3. Once bindings are gone, cascade-delete owned ResourceReleases.
//  4. Once releases are gone, remove the finalizer and let K8s GC the Resource.
func (r *Reconciler) finalize(ctx context.Context, old, res *openchoreov1alpha1.Resource) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("resource", res.Name)

	if !controllerutil.ContainsFinalizer(res, ResourceFinalizer) {
		// Either the finalizer was never added or another reconcile already
		// removed it. Log so an unexpected absence is visible in operator logs.
		logger.Info("Resource is being deleted but resource-cleanup finalizer is not present; skipping cleanup")
		return ctrl.Result{}, nil
	}

	// finalize uses UpdateStatusConditionsAndReturn directly rather than the
	// deferred whole-status writer used by reconcile: every exit path either
	// sets exactly one condition (Finalizing) and persists immediately, or
	// sets nothing.
	if meta.SetStatusCondition(&res.Status.Conditions, NewFinalizingCondition(res.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, res)
	}

	hasBindings, err := r.hasOwnedResourceReleaseBindings(ctx, res)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasBindings {
		logger.Info("Waiting for ResourceReleaseBindings to be deleted")
		return ctrl.Result{RequeueAfter: requeueWaitForChildren}, nil
	}

	deleted, err := r.deleteOwnedResourceReleases(ctx, res)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		return ctrl.Result{RequeueAfter: requeueWaitForChildren}, nil
	}

	if controllerutil.RemoveFinalizer(res, ResourceFinalizer) {
		if err := r.Update(ctx, res); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// hasOwnedResourceReleaseBindings returns true if any ResourceReleaseBinding in
// the same namespace still references the given Resource.
func (r *Reconciler) hasOwnedResourceReleaseBindings(ctx context.Context, res *openchoreov1alpha1.Resource) (bool, error) {
	bindings := &openchoreov1alpha1.ResourceReleaseBindingList{}
	if err := r.List(ctx, bindings,
		client.InNamespace(res.Namespace),
		client.MatchingFields{controller.IndexKeyResourceReleaseBindingOwnerResourceName: res.Name}); err != nil {
		return false, fmt.Errorf("list ResourceReleaseBindings: %w", err)
	}
	return len(bindings.Items) > 0, nil
}

// deleteOwnedResourceReleases triggers deletion of every ResourceRelease owned
// by the given Resource. Returns true if any were deleted; the caller should
// requeue to wait for the API server to GC them.
func (r *Reconciler) deleteOwnedResourceReleases(ctx context.Context, res *openchoreov1alpha1.Resource) (bool, error) {
	releases := &openchoreov1alpha1.ResourceReleaseList{}
	if err := r.List(ctx, releases,
		client.InNamespace(res.Namespace),
		client.MatchingFields{controller.IndexKeyResourceReleaseOwnerResourceName: res.Name}); err != nil {
		return false, fmt.Errorf("list ResourceReleases: %w", err)
	}
	if len(releases.Items) == 0 {
		return false, nil
	}
	for i := range releases.Items {
		if err := client.IgnoreNotFound(r.Delete(ctx, &releases.Items[i])); err != nil {
			return false, fmt.Errorf("delete ResourceRelease %s: %w", releases.Items[i].Name, err)
		}
	}
	return true, nil
}
