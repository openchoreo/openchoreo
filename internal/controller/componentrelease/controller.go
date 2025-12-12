// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

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
	logger := log.FromContext(ctx)

	// Fetch the ComponentRelease instance
	componentRelease := &openchoreov1alpha1.ComponentRelease{}
	if err := r.Get(ctx, req.NamespacedName, componentRelease); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ComponentRelease")
		return ctrl.Result{}, err
	}

	if r.ensureLabels(componentRelease) {
		if err := r.Update(ctx, componentRelease); err != nil {
			logger.Error(err, "Failed to update ComponentRelease labels")
			return ctrl.Result{}, err
		}
		logger.Info("Updated ComponentRelease with required labels", "componentRelease", componentRelease.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// ensureLabels ensures that required labels are set on the ComponentRelease.
// Returns true if labels were updated.
func (r *Reconciler) ensureLabels(cr *openchoreov1alpha1.ComponentRelease) bool {
	return labels.SetLabels(&cr.ObjectMeta, labels.MakeComponentReleaseLabels(cr))
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ComponentRelease{}).
		Named("componentrelease").
		Complete(r)
}
