// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ProjectReleaseBindingFinalizer gates deletion of the binding on cleanup
	// of its owned RenderedRelease. The binding must outlive its RenderedRelease
	// so the RenderedRelease finalizer can resolve the data-plane client through
	// the (still-present) Environment during its own teardown.
	ProjectReleaseBindingFinalizer = "openchoreo.dev/projectreleasebinding-cleanup"

	// requeueWaitForChild is how long we wait between reconciles while the
	// owned RenderedRelease is being torn down.
	requeueWaitForChild = 5 * time.Second
)

// ensureFinalizer adds the projectreleasebinding-cleanup finalizer when
// missing. The first return value indicates whether the finalizer was just
// added (caller should requeue to pick up the change).
func (r *Reconciler) ensureFinalizer(ctx context.Context, binding *openchoreov1alpha1.ProjectReleaseBinding) (bool, error) {
	if !binding.DeletionTimestamp.IsZero() {
		return false, nil
	}
	if controllerutil.AddFinalizer(binding, ProjectReleaseBindingFinalizer) {
		return true, r.Update(ctx, binding)
	}
	return false, nil
}

// finalize handles the deletion path. The flow:
//
//  1. Set the Finalizing condition the first time we observe deletion so the
//     status surfaces "deletion in progress" and exit (status update
//     re-enqueues).
//  2. Cascade-delete the owned RenderedRelease and requeue while it still
//     exists.
//  3. Once the RenderedRelease is gone, remove the finalizer; K8s GCs the
//     binding.
func (r *Reconciler) finalize(ctx context.Context, old, binding *openchoreov1alpha1.ProjectReleaseBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("binding", binding.Name)

	if !controllerutil.ContainsFinalizer(binding, ProjectReleaseBindingFinalizer) {
		return ctrl.Result{}, nil
	}

	if meta.SetStatusCondition(&binding.Status.Conditions, NewFinalizingCondition(binding.Generation)) {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, old, binding)
	}

	rrName := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{}
	err := r.Get(ctx, types.NamespacedName{Name: rrName, Namespace: binding.Namespace}, rr)
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

	if controllerutil.RemoveFinalizer(binding, ProjectReleaseBindingFinalizer) {
		if err := r.Update(ctx, binding); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}
