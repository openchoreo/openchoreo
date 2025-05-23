/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package deploymenttrack

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

// Reconciler reconciles a DeploymentTrack object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DeploymentTrack object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling deploymentTrack")

	// Fetch the DeploymentTrack instance
	deploymentTrack := &choreov1.DeploymentTrack{}
	if err := r.Get(ctx, req.NamespacedName, deploymentTrack); err != nil {
		if apierrors.IsNotFound(err) {
			// The DeploymentTrack resource may have been deleted since it triggered the reconcile
			logger.Info("DeploymentTrack resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get DeploymentTrack")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := deploymentTrack.DeepCopy()

	// Handle the deletion of the deploymentTrack
	if !deploymentTrack.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing deploymentTrack")
		return r.finalize(ctx, old, deploymentTrack)
	}

	// Ensure the finalizer is added to the deploymentTrack
	if finalizerAdded, err := r.ensureFinalizer(ctx, deploymentTrack); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Handle create
	// Check if a condition exists already to determine if this is a first-time creation
	existingCondition := meta.FindStatusCondition(old.Status.Conditions, controller.TypeAvailable)
	isNewResource := existingCondition == nil

	// Set the observed generation
	deploymentTrack.Status.ObservedGeneration = deploymentTrack.Generation

	// Update the status condition to indicate the deploymentTrack is available
	meta.SetStatusCondition(
		&deploymentTrack.Status.Conditions,
		NewDeploymentTrackAvailableCondition(deploymentTrack.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, deploymentTrack); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.recorder.Event(deploymentTrack, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+deploymentTrack.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.recorder == nil {
		r.recorder = mgr.GetEventRecorderFor("deploymentTrack-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.DeploymentTrack{}).
		Named("deploymenttrack").
		// Watch for Build changes to reconcile the Deployment Track
		Watches(
			&choreov1.Build{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*choreov1.Build, *choreov1.DeploymentTrack](
				r.Client, controller.GetDeploymentTrack)),
		).
		// Watch for DeployableArtifact changes to reconcile the Deployment Track
		Watches(
			&choreov1.DeployableArtifact{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*choreov1.DeployableArtifact, *choreov1.DeploymentTrack](
				r.Client, controller.GetDeploymentTrack)),
		).
		// Watch for Deployment changes to reconcile the Deployment Track
		Watches(
			&choreov1.Deployment{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*choreov1.Deployment, *choreov1.DeploymentTrack](
				r.Client, controller.GetDeploymentTrack)),
		).
		Complete(r)
}
