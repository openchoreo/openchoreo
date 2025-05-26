// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a DeploymentPipeline object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=deploymentpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=deploymentpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=deploymentpipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DeploymentPipeline object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DeploymentPipeline instance
	deploymentPipeline := &choreov1.DeploymentPipeline{}
	if err := r.Get(ctx, req.NamespacedName, deploymentPipeline); err != nil {
		if apierrors.IsNotFound(err) {
			// The DeploymentPipeline resource may have been deleted since it triggered the reconcile
			logger.Info("DeploymentPipeline resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get DeploymentPipeline")
		return ctrl.Result{}, err
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
		For(&choreov1.DeploymentPipeline{}).
		Named("deploymentpipeline").
		Complete(r)
}
