// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildv2

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"

	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a BuildV2 object
type Reconciler struct {
	k8sClientMgr *kubernetesClient.KubeMultiClientManager
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
	// In GitOps mode, the controller will not create or update resources directly in the cluster,
	// but will instead generate the necessary manifests and creates GitCommitRequests to update the Git repository.
	IsGitOpsMode bool
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildv2s,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildv2s/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildv2s/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("buildv2", req.NamespacedName)

	// Fetch the build resource
	build := &choreov1.BuildV2{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("BuildV2 resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get BuildV2")
		return ctrl.Result{}, err
	}

	// Get build plane
	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, build)
	if err != nil {
		logger.Error(err, "Cannot retrieve the build plane")
		return r.updateStatusWithError(ctx, build, "BuildPlaneNotFound", fmt.Sprintf("Failed to get build plane: %v", err))
	}

	// Get build plane client
	bpClient, err := r.getBPClient(ctx, buildPlane)
	if err != nil {
		logger.Error(err, "Error in getting build plane client")
		return r.updateStatusWithError(ctx, build, "BuildPlaneClientError", fmt.Sprintf("Failed to get build plane client: %v", err))
	}

	// Create or update workflow
	return r.reconcileWorkflow(ctx, build, bpClient, logger)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.k8sClientMgr == nil {
		r.k8sClientMgr = kubernetesClient.NewManager()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.BuildV2{}).
		Named("buildv2").
		Complete(r)
}

func (r *Reconciler) getBPClient(ctx context.Context, buildPlane *choreov1.BuildPlane) (client.Client, error) {
	bpClient, err := kubernetesClient.GetK8sClient(r.k8sClientMgr, buildPlane.Spec.Owner.OrganizationName, buildPlane.Name, buildPlane.Spec.KubernetesCluster)
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Failed to get build plane client")
		return nil, err
	}
	return bpClient, nil
}

// reconcileWorkflow creates or updates the workflow for the build
func (r *Reconciler) reconcileWorkflow(ctx context.Context, build *choreov1.BuildV2, bpClient client.Client, logger logr.Logger) (ctrl.Result, error) {
	workflow := makeArgoWorkflow(build)

	// Create workflow - ignore if already exists
	logger.Info("Creating workflow", "workflow", workflow.Name)
	if err := bpClient.Create(ctx, workflow); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.V(1).Info("Workflow already exists", "workflow", workflow.Name)
			return r.updateStatusWithSuccess(ctx, build, "WorkflowReady", "Workflow already exists")
		}
		logger.Error(err, "Failed to create workflow")
		return r.updateStatusWithError(ctx, build, "WorkflowCreationFailed", fmt.Sprintf("Failed to create workflow: %v", err))
	}

	return r.updateStatusWithSuccess(ctx, build, "WorkflowCreated", "Workflow created successfully")
}

// updateStatusWithError updates the build status with error information
func (r *Reconciler) updateStatusWithError(ctx context.Context, build *choreov1.BuildV2, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	//if err := r.Status().Update(ctx, build); err != nil {
	//	logger.Error(err, "Failed to update build status")
	//	return ctrl.Result{}, err
	//}

	logger.Info("Updated build status with error", "reason", reason, "message", message)
	return ctrl.Result{Requeue: true}, nil
}

// updateStatusWithSuccess updates the build status with success information
func (r *Reconciler) updateStatusWithSuccess(ctx context.Context, build *choreov1.BuildV2, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	//if err := r.Status().Update(ctx, build); err != nil {
	//	logger.Error(err, "Failed to update build status")
	//	return ctrl.Result{}, err
	//}

	logger.V(1).Info("Updated build status with success", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}
