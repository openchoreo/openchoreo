// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ResourceReleaseBindingFinalizer gates cleanup of the underlying
	// RenderedRelease on the binding's effective retainPolicy.
	ResourceReleaseBindingFinalizer = "openchoreo.dev/resourcereleasebinding-cleanup"

	// requeueWaitForChild is how long we wait between reconciles while the
	// owned RenderedRelease is being torn down. Mirrors the Resource
	// controller's child-wait cadence.
	requeueWaitForChild = 5 * time.Second
)

// ensureFinalizer adds the resourcereleasebinding-cleanup finalizer when
// missing. The first return value indicates whether the finalizer was just
// added (caller should requeue to pick up the change).
func (r *Reconciler) ensureFinalizer(ctx context.Context, binding *openchoreov1alpha1.ResourceReleaseBinding) (bool, error) {
	if !binding.DeletionTimestamp.IsZero() {
		return false, nil
	}
	if controllerutil.AddFinalizer(binding, ResourceReleaseBindingFinalizer) {
		return true, r.Update(ctx, binding)
	}
	return false, nil
}

// finalize handles the deletion path. The flow:
//
//  1. Set the Finalizing condition the first time we observe deletion so the
//     status surfaces "deletion in progress" and exit (status update
//     re-enqueues).
//  2. Resolve the effective retainPolicy (binding override > snapshot
//     default > Delete).
//  3. Retain → set Reason=RetainHold and exit; PE clears the finalizer
//     manually after reclaiming DP-side state.
//  4. Delete → cascade-delete the owned RenderedRelease and requeue while
//     it still exists.
//  5. Once the RR is gone, remove the finalizer; K8s GCs the binding.
func (r *Reconciler) finalize(ctx context.Context, old, binding *openchoreov1alpha1.ResourceReleaseBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("binding", binding.Name)

	if !controllerutil.ContainsFinalizer(binding, ResourceReleaseBindingFinalizer) {
		logger.Info("Binding is being deleted but resourcereleasebinding-cleanup finalizer is not present; skipping cleanup")
		return ctrl.Result{}, nil
	}

	if meta.SetStatusCondition(&binding.Status.Conditions, NewFinalizingCondition(binding.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, binding)
	}

	retain, err := r.effectiveRetainPolicy(ctx, binding)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve effective retainPolicy: %w", err)
	}
	if retain == openchoreov1alpha1.ResourceRetainPolicyRetain {
		if controller.MarkTrueCondition(binding, ConditionFinalizing, ReasonRetainHold,
			"retainPolicy=Retain; finalizer held until cleared manually") {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, binding)
		}
		return ctrl.Result{}, nil
	}

	rrName := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{}
	err = r.Get(ctx, types.NamespacedName{Name: rrName, Namespace: binding.Namespace}, rr)
	switch {
	case err == nil:
		if rr.DeletionTimestamp.IsZero() {
			if delErr := r.Delete(ctx, rr); delErr != nil && !apierrors.IsNotFound(delErr) {
				return ctrl.Result{}, fmt.Errorf("delete RenderedRelease %q: %w", rrName, delErr)
			}
			logger.Info("Cascaded RenderedRelease delete", "renderedRelease", rrName)
		}
		return ctrl.Result{RequeueAfter: requeueWaitForChild}, nil
	case apierrors.IsNotFound(err):
		// fall through: clear finalizer
	default:
		return ctrl.Result{}, fmt.Errorf("get RenderedRelease %q: %w", rrName, err)
	}

	if controllerutil.RemoveFinalizer(binding, ResourceReleaseBindingFinalizer) {
		if err := r.Update(ctx, binding); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

// effectiveRetainPolicy resolves the policy in priority order:
//  1. binding override (spec.retainPolicy)
//  2. snapshot default (ResourceRelease.spec.resourceType.spec.retainPolicy)
//  3. universal default (Delete)
//
// A snapshot that is genuinely gone (NotFound) falls through to Delete —
// there is no record left to consult, and holding the binding open serves
// no purpose. A transient lookup failure (network blip, server timeout)
// returns an error so the caller can requeue without acting; treating it
// as Delete would cascade DP-side resources the PE may have intended to
// retain.
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
