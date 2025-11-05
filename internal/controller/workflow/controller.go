// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// Test hook: if set, this function provides the client to use for build-plane calls.
	// In production this should be nil.
	BuildPlaneClientProvider func(*openchoreodevv1alpha1.BuildPlane) (client.Client, error)
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("workflow", req.NamespacedName)

	workflow := &openchoreodevv1alpha1.Workflow{}
	if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	oldWorkflow := workflow.DeepCopy()

	if isWorkloadUpdated(workflow) {
		return ctrl.Result{}, nil
	}

	if isWorkflowCompleted(workflow) {
		if isWorkflowFailed(workflow) {
			return ctrl.Result{}, nil
		}
		return r.handleWorkloadCreation(ctx, oldWorkflow, workflow)
	}

	if !isWorkflowInitiated(workflow) {
		setWorkflowPendingCondition(workflow)
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, workflow)
	if err != nil {
		logger.Error(err, "failed to get build plane")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	workflowDef := &openchoreodevv1alpha1.WorkflowDefinition{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflow.Spec.WorkflowDefinitionRef,
		Namespace: workflow.Namespace,
	}, workflowDef); err != nil {
		logger.Error(err, "failed to get WorkflowDefinition")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	workflowName, workflowNamespace, err := r.getWorkflowMetadata(workflow, workflowDef)
	if err != nil {
		logger.Error(err, "failed to get workflow run name and namespace")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	workflowRun := &argoproj.Workflow{}
	err = bpClient.Get(ctx, types.NamespacedName{
		Name:      workflowName,
		Namespace: workflowNamespace,
	}, workflowRun)

	if err == nil {
		return r.syncWorkflowRunStatus(ctx, oldWorkflow, workflow, workflowRun)
	}

	if !errors.IsNotFound(err) {
		logger.Error(err, "failed to check workflow run existence")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	return r.ensureWorkflowResource(ctx, oldWorkflow, workflow, workflowDef, bpClient)
}

func (r *Reconciler) handleWorkloadCreation(
	ctx context.Context,
	oldWorkflow, workflow *openchoreodevv1alpha1.Workflow,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, workflow)
	if err != nil {
		logger.Error(err, "failed to get build plane for workload creation")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client for workload creation")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	shouldRequeue, err := r.createWorkloadFromWorkflowRun(ctx, workflow, bpClient)
	if err != nil {
		logger.Error(err, "failed to create workload CR")
		if shouldRequeue {
			return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
		}
		setWorkloadUpdateFailedCondition(workflow)
		return r.updateStatusAndReturn(ctx, oldWorkflow, workflow)
	}

	setWorkloadUpdatedCondition(workflow)
	return r.updateStatusAndReturn(ctx, oldWorkflow, workflow)
}

func (r *Reconciler) ensureWorkflowResource(
	ctx context.Context,
	oldWorkflow, workflow *openchoreodevv1alpha1.Workflow,
	workflowDef *openchoreodevv1alpha1.WorkflowDefinition,
	bpClient client.Client,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	component := &openchoreodevv1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflow.Spec.Owner.ComponentName,
		Namespace: workflow.Namespace,
	}, component); err != nil {
		logger.Error(err, "failed to get Component")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	componentTypeDefName, err := parseComponentTypeDefinitionName(component.Spec.ComponentType)
	if err != nil {
		logger.Error(err, "failed to parse ComponentTypeDefinition name")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	componentTypeDef := &openchoreodevv1alpha1.ComponentTypeDefinition{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      componentTypeDefName,
		Namespace: workflow.Namespace,
	}, componentTypeDef); err != nil {
		logger.Error(err, "failed to get ComponentTypeDefinition")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	pipeline := workflowpipeline.NewPipeline()
	renderInput := &workflowpipeline.RenderInput{
		Workflow:                workflow,
		WorkflowDefinition:      workflowDef,
		ComponentTypeDefinition: componentTypeDef,
		Context: workflowpipeline.WorkflowContext{
			OrgName:       workflow.Namespace,
			ProjectName:   workflow.Spec.Owner.ProjectName,
			ComponentName: workflow.Spec.Owner.ComponentName,
			WorkflowName:  workflow.Name,
		},
	}

	output, err := pipeline.Render(renderInput)
	if err != nil {
		logger.Error(err, "failed to render workflow run")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	if err := r.applyRenderedResource(ctx, workflow, output.Resource, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered resource")
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}

	return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
}

func (r *Reconciler) syncWorkflowRunStatus(
	ctx context.Context,
	oldWorkflow, workflow *openchoreodevv1alpha1.Workflow,
	workflowRun *argoproj.Workflow,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if workflow.Status.StartTime == nil && !workflowRun.Status.StartedAt.IsZero() {
		workflow.Status.StartTime = &metav1.Time{Time: workflowRun.Status.StartedAt.Time}
	}

	switch workflowRun.Status.Phase {
	case argoproj.WorkflowRunning:
		setWorkflowRunningCondition(workflow)
		return r.updateStatusAndRequeueAfter(ctx, oldWorkflow, workflow, 20*time.Second)
	case argoproj.WorkflowSucceeded:
		setWorkflowSucceededCondition(workflow)
		if !workflowRun.Status.FinishedAt.IsZero() {
			workflow.Status.CompletionTime = &metav1.Time{Time: workflowRun.Status.FinishedAt.Time}
		}
		// Extract image from push-step
		if pushStep := getStepByTemplateName(workflowRun.Status.Nodes, engines.StepPush); pushStep != nil {
			if image := getImageNameFromWorkflow(*pushStep.Outputs); image != "" {
				workflow.Status.ImageStatus.Image = string(image)
			}
		}
		if err := r.Status().Update(ctx, workflow); err != nil {
			logger.Error(err, "Failed to update workflow status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		setWorkflowFailedCondition(workflow)
		if !workflowRun.Status.FinishedAt.IsZero() {
			workflow.Status.CompletionTime = &metav1.Time{Time: workflowRun.Status.FinishedAt.Time}
		}
		return r.updateStatusAndReturn(ctx, oldWorkflow, workflow)
	default:
		return r.updateStatusAndRequeue(ctx, oldWorkflow, workflow)
	}
}

func (r *Reconciler) applyRenderedResource(
	ctx context.Context,
	workflow *openchoreodevv1alpha1.Workflow,
	resource map[string]any,
	bpClient client.Client,
) error {
	logger := log.FromContext(ctx)

	resource = convertParameterValuesToStrings(resource)

	unstructuredResource := &unstructured.Unstructured{Object: resource}

	resourceNamespace := unstructuredResource.GetNamespace()
	if resourceNamespace == workflow.Namespace || resourceNamespace == "" {
		if err := ctrl.SetControllerReference(workflow, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	} else {
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/workflow"] = workflow.Name
		labels["openchoreo.dev/workflow-namespace"] = workflow.Namespace
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
			logger.Info("creating rendered resource on build plane",
				"kind", unstructuredResource.GetKind(),
				"name", name,
				"namespace", namespace)
			return bpClient.Create(ctx, unstructuredResource)
		}
		return fmt.Errorf("failed to get existing resource: %w", err)
	}

	logger.Info("updating rendered resource on build plane",
		"kind", unstructuredResource.GetKind(),
		"name", name,
		"namespace", namespace)

	unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
	return bpClient.Update(ctx, unstructuredResource)
}

func (r *Reconciler) createWorkloadFromWorkflowRun(
	ctx context.Context,
	workflow *openchoreodevv1alpha1.Workflow,
	bpClient client.Client,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("workflow", workflow.Name)

	workflowDef := &openchoreodevv1alpha1.WorkflowDefinition{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflow.Spec.WorkflowDefinitionRef,
		Namespace: workflow.Namespace,
	}, workflowDef); err != nil {
		return true, fmt.Errorf("failed to get WorkflowDefinition: %w", err)
	}

	workflowRunName, workflowRunNamespace, err := r.getWorkflowMetadata(workflow, workflowDef)
	if err != nil {
		return true, fmt.Errorf("failed to get workflow run name and namespace: %w", err)
	}

	argoWorkflow := &argoproj.Workflow{}
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      workflowRunName,
		Namespace: workflowRunNamespace,
	}, argoWorkflow); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("argo workflow not found, skipping workload creation")
			return false, fmt.Errorf("argo workflow not found: %w", err)
		}
		return true, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	workloadCR := extractWorkloadCRFromArgoWorkflow(argoWorkflow)
	if workloadCR == "" {
		logger.Info("no workload CR found in argo workflow outputs")
		return false, fmt.Errorf("no workload CR found in argo workflow outputs: %w", err)
	}

	workload := &openchoreodevv1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadCR), workload); err != nil {
		return true, fmt.Errorf("failed to unmarshal workload CR: %w", err)
	}

	// Set the namespace to match the workflow
	workload.Namespace = workflow.Namespace

	if err := r.Patch(ctx, workload, client.Apply, client.FieldOwner("workflow-controller"), client.ForceOwnership); err != nil {
		return true, fmt.Errorf("failed to apply workload CR: %w", err)
	}

	return false, nil
}

