// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentenvsnapshot

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconciler reconciles a ComponentEnvSnapshot object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=envsettings,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=addons,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	snapshot := &openchoreov1alpha1.ComponentEnvSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, snapshot); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling ComponentEnvSnapshot (stub - rendering not implemented yet)",
		"name", snapshot.Name,
		"component", snapshot.Spec.Owner.ComponentName,
		"environment", snapshot.Spec.Environment)

	// TODO Phase 7: Implement rendering logic here
	// For now, just update the observed generation

	snapshot.Status.ObservedGeneration = snapshot.Generation

	if err := r.Status().Update(ctx, snapshot); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	// Set up field indexes for efficient lookups
	if err := r.setupComponentNameRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup component name reference index: %w", err)
	}

	if err := r.setupEnvironmentRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup environment reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ComponentEnvSnapshot{}).
		Watches(&openchoreov1alpha1.EnvSettings{},
			handler.EnqueueRequestsFromMapFunc(r.listSnapshotsForEnvSettings)).
		Watches(&openchoreov1alpha1.Addon{},
			handler.EnqueueRequestsFromMapFunc(r.listSnapshotsUsingAddon)).
		Named("componentenvsnapshot").
		Complete(r)
}
