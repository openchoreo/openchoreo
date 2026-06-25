// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	cond := meta.FindStatusCondition(res.Status.Conditions, string(ConditionFinalizing))
	if cond == nil || cond.Status != metav1.ConditionTrue {
		if meta.SetStatusCondition(&res.Status.Conditions, NewFinalizingCondition(res.Generation)) {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, res)
		}
	}

	hasBindings, err := r.deleteOwnedResourceReleaseBindingsAndWait(ctx, res)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasBindings {
		logger.Info("Waiting for ResourceReleaseBindings to be deleted")
		controller.MarkTrueCondition(res, ConditionFinalizing, ReasonFinalizing, "Waiting for ResourceReleaseBindings to be deleted")
		if !equality.Semantic.DeepEqual(old.Status.Conditions, res.Status.Conditions) {
			if err := controller.UpdateStatusConditions(ctx, r.Client, old, res); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: requeueWaitForChildren}, nil
	}

	deleted, err := r.deleteOwnedResourceReleases(ctx, res)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		logger.Info("Waiting for ResourceReleases to be deleted")
		controller.MarkTrueCondition(res, ConditionFinalizing, ReasonFinalizing, "Waiting for ResourceReleases to be deleted")
		if !equality.Semantic.DeepEqual(old.Status.Conditions, res.Status.Conditions) {
			if err := controller.UpdateStatusConditions(ctx, r.Client, old, res); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: requeueWaitForChildren}, nil
	}

	if controllerutil.RemoveFinalizer(res, ResourceFinalizer) {
		if err := r.Update(ctx, res); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// deleteOwnedResourceReleaseBindingsAndWait triggers deletion of every ResourceReleaseBinding
// owned by the given Resource depending on its retention policy. Returns true if any
// bindings still exist; the caller should requeue to wait for them to be deleted.
func (r *Reconciler) deleteOwnedResourceReleaseBindingsAndWait(ctx context.Context, res *openchoreov1alpha1.Resource) (bool, error) {
	bindings := &openchoreov1alpha1.ResourceReleaseBindingList{}
	if err := r.List(ctx, bindings,
		client.InNamespace(res.Namespace),
		client.MatchingFields{controller.IndexKeyResourceReleaseBindingOwnerResourceName: res.Name}); err != nil {
		return false, fmt.Errorf("list ResourceReleaseBindings: %w", err)
	}
	if len(bindings.Items) == 0 {
		return false, nil
	}
	for i := range bindings.Items {
		binding := &bindings.Items[i]
		if !binding.DeletionTimestamp.IsZero() {
			continue
		}
		retain, err := r.effectiveRetainPolicy(ctx, binding)
		if err != nil {
			return false, fmt.Errorf("resolve effective retainPolicy for binding %s: %w", binding.Name, err)
		}
		if retain == openchoreov1alpha1.ResourceRetainPolicyRetain {
			continue
		}
		if err := r.Delete(ctx, binding); err != nil {
			return false, fmt.Errorf("delete ResourceReleaseBinding %s: %w", binding.Name, err)
		}
	}
	return true, nil
}

// effectiveRetainPolicy resolves the policy in priority order:
//  1. binding override (spec.retainPolicy)
//  2. snapshot default (ResourceRelease.spec.resourceType.spec.retainPolicy)
//  3. universal default (Delete)
func (r *Reconciler) effectiveRetainPolicy(ctx context.Context, binding *openchoreov1alpha1.ResourceReleaseBinding) (openchoreov1alpha1.ResourceRetainPolicy, error) {
	if binding.Spec.RetainPolicy != "" {
		return binding.Spec.RetainPolicy, nil
	}
	if binding.Spec.ResourceRelease == "" {
		return openchoreov1alpha1.ResourceRetainPolicyDelete, nil
	}
	rr := &openchoreov1alpha1.ResourceRelease{}
	err := r.Get(ctx, client.ObjectKey{Name: binding.Spec.ResourceRelease, Namespace: binding.Namespace}, rr)
	switch {
	case err == nil:
		if rr.Spec.ResourceType.Spec.RetainPolicy != "" {
			return rr.Spec.ResourceType.Spec.RetainPolicy, nil
		}
		return openchoreov1alpha1.ResourceRetainPolicyDelete, nil
	case apierrors.IsNotFound(err):
		return openchoreov1alpha1.ResourceRetainPolicyDelete, nil
	default:
		return "", fmt.Errorf("get ResourceRelease %q: %w", binding.Spec.ResourceRelease, err)
	}
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
