// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
//  2. Find the RenderedRelease the binding controls. One with its name that
//     another object controls is treated as absent and never deleted.
//  3. If deleting it could remove what Manage mode applied, resolve the
//     effective retainPolicy (binding override > snapshot default > Delete).
//     Retain → set Reason=RetainHold and exit; PE clears the finalizer
//     manually after reclaiming DP-side state. Observe mode with nothing
//     applied skips this: there is nothing to keep.
//  4. Cascade-delete the owned RenderedRelease and requeue while it still
//     exists.
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

	rr, err := r.controlledRelease(ctx, binding)
	if err != nil {
		return ctrl.Result{}, err
	}

	if retainMayApply(binding, rr) {
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
	}

	if rr != nil {
		if rr.DeletionTimestamp.IsZero() {
			if delErr := r.Delete(ctx, rr); delErr != nil && !apierrors.IsNotFound(delErr) {
				return ctrl.Result{}, fmt.Errorf("delete RenderedRelease %q: %w", rr.Name, delErr)
			}
			logger.Info("Cascaded RenderedRelease delete", "renderedRelease", rr.Name)
		}
		return ctrl.Result{RequeueAfter: requeueWaitForChild}, nil
	}

	if controllerutil.RemoveFinalizer(binding, ResourceReleaseBindingFinalizer) {
		if err := r.Update(ctx, binding); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

// controlledRelease returns the binding's RenderedRelease if the binding
// controls it, and nil when there is none or another object controls a release
// with that name.
func (r *Reconciler) controlledRelease(ctx context.Context,
	binding *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
	name := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: binding.Namespace}, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get RenderedRelease %q: %w", name, err)
	}
	if !metav1.IsControlledBy(rr, binding) {
		return nil, nil
	}
	return rr, nil
}

// retainMayApply reports whether deleting the binding could remove what Manage
// mode applied, so that its retainPolicy decides. A binding in Observe mode
// without a release, and a release in Observe mode that applied nothing, have
// nothing to keep. A binding in Manage mode holds without a release, as
// before.
func retainMayApply(binding *openchoreov1alpha1.ResourceReleaseBinding, rr *openchoreov1alpha1.RenderedRelease) bool {
	if rr == nil {
		return !binding.Spec.IsObserve()
	}
	return rr.Spec.Mode != openchoreov1alpha1.RenderedReleaseModeObserve || len(rr.Status.Resources) > 0
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
