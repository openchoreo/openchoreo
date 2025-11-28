// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

type Reconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	k8sClientMgr *kubernetesClient.KubeMultiClientManager

	// Pipeline is the workflow rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different workflow runs and reconciliations.
	Pipeline *workflowpipeline.Pipeline
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflows,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("workflowrun", req.NamespacedName)

	workflowRun := &openchoreodevv1alpha1.WorkflowRun{}
	if err := r.Get(ctx, req.NamespacedName, workflowRun); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	oldWorkflowRun := workflowRun.DeepCopy()

	if isWorkloadUpdated(workflowRun) {
		return ctrl.Result{}, nil
	}

	if isWorkflowCompleted(workflowRun) {
		if isWorkflowSucceeded(workflowRun) {
			return r.handleWorkloadCreation(ctx, oldWorkflowRun, workflowRun)
		}
		return ctrl.Result{}, nil
	}

	if !isWorkflowInitiated(workflowRun) {
		setWorkflowPendingCondition(workflowRun)
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, workflowRun)
	if err != nil {
		logger.Error(err, "failed to get build plane")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	if workflowRun.Status.RunReference.Name != "" && workflowRun.Status.RunReference.Namespace != "" {
		pipeline := &argoproj.Workflow{}
		err = bpClient.Get(ctx, types.NamespacedName{
			Name:      workflowRun.Status.RunReference.Name,
			Namespace: workflowRun.Status.RunReference.Namespace,
		}, pipeline)

		if err == nil {
			return r.syncWorkflowRunStatus(ctx, oldWorkflowRun, workflowRun, pipeline)
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "failed to get build plane pipeline")
			return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
		}
		setWorkflowNotFoundCondition(workflowRun)
		return r.updateStatusAndReturn(ctx, oldWorkflowRun, workflowRun)
	}

	workflow := &openchoreodevv1alpha1.Workflow{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflowRun.Spec.Workflow.Name,
		Namespace: workflowRun.Namespace,
	}, workflow); err != nil {
		logger.Error(err, "failed to get Workflow")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	renderInput := &workflowpipeline.RenderInput{
		WorkflowRun: workflowRun,
		Workflow:    workflow,
		Context: workflowpipeline.WorkflowContext{
			OrgName:         workflowRun.Namespace,
			ProjectName:     workflowRun.Spec.Owner.ProjectName,
			ComponentName:   workflowRun.Spec.Owner.ComponentName,
			WorkflowRunName: workflowRun.Name,
		},
	}

	output, err := r.Pipeline.Render(renderInput)
	if err != nil {
		logger.Error(err, "failed to render workflow")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	pipelineNamespace, err := extractNamespace(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract namespace from rendered resource")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	return r.ensurePipelineResource(ctx, oldWorkflowRun, workflowRun, output, pipelineNamespace, bpClient)
}

func (r *Reconciler) handleWorkloadCreation(
	ctx context.Context,
	oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, workflowRun)
	if err != nil {
		logger.Error(err, "failed to get build plane for workload creation")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client for workload creation")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	shouldRequeue, err := r.createWorkloadFromWorkflowRun(ctx, workflowRun, bpClient)
	if err != nil {
		logger.Error(err, "failed to create workload CR")
		if shouldRequeue {
			return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
		}
		setWorkloadUpdateFailedCondition(workflowRun)
		return r.updateStatusAndReturn(ctx, oldWorkflowRun, workflowRun)
	}

	setWorkloadUpdatedCondition(workflowRun)
	return r.updateStatusAndReturn(ctx, oldWorkflowRun, workflowRun)
}

func (r *Reconciler) ensurePipelineResource(
	ctx context.Context,
	oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun,
	output *workflowpipeline.RenderOutput,
	pipelineNamespace string,
	bpClient client.Client,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	serviceAccountName, err := extractServiceAccountName(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract service account name from rendered resource")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	// Ensure prerequisite resources (namespace, RBAC) are created in the build plane
	if err := r.ensurePrerequisites(ctx, pipelineNamespace, serviceAccountName, bpClient); err != nil {
		logger.Error(err, "failed to ensure prerequisite resources")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	if err := r.applyRenderedPipeline(ctx, workflowRun, output.Resource, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered pipeline resource")
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}

	return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
}

func (r *Reconciler) syncWorkflowRunStatus(
	ctx context.Context,
	oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun,
	pipeline *argoproj.Workflow,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	switch pipeline.Status.Phase {
	case argoproj.WorkflowRunning:
		setWorkflowRunningCondition(workflowRun)
		return r.updateStatusAndRequeueAfter(ctx, oldWorkflowRun, workflowRun, 20*time.Second)
	case argoproj.WorkflowSucceeded:
		setWorkflowSucceededCondition(workflowRun)
		if pushStep := getStepByTemplateName(pipeline.Status.Nodes, engines.StepPush); pushStep != nil {
			if image := getImageNameFromPipeline(*pushStep.Outputs); image != "" {
				workflowRun.Status.ImageStatus.Image = string(image)
			}
		}
		if err := r.Status().Update(ctx, workflowRun); err != nil {
			logger.Error(err, "Failed to update workflowrun status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		setWorkflowFailedCondition(workflowRun)
		return r.updateStatusAndReturn(ctx, oldWorkflowRun, workflowRun)
	default:
		return r.updateStatusAndRequeue(ctx, oldWorkflowRun, workflowRun)
	}
}

func (r *Reconciler) applyRenderedPipeline(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	resource map[string]any,
	bpClient client.Client,
) error {
	logger := log.FromContext(ctx)

	resource = convertParameterValuesToStrings(resource)

	unstructuredResource := &unstructured.Unstructured{Object: resource}

	resourceNamespace := unstructuredResource.GetNamespace()
	if resourceNamespace == workflowRun.Namespace || resourceNamespace == "" {
		if err := ctrl.SetControllerReference(workflowRun, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	} else {
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/workflowrun"] = workflowRun.Name
		labels["openchoreo.dev/workflowrun-namespace"] = workflowRun.Namespace
		unstructuredResource.SetLabels(labels)
	}

	existingResource := &unstructured.Unstructured{}
	existingResource.SetGroupVersionKind(unstructuredResource.GroupVersionKind())

	namespace := unstructuredResource.GetNamespace()
	name := unstructuredResource.GetName()

	err := bpClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, existingResource)

	if err != nil {
		if errors.IsNotFound(err) {
			if err := bpClient.Create(ctx, unstructuredResource); err != nil {
				return err
			}
			workflowRun.Status.RunReference.Name = name
			workflowRun.Status.RunReference.Namespace = namespace
			if err := r.Status().Update(ctx, workflowRun); err != nil {
				logger.Error(err, "Failed to update workflowrun status")
				return err
			}
			return nil
		}
		return fmt.Errorf("failed to get existing resource: %w", err)
	}

	unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
	if err := bpClient.Update(ctx, unstructuredResource); err != nil {
		return err
	}

	workflowRun.Status.RunReference.Name = name
	workflowRun.Status.RunReference.Namespace = namespace
	return nil
}

func (r *Reconciler) createWorkloadFromWorkflowRun(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	bpClient client.Client,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("workflowrun", workflowRun.Name)

	// Use the stored RunReference to retrieve the build plane pipeline
	if workflowRun.Status.RunReference.Name == "" || workflowRun.Status.RunReference.Namespace == "" {
		logger.Error(nil, "build plane pipeline reference not found in status")
		return true, fmt.Errorf("pipeline reference not set in status")
	}

	pipeline := &argoproj.Workflow{}
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      workflowRun.Status.RunReference.Name,
		Namespace: workflowRun.Status.RunReference.Namespace,
	}, pipeline); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("build plane pipeline not found, skipping workload creation")
			return false, fmt.Errorf("build plane pipeline not found: %w", err)
		}
		return true, fmt.Errorf("failed to get build plane pipeline: %w", err)
	}

	workloadCR := extractWorkloadCRFromPipeline(pipeline)
	if workloadCR == "" {
		logger.Info("no workload CR found in build plane pipeline outputs")
		return false, fmt.Errorf("no workload CR found in pipeline outputs")
	}

	workload := &openchoreodevv1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadCR), workload); err != nil {
		return true, fmt.Errorf("failed to unmarshal workload CR: %w", err)
	}

	// Set the namespace to match the workflowrun
	workload.Namespace = workflowRun.Namespace

	if err := r.Patch(ctx, workload, client.Apply, client.FieldOwner("workflowrun-controller"), client.ForceOwnership); err != nil {
		return true, fmt.Errorf("failed to apply workload CR: %w", err)
	}

	return false, nil
}

