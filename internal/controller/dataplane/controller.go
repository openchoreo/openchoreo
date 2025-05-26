// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a DataPlane object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DataPlane instance
	dataPlane := &choreov1.DataPlane{}
	if err := r.Get(ctx, req.NamespacedName, dataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// The DataPlane resource may have been deleted since it triggered the reconcile
			logger.Info("DataPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get DataPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old DataPlane object
	old := dataPlane.DeepCopy()

	// Handle the deletion of the dataplane
	if !dataPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing dataplane")
		return r.finalize(ctx, old, dataPlane)
	}

	// Ensure the finalizer is added to the dataplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, dataPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Handle create
	// Ignore reconcile if the Dataplane is already available since this is a one-time create
	if r.shouldIgnoreReconcile(dataPlane) {
		return ctrl.Result{}, nil
	}

	// Set the observed generation
	dataPlane.Status.ObservedGeneration = dataPlane.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&dataPlane.Status.Conditions,
		NewDataPlaneCreatedCondition(dataPlane.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, dataPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(dataPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", dataPlane.Name))

	return ctrl.Result{}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(dataPlane *choreov1.DataPlane) bool {
	return meta.FindStatusCondition(dataPlane.Status.Conditions, string(controller.TypeAvailable)) != nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("dataplane-controller")
	}

	// Set up the index for the environment reference
	if err := r.setupDataPlaneRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup dataPlane reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.DataPlane{}).
		Named("dataplane").
		// Watch for Environment changes to reconcile the dataplane
		Watches(
			&choreov1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.GetDataPlaneForEnvironment),
		).
		Complete(r)
}
