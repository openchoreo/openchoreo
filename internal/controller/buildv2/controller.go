// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildv2

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

// Reconciler reconciles a BuildV2 object
type Reconciler struct {
	k8sClientMgr *kubernetesClient.KubeMultiClientManager
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
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

	oldBuild := build.DeepCopy()

	// Check if we should ignore reconciliation
	if shouldIgnoreReconcile(build) {
		return ctrl.Result{}, nil
	}

	// Set BuildInitiated condition if not already set
	if !isBuildInitiated(build) {
		setBuildInitiatedCondition(build)
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}

	// Get build plane
	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, build)
	if err != nil {
		logger.Error(err, "Cannot retrieve the build plane")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	}

	// Get build plane client
	bpClient, err := r.getBPClient(ctx, buildPlane)
	if err != nil {
		logger.Error(err, "Error in getting build plane client")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	}

	// Create prerequisite resources (namespace, RBAC)
	if err := r.ensurePrerequisiteResources(ctx, bpClient, build, logger); err != nil {
		logger.Error(err, "Error ensuring prerequisite resources")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	}

	workflow, created, err := r.ensureWorkflow(ctx, build, bpClient)
	if err != nil {
		logger.Error(err, "cannot ensure workflow")
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
	if created {
		setBuildTriggeredCondition(build)
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}

	// Update build status based on workflow status
	return r.updateBuildStatus(ctx, oldBuild, build, workflow)
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

// ensurePrerequisiteResources ensures that all prerequisite resources exist for the workflow
func (r *Reconciler) ensurePrerequisiteResources(ctx context.Context, bpClient client.Client, build *choreov1.BuildV2, logger logr.Logger) error {
	// Create namespace
	namespace := makeNamespace(build)
	if err := r.ensureResource(ctx, bpClient, namespace, "Namespace", logger); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Create service account
	serviceAccount := makeServiceAccount(build)
	if err := r.ensureResource(ctx, bpClient, serviceAccount, "ServiceAccount", logger); err != nil {
		return fmt.Errorf("failed to ensure service account: %w", err)
	}

	// Create role
	role := makeRole(build)
	if err := r.ensureResource(ctx, bpClient, role, "Role", logger); err != nil {
		return fmt.Errorf("failed to ensure role: %w", err)
	}

	// Create role binding
	roleBinding := makeRoleBinding(build)
	if err := r.ensureResource(ctx, bpClient, roleBinding, "RoleBinding", logger); err != nil {
		return fmt.Errorf("failed to ensure role binding: %w", err)
	}

	return nil
}

// ensureResource creates a resource if it doesn't exist, ignoring "already exists" errors
func (r *Reconciler) ensureResource(ctx context.Context, bpClient client.Client, obj client.Object, resourceType string, logger logr.Logger) error {
	err := bpClient.Create(ctx, obj)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.V(1).Info("Resource already exists", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
			return nil
		}
		logger.Error(err, "Failed to create resource", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
		return err
	}
	logger.Info("Created resource", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
	return nil
}

// ensureWorkflow fetches the Argo Workflow; if it doesn't exist it creates one.
// Returns (workflow, created, error)
func (r *Reconciler) ensureWorkflow(
	ctx context.Context,
	build *choreov1.BuildV2,
	bpClient client.Client,
) (*argoproj.Workflow, bool, error) {

	wf := &argoproj.Workflow{}
	err := bpClient.Get(ctx,
		client.ObjectKey{Name: makeWorkflowName(build), Namespace: makeNamespaceName(build)},
		wf,
	)

	if err == nil || apierrors.IsAlreadyExists(err) {
		return wf, false, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, false, err
	}

	wf = makeArgoWorkflow(build)
	if err := bpClient.Create(ctx, wf); err != nil {
		return nil, false, err
	}
	return wf, true, nil
}

// updateBuildStatus updates build status based on workflow status
func (r *Reconciler) updateBuildStatus(ctx context.Context, oldBuild, build *choreov1.BuildV2, workflow *argoproj.Workflow) (ctrl.Result, error) {
	// Check workflow status
	switch workflow.Status.Phase {
	case argoproj.WorkflowSucceeded:
		setBuildCompletedCondition(build, "Build completed successfully")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		setBuildFailedCondition(build, ReasonBuildFailed, "Build workflow failed")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	case argoproj.WorkflowRunning:
		setBuildInProgressCondition(build)
		// Requeue after 20 seconds to check workflow status
		return r.updateStatusAndRequeueAfter(ctx, oldBuild, build, 20*time.Second)
	default:
		// Workflow is pending or in unknown state, requeue
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
}

// Status update methods
func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, oldBuild, build *choreov1.BuildV2) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndReturn(ctx context.Context, oldBuild, build *choreov1.BuildV2) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndRequeueAfter(ctx context.Context, oldBuild, build *choreov1.BuildV2, duration time.Duration) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, oldBuild, build, duration)
}
