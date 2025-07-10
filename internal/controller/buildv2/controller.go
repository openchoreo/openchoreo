// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildv2

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
	build := &openchoreov1alpha1.BuildV2{}
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

	if !isBuildSucceeded(build) {
		// Update build status based on workflow status
		return r.updateBuildStatus(ctx, oldBuild, build, workflow)
	}

	err = r.updateWorkloadWithBuiltImage(ctx, build)
	if err != nil {
		logger.Error(err, "Failed to patch workload with image")
		meta.SetStatusCondition(&build.Status.Conditions, NewWorkloadUpdateFailedCondition(build.Generation))
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
	meta.SetStatusCondition(&build.Status.Conditions, NewWorkloadUpdatedCondition(build.Generation))
	return r.updateStatusAndReturn(ctx, oldBuild, build)
}

const (
	workloadProjectIndexKey   = "spec.owner.projectName"
	workloadComponentIndexKey = "spec.owner.componentName"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.k8sClientMgr == nil {
		r.k8sClientMgr = kubernetesClient.NewManager()
	}

	ctx := context.Background()

	// Field index: spec.owner.projectName
	if err := mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Workload{}, workloadProjectIndexKey,
		func(obj client.Object) []string {
			if wl, ok := obj.(*openchoreov1alpha1.Workload); ok {
				return []string{wl.Spec.Owner.ProjectName}
			}
			return nil
		}); err != nil {
		return fmt.Errorf("index owner.projectName: %w", err)
	}

	// Field index: spec.owner.componentName
	if err := mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Workload{}, workloadComponentIndexKey,
		func(obj client.Object) []string {
			if wl, ok := obj.(*openchoreov1alpha1.Workload); ok {
				return []string{wl.Spec.Owner.ComponentName}
			}
			return nil
		}); err != nil {
		return fmt.Errorf("index owner.componentName: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.BuildV2{}).
		Named("buildv2").
		Complete(r)
}

func (r *Reconciler) updateWorkloadWithBuiltImage(
	ctx context.Context,
	build *openchoreov1alpha1.BuildV2,
) error {
	wlList := &openchoreov1alpha1.WorkloadList{}
	if err := r.List(
		ctx,
		wlList,
		client.InNamespace(build.Namespace),
		client.MatchingFields{
			workloadProjectIndexKey:   build.Spec.Owner.ProjectName,
			workloadComponentIndexKey: build.Spec.Owner.ComponentName,
		},
	); err != nil {
		return fmt.Errorf("list workloads: %w", err)
	}

	if len(wlList.Items) == 0 {
		return fmt.Errorf("no Workload found for project=%s component=%s",
			build.Spec.Owner.ProjectName, build.Spec.Owner.ComponentName)
	}
	workload := &wlList.Items[0]

	oldWorkload := workload.DeepCopy()

	for name, c := range workload.Spec.Containers {
		c.Image = build.Status.ImageStatus.Image
		workload.Spec.Containers[name] = c
		break
	}

	return r.Patch(ctx, workload, client.MergeFrom(oldWorkload))
}

func (r *Reconciler) getBPClient(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) (client.Client, error) {
	bpClient, err := kubernetesClient.GetK8sClient(r.k8sClientMgr, buildPlane.Spec.Owner.OrganizationName, buildPlane.Name, buildPlane.Spec.KubernetesCluster)
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Failed to get build plane client")
		return nil, err
	}
	return bpClient, nil
}

// ensurePrerequisiteResources ensures that all prerequisite resources exist for the workflow
func (r *Reconciler) ensurePrerequisiteResources(ctx context.Context, bpClient client.Client, build *openchoreov1alpha1.BuildV2, logger logr.Logger) error {
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
	build *openchoreov1alpha1.BuildV2,
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
func (r *Reconciler) updateBuildStatus(ctx context.Context, oldBuild, build *openchoreov1alpha1.BuildV2, workflow *argoproj.Workflow) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("build", build.Name)
	switch workflow.Status.Phase {
	case argoproj.WorkflowSucceeded:
		setBuildCompletedCondition(build, "Build completed successfully")
		stepInfo := getStepByTemplateName(workflow.Status.Nodes, "push-step")
		if stepInfo == nil {
			logger.Error(fmt.Errorf("push-step not found in workflow nodes"), "Push step not found")
			return r.updateStatusAndRequeue(ctx, oldBuild, build)
		}
		image := getImageNameFromWorkflow(*stepInfo.Outputs)
		if image == "" {
			logger.Error(fmt.Errorf("image not found in workflow outputs"), "Image not found in workflow outputs")
			return r.updateStatusAndRequeue(ctx, oldBuild, build)
		}
		build.Status.ImageStatus.Image = string(image)
		if err := r.Status().Update(ctx, build); err != nil {
			logger.Error(err, "Failed to update build status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
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

func getStepByTemplateName(nodes argoproj.Nodes, step string) *argoproj.NodeStatus {
	for _, node := range nodes {
		if node.TemplateName == step {
			return &node
		}
	}
	return nil
}

func getImageNameFromWorkflow(output argoproj.Outputs) argoproj.AnyString {
	for _, param := range output.Parameters {
		if param.Name == "image" {
			return *param.Value
		}
	}
	return ""
}

// Status update methods
func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, oldBuild, build *openchoreov1alpha1.BuildV2) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndReturn(ctx context.Context, oldBuild, build *openchoreov1alpha1.BuildV2) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndRequeueAfter(ctx context.Context, oldBuild, build *openchoreov1alpha1.BuildV2, duration time.Duration) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, oldBuild, build, duration)
}
