// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconciler reconciles a ComponentRelease object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// No-op controller - ComponentReleases are managed externally
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ComponentRelease{}).
		Named("componentrelease").
		Complete(r)
}