func (r *Reconciler) getBuildPlaneClient(buildPlane *openchoreodevv1alpha1.BuildPlane) (client.Client, error) {
	if r.BuildPlaneClientProvider != nil {
		return r.BuildPlaneClientProvider(buildPlane)
	}
	bpClient, err := kubernetesClient.GetK8sClient(r.k8sClientMgr, buildPlane.Namespace, buildPlane.Name, buildPlane.Spec.KubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}
	return bpClient, nil
}

func (r *Reconciler) getWorkflowMetadata(
	workflow *openchoreodevv1alpha1.Workflow,
	workflowDef *openchoreodevv1alpha1.WorkflowDefinition,
) (string, string, error) {
	minimalPipeline := workflowpipeline.NewPipeline()
	minimalInput := &workflowpipeline.RenderInput{
		Workflow:           workflow,
		WorkflowDefinition: workflowDef,
		Context: workflowpipeline.WorkflowContext{
			OrgName:       workflow.Namespace,
			ProjectName:   workflow.Spec.Owner.ProjectName,
			ComponentName: workflow.Spec.Owner.ComponentName,
			WorkflowName:  workflow.Name,
		},
	}

	minimalOutput, err := minimalPipeline.Render(minimalInput)
	if err != nil {
		return "", "", fmt.Errorf("failed to render workflow for name extraction: %w", err)
	}

	resourceMetadata, ok := minimalOutput.Resource["metadata"].(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("failed to extract metadata from rendered resource")
	}

	workflowRunName, _ := resourceMetadata["name"].(string)
	workflowRunNamespace, _ := resourceMetadata["namespace"].(string)

	if workflowRunName == "" || workflowRunNamespace == "" {
		return "", "", fmt.Errorf("failed to extract name or namespace from rendered resource")
	}

	return workflowRunName, workflowRunNamespace, nil
}

