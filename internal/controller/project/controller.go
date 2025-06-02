// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

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

// Reconciler reconciles a Project object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Project object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Project instance
	project := &choreov1.Project{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			// The Project resource may have been deleted since it triggered the reconcile
			logger.Info("Project resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Project")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := project.DeepCopy()

	// Handle the deletion of the project
	if !project.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing project")
		return r.finalize(ctx, old, project)
	}

	// Ensure the finalizer is added to the project
	if finalizerAdded, err := r.ensureFinalizer(ctx, project); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Handle creation of the project
	// Check if a condition exists already to determine if this is a first-time creation
	existingCondition := meta.FindStatusCondition(old.Status.Conditions, controller.TypeCreated)
	isNewResource := existingCondition == nil

	// Set the observed generation
	project.Status.ObservedGeneration = project.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&project.Status.Conditions,
		NewProjectCreatedCondition(project.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, project); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.Recorder.Event(project, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+project.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("project-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Project{}).
		Named("project").
		// Watch for Component changes to reconcile the project
		Watches(
			&choreov1.Component{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*choreov1.Component, *choreov1.Project](
				r.Client, controller.GetProject)),
		).
		Complete(r)
}
