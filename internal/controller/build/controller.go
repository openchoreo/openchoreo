// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"
)

const (
	// ControllerName is the name of the controller managing Build resources
	ControllerName = "build-controller"
)

// Reconciler reconciles a Build object
type Reconciler struct {
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
	IsGitOpsMode bool
	K8sClientMgr *kubernetesClient.KubeMultiClientManager
	Scheme       *runtime.Scheme
	GatewayURL   string
	engine       *Builder
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=builds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=builds/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("build", req.NamespacedName)

	// Fetch the build resource
	build := &openchoreov1alpha1.Build{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Build resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Build")
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
	if err := r.engine.EnsurePrerequisites(ctx, build, bpClient); err != nil {
		logger.Error(err, "Error ensuring prerequisite resources")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	}

	buildResponse, err := r.engine.CreateBuild(ctx, build, bpClient)
	if err != nil {
		logger.Error(err, "cannot ensure workflow")
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
	if buildResponse.Created {
		setBuildTriggeredCondition(build)
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}

	if !isBuildWorkflowSucceeded(build) {
		// Update build status based on workflow status
		return r.updateBuildStatus(ctx, oldBuild, build, bpClient)
	}

	err = r.applyWorkloadCR(ctx, build, bpClient)
	if err != nil {
		logger.Error(err, "Failed to create workload CR")
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
	if r.engine == nil {
		r.engine = NewBuilder(r.Client, r.K8sClientMgr, r.GatewayURL)

		// Register build engines here to avoid circular imports
		r.engine.registerBuildEngines()
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
		For(&openchoreov1alpha1.Build{}).
		Named("build").
		Complete(r)
}

func (r *Reconciler) applyWorkloadCR(ctx context.Context, build *openchoreov1alpha1.Build, bpClient client.Client) error {
	logger := log.FromContext(ctx).WithValues("build", build.Name)

	buildArtifacts, err := r.engine.ExtractBuildArtifacts(ctx, build, bpClient)
	if err != nil {
		logger.Error(err, "Failed to extract build artifacts")
		return fmt.Errorf("failed to extract build artifacts: %w", err)
	}

	if buildArtifacts.WorkloadCR == "" {
		logger.Info("No workload CR found in build artifacts, waiting workload creation step to be completed")
		return nil
	}

	// Parse the YAML into a Workload object
	workload := &openchoreov1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(buildArtifacts.WorkloadCR), workload); err != nil {
		logger.Error(err, "Failed to unmarshal workload CR YAML")
		return fmt.Errorf("failed to unmarshal workload CR: %w", err)
	}

	// Set the namespace to match the build
	workload.Namespace = build.Namespace

	// Use server-side apply to create or update the workload
	if err := r.Patch(ctx, workload, client.Apply, client.FieldOwner(ControllerName), client.ForceOwnership); err != nil {
		logger.Error(err, "Failed to apply workload CR", "name", workload.Name, "namespace", workload.Namespace)
		return fmt.Errorf("failed to apply workload CR: %w", err)
	}

	logger.Info("Successfully applied workload CR", "name", workload.Name, "namespace", workload.Namespace)
	return nil
}

//nolint:unused // Temporarily disabled
func (r *Reconciler) updateWorkloadWithBuiltImage(
	ctx context.Context,
	build *openchoreov1alpha1.Build,
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

	workload.Spec.Container.Image = build.Status.ImageStatus.Image

	return r.Patch(ctx, workload, client.MergeFrom(oldWorkload))
}

func (r *Reconciler) getBPClient(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) (client.Client, error) {
	bpClient, err := r.engine.GetBuildPlaneClient(ctx, buildPlane)
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Failed to get build plane client")
		return nil, err
	}
	return bpClient, nil
}

// updateBuildStatus updates build status based on workflow status
func (r *Reconciler) updateBuildStatus(ctx context.Context, oldBuild, build *openchoreov1alpha1.Build, bpClient client.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("build", build.Name)
	buildStatus, err := r.engine.GetBuildStatus(ctx, build, bpClient)
	if err != nil {
		logger.Error(err, "Failed to get build status")
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
	switch buildStatus.Phase {
	case engines.BuildPhaseRunning:
		setBuildInProgressCondition(build)
		// Requeue after 20 seconds to check workflow status
		return r.updateStatusAndRequeueAfter(ctx, oldBuild, build, 20*time.Second)
	case engines.BuildPhaseSucceeded:
		setBuildCompletedCondition(build, "Build completed successfully")
		buildArtifacts, err := r.engine.ExtractBuildArtifacts(ctx, build, bpClient)
		if err != nil {
			logger.Error(err, "Failed to extract build artifacts")
			return r.updateStatusAndRequeue(ctx, oldBuild, build)
		}
		build.Status.ImageStatus.Image = buildArtifacts.Image
		if err := r.Status().Update(ctx, build); err != nil {
			logger.Error(err, "Failed to update build status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	case engines.BuildPhaseFailed:
		setBuildFailedCondition(build, ReasonBuildFailed, "Build workflow failed")
		return r.updateStatusAndReturn(ctx, oldBuild, build)
	default:
		// Workflow is pending or in unknown state, requeue
		return r.updateStatusAndRequeue(ctx, oldBuild, build)
	}
}

// Status update methods
func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, oldBuild, build *openchoreov1alpha1.Build) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndReturn(ctx context.Context, oldBuild, build *openchoreov1alpha1.Build) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
}

func (r *Reconciler) updateStatusAndRequeueAfter(ctx context.Context, oldBuild, build *openchoreov1alpha1.Build, duration time.Duration) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, oldBuild, build, duration)
}
