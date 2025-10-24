// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package addon

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconciler reconciles an Addon object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=addons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=addons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=addons/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	addon := &openchoreov1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling Addon", "name", addon.Name)

	// Update observedGeneration in status
	// Note: Validation is now handled by CEL validations at admission time,
	// so invalid resources are rejected before they reach the controller.
	if err := r.updateStatus(ctx, addon); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the Addon status
func (r *Reconciler) updateStatus(ctx context.Context, addon *openchoreov1alpha1.Addon) error {
	addon.Status.ObservedGeneration = addon.Generation
	return r.Status().Update(ctx, addon)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Addon{}).
		Named("addon").
		Complete(r)
}