func (r *Reconciler) getBuildPlaneClient(buildPlane *openchoreodevv1alpha1.BuildPlane) (client.Client, error) {
	bpClient, err := kubernetesClient.GetK8sClient(r.k8sClientMgr, buildPlane.Namespace, buildPlane.Name, buildPlane.Spec.KubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}
	return bpClient, nil
}

func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldWorkflowRun, workflowRun)
}

func (r *Reconciler) updateStatusAndReturn(ctx context.Context, oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldWorkflowRun, workflowRun)
}

func (r *Reconciler) updateStatusAndRequeueAfter(ctx context.Context, oldWorkflowRun, workflowRun *openchoreodevv1alpha1.WorkflowRun, duration time.Duration) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, oldWorkflowRun, workflowRun, duration)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.k8sClientMgr == nil {
		r.k8sClientMgr = kubernetesClient.NewManager()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.WorkflowRun{}).
		Named("workflowrun").
		Complete(r)
}

// extractWorkloadCRFromPipeline extracts workload CR from build plane pipeline outputs
func extractWorkloadCRFromPipeline(pipeline *argoproj.Workflow) string {
	for _, node := range pipeline.Status.Nodes {
		if node.TemplateName == "workload-create-step" && node.Phase == argoproj.NodeSucceeded {
			if node.Outputs != nil {
				for _, param := range node.Outputs.Parameters {
					if param.Name == "workload-cr" && param.Value != nil {
						return string(*param.Value)
					}
				}
			}
		}
	}
	return ""
}

