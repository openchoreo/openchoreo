// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// ConditionAvailable reports that the ClusterHook was admitted and can be bound.
	ConditionAvailable = "Available"
	// ReasonAvailable is the reason set on ConditionAvailable.
	ReasonAvailable = "ClusterHookAvailable"
)

// Reconciler reconciles a ClusterHook object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterhooks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterhooks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterhooks/finalizers,verbs=update

// Reconcile marks the ClusterHook Available; validation happens in the admission webhook
// and execution in the ReleaseBinding controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	hook := &openchoreov1alpha1.ClusterHook{}
	if err := r.Get(ctx, req.NamespacedName, hook); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !hook.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	old := hook.DeepCopy()
	hook.Status.ObservedGeneration = hook.Generation
	meta.SetStatusCondition(&hook.Status.Conditions, metav1.Condition{
		Type:               ConditionAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonAvailable,
		Message:            "ClusterHook is available for binding",
		ObservedGeneration: hook.Generation,
	})
	if old.Status.ObservedGeneration == hook.Status.ObservedGeneration &&
		meta.IsStatusConditionTrue(old.Status.Conditions, ConditionAvailable) {
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, hook); err != nil {
		logger.Error(err, "Failed to update ClusterHook status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ClusterHook{}).
		Named("clusterhook").
		Complete(r)
}
