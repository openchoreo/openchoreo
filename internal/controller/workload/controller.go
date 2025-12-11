// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles a Workload object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Workload instance
	workload := &openchoreov1alpha1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workload")
		return ctrl.Result{}, err
	}

	if r.ensureLabels(workload) {
		if err := r.Update(ctx, workload); err != nil {
			logger.Error(err, "Failed to update Workload labels")
			return ctrl.Result{}, err
		}
		logger.Info("Updated Workload with required labels", "workload", workload.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// ensureLabels ensures that required labels are set on the Workload.
// Returns true if labels were updated.
func (r *Reconciler) ensureLabels(workload *openchoreov1alpha1.Workload) bool {
	return labels.SetLabels(&workload.ObjectMeta, labels.MakeWorkloadLabels(workload))
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Workload{}).
		Named("workload").
		Complete(r)
}