func convertParameterValuesToStrings(resource map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range resource {
		if key == "spec" {
			if spec, ok := value.(map[string]any); ok {
				result[key] = convertSpecParametersToStrings(spec)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

func convertSpecParametersToStrings(spec map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range spec {
		if key == "arguments" {
			if args, ok := value.(map[string]any); ok {
				result[key] = convertArgumentsParametersToStrings(args)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

func convertArgumentsParametersToStrings(args map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range args {
		if key == "parameters" {
			if params, ok := value.([]any); ok {
				result[key] = convertParameterListToStrings(params)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

func convertParameterListToStrings(params []any) []any {
	result := make([]any, len(params))

	for i, param := range params {
		if paramMap, ok := param.(map[string]any); ok {
			convertedParam := make(map[string]any)
			for k, v := range paramMap {
				if k == "value" {
					convertedParam[k] = convertToString(v)
				} else {
					convertedParam[k] = v
				}
			}
			result[i] = convertedParam
		} else {
			result[i] = param
		}
	}

	return result
}

func convertToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []any, map[string]any:
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// getStepByTemplateName finds a node in the workflow by its template name
func getStepByTemplateName(nodes argoproj.Nodes, step string) *argoproj.NodeStatus {
	for _, node := range nodes {
		if node.TemplateName == step {
			return &node
		}
	}
	return nil
}

// getImageNameFromPipeline extracts the image name from build plane pipeline outputs
func getImageNameFromPipeline(output argoproj.Outputs) argoproj.AnyString {
	for _, param := range output.Parameters {
		if param.Name == "image" && param.Value != nil {
			return *param.Value
		}
	}
	return ""
}

// extractServiceAccountName extracts the service account name from the rendered workflow resource
func extractServiceAccountName(resource map[string]any) (string, error) {
	spec, ok := resource["spec"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("spec not found in rendered resource")
	}

	serviceAccountName, ok := spec["serviceAccountName"].(string)
	if !ok || serviceAccountName == "" {
		return "", fmt.Errorf("serviceAccountName not found in rendered resource spec")
	}

	return serviceAccountName, nil
}

// extractNamespace extracts the namespace from rendered resource metadata
func extractNamespace(resource map[string]any) (string, error) {
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("metadata not found in rendered resource")
	}

	namespace, ok := metadata["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("namespace not found in rendered resource metadata")
	}

	return namespace, nil
}
