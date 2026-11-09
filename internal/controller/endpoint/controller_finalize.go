// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
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
	// EndpointFinalizer ensures an Endpoint's EndpointReleaseBindings and
	// EndpointReleases are cleaned up before the Endpoint goes away. Bindings
	// tear down their own RenderedReleases through their finalizers.
	EndpointFinalizer = "openchoreo.dev/endpoint-cleanup"

	// requeueWaitForChildren is how long we wait between reconciles while owned
	// children (bindings or releases) are still being torn down. Matches
	// Resource's 5s convention.
	requeueWaitForChildren = 5 * time.Second
)

// ensureFinalizer adds the endpoint-cleanup finalizer when missing. The first
// return value indicates whether the finalizer was just added (caller should
// requeue to pick up the change).
func (r *Reconciler) ensureFinalizer(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (bool, error) {
	if !ep.DeletionTimestamp.IsZero() {
		return false, nil
	}
	if controllerutil.AddFinalizer(ep, EndpointFinalizer) {
		return true, r.Update(ctx, ep)
	}
	return false, nil
}

// finalize handles the deletion path.
//
//  1. Set the Finalizing condition the first time we observe deletion so users
//     see "deletion in progress" via status.
//  2. Delete the Endpoint's EndpointReleaseBindings and wait for them to go.
//     Unlike a Resource, an Endpoint provisions nothing worth retaining: its
//     rendered routes stop being meaningful once the Endpoint is gone.
//  3. Once bindings are gone, delete owned EndpointReleases and wait.
//  4. Once releases are gone, remove the finalizer and let K8s GC the Endpoint.
func (r *Reconciler) finalize(ctx context.Context, old, ep *openchoreov1alpha1.Endpoint) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("endpoint", ep.Name)

	if !controllerutil.ContainsFinalizer(ep, EndpointFinalizer) {
		logger.Info("Endpoint is being deleted but endpoint-cleanup finalizer is not present; skipping cleanup")
		return ctrl.Result{}, nil
	}

	cond := meta.FindStatusCondition(ep.Status.Conditions, string(ConditionFinalizing))
	if cond == nil || cond.Status != metav1.ConditionTrue {
		if meta.SetStatusCondition(&ep.Status.Conditions, NewFinalizingCondition(ep.Generation)) {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, ep)
		}
	}

	pending, err := r.deleteOwned(ctx, ep, &openchoreov1alpha1.EndpointReleaseBindingList{},
		controller.IndexKeyEndpointReleaseBindingOwnerEndpointName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		return r.waitForChildren(ctx, old, ep, "Waiting for EndpointReleaseBindings to be deleted")
	}

	pending, err = r.deleteOwned(ctx, ep, &openchoreov1alpha1.EndpointReleaseList{},
		controller.IndexKeyEndpointReleaseOwnerEndpointName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		return r.waitForChildren(ctx, old, ep, "Waiting for EndpointReleases to be deleted")
	}

	if controllerutil.RemoveFinalizer(ep, EndpointFinalizer) {
		if err := r.Update(ctx, ep); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// waitForChildren records why deletion is blocked and requeues.
func (r *Reconciler) waitForChildren(
	ctx context.Context, old, ep *openchoreov1alpha1.Endpoint, msg string,
) (ctrl.Result, error) {
	log.FromContext(ctx).Info(msg, "endpoint", ep.Name)
	controller.MarkTrueCondition(ep, ConditionFinalizing, ReasonFinalizing, msg)
	if !equality.Semantic.DeepEqual(old.Status.Conditions, ep.Status.Conditions) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, ep); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: requeueWaitForChildren}, nil
}

// deleteOwned triggers deletion of every object in list owned by the given
// Endpoint, located through the owner-name field index. Returns true while any
// still exist; the caller should requeue until they are gone.
func (r *Reconciler) deleteOwned(
	ctx context.Context, ep *openchoreov1alpha1.Endpoint, list client.ObjectList, index string,
) (bool, error) {
	if err := r.List(ctx, list, client.InNamespace(ep.Namespace), client.MatchingFields{index: ep.Name}); err != nil {
		return false, fmt.Errorf("list %T: %w", list, err)
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return false, fmt.Errorf("extract %T: %w", list, err)
	}
	if len(items) == 0 {
		return false, nil
	}
	for _, item := range items {
		obj := item.(client.Object)
		if !obj.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, obj)); err != nil {
			return false, fmt.Errorf("delete %s: %w", obj.GetName(), err)
		}
	}
	return true, nil
}