func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, oldWorkflow, workflow *openchoreodevv1alpha1.Workflow) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldWorkflow, workflow)
}

func (r *Reconciler) updateStatusAndReturn(ctx context.Context, oldWorkflow, workflow *openchoreodevv1alpha1.Workflow) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldWorkflow, workflow)
}

func (r *Reconciler) updateStatusAndRequeueAfter(ctx context.Context, oldWorkflow, workflow *openchoreodevv1alpha1.Workflow, duration time.Duration) (ctrl.Result, error) {
	return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, oldWorkflow, workflow, duration)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.k8sClientMgr == nil {
		r.k8sClientMgr = kubernetesClient.NewManager()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.Workflow{}).
		Named("workflow").
		Complete(r)
}

func parseComponentTypeDefinitionName(componentType string) (string, error) {
	parts := strings.Split(componentType, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid componentType format: expected 'workloadType/name', got '%s'", componentType)
	}
	return parts[1], nil
}

func extractWorkloadCRFromArgoWorkflow(argoWorkflow *argoproj.Workflow) string {
	for _, node := range argoWorkflow.Status.Nodes {
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

// getImageNameFromWorkflow extracts the image name from workflow outputs
func getImageNameFromWorkflow(output argoproj.Outputs) argoproj.AnyString {
	for _, param := range output.Parameters {
		if param.Name == "image" && param.Value != nil {
			return *param.Value
		}
	}
	return ""
}
