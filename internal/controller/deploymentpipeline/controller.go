// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a DeploymentPipeline object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=deploymentpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=deploymentpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=deploymentpipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	deploymentPipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := r.Get(ctx, req.NamespacedName, deploymentPipeline); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DeploymentPipeline resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get DeploymentPipeline")
		return ctrl.Result{}, err
	}

	// Add finalizer if not being deleted
	if deploymentPipeline.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(deploymentPipeline, PipelineCleanupFinalizer) {
			if err := r.Update(ctx, deploymentPipeline); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion
	if !deploymentPipeline.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, deploymentPipeline)
	}

	previousCondition := meta.FindStatusCondition(deploymentPipeline.Status.Conditions, controller.TypeAvailable)

	deploymentPipeline.Status.ObservedGeneration = deploymentPipeline.Generation
	if err := controller.UpdateCondition(
		ctx,
		r.Status(),
		deploymentPipeline,
		&deploymentPipeline.Status.Conditions,
		controller.TypeAvailable,
		metav1.ConditionTrue,
		"DeploymentPipelineAvailable",
		"DeploymentPipeline is available",
	); err != nil {
		return ctrl.Result{}, err
	} else {
		if previousCondition == nil {
			r.Recorder.Event(deploymentPipeline, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+deploymentPipeline.Name)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("deploymentPipeline-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.DeploymentPipeline{}).
		Named("deploymentpipeline").
		Complete(r)
}
