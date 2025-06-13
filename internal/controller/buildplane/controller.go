// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"context"
	"fmt"
	"github.com/openchoreo/openchoreo/internal/controller"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a BuildPlane object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BuildPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the BuildPlane instance
	buildPlane := &choreov1.BuildPlane{}
	if err := r.Get(ctx, req.NamespacedName, buildPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// The BuildPlane resource may have been deleted since it triggered the reconcile
			logger.Info("BuildPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get BuildPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old BuildPlane object
	old := buildPlane.DeepCopy()

	// Handle create
	// Ignore reconcile if the BuildPlane is already available since this is a one-time create
	if r.shouldIgnoreReconcile(buildPlane) {
		return ctrl.Result{}, nil
	}

	// Set the observed generation
	buildPlane.Status.ObservedGeneration = buildPlane.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&buildPlane.Status.Conditions,
		NewBuildPlaneCreatedCondition(buildPlane.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, buildPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(buildPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", buildPlane.Name))

	return ctrl.Result{}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(buildplane *choreov1.BuildPlane) bool {
	return meta.FindStatusCondition(buildplane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.BuildPlane{}).
		Named("buildplane").
		Complete(r)
}
