// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	componentworkflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/componentworkflow"
)

// ComponentWorkflowRunReconciler reconciles a ComponentWorkflowRun object
type ComponentWorkflowRunReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	K8sClientMgr *kubernetesClient.KubeMultiClientManager

	// Pipeline is the component workflow rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different workflow runs and reconciliations.
	Pipeline   *componentworkflowpipeline.Pipeline
	GatewayURL string
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflowruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflowruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflowruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflows,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ComponentWorkflowRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx).WithValues("componentworkflowrun", req.NamespacedName)

	componentWorkflowRun := &openchoreodevv1alpha1.ComponentWorkflowRun{}
	if err := r.Get(ctx, req.NamespacedName, componentWorkflowRun); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	// Keep a copy for comparison
	old := componentWorkflowRun.DeepCopy()

	// Deferred status update
	defer func() {
		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, componentWorkflowRun.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, componentWorkflowRun); err != nil {
			logger.Error(err, "Failed to update ComponentWorkflowRun status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	if isWorkloadUpdated(componentWorkflowRun) {
		return ctrl.Result{}, nil
	}

	if !isWorkflowInitiated(componentWorkflowRun) {
		setWorkflowPendingCondition(componentWorkflowRun)
		return ctrl.Result{Requeue: true}, nil
	}

	buildPlane, err := controller.GetBuildPlane(ctx, r.Client, componentWorkflowRun)
	if err != nil {
		logger.Error(err, "failed to get build plane")
		return ctrl.Result{Requeue: true}, nil
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client")
		return ctrl.Result{Requeue: true}, nil
	}

	if isWorkflowCompleted(componentWorkflowRun) {
		if isWorkflowSucceeded(componentWorkflowRun) {
			return r.handleWorkloadCreation(ctx, componentWorkflowRun, bpClient), nil
		}
		return ctrl.Result{}, nil
	}

	if componentWorkflowRun.Status.RunReference.Name != "" && componentWorkflowRun.Status.RunReference.Namespace != "" {
		runResource := &argoproj.Workflow{}
		err = bpClient.Get(ctx, types.NamespacedName{
			Name:      componentWorkflowRun.Status.RunReference.Name,
			Namespace: componentWorkflowRun.Status.RunReference.Namespace,
		}, runResource)

		if err == nil {
			return r.syncWorkflowRunStatus(componentWorkflowRun, runResource), nil
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "failed to get run resource")
			return ctrl.Result{Requeue: true}, nil
		}
		setWorkflowNotFoundCondition(componentWorkflowRun)
		return ctrl.Result{}, nil
	}

	componentWorkflow := &openchoreodevv1alpha1.ComponentWorkflow{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      componentWorkflowRun.Spec.Workflow.Name,
		Namespace: componentWorkflowRun.Namespace,
	}, componentWorkflow); err != nil {
		logger.Error(err, "failed to get ComponentWorkflow")
		return ctrl.Result{Requeue: true}, nil
	}

	renderInput := &componentworkflowpipeline.RenderInput{
		ComponentWorkflowRun: componentWorkflowRun,
		ComponentWorkflow:    componentWorkflow,
		Context: componentworkflowpipeline.ComponentWorkflowContext{
			OrgName:         componentWorkflowRun.Namespace,
			ProjectName:     componentWorkflowRun.Spec.Owner.ProjectName,
			ComponentName:   componentWorkflowRun.Spec.Owner.ComponentName,
			WorkflowRunName: componentWorkflowRun.Name,
		},
	}

	output, err := r.Pipeline.Render(renderInput)
	if err != nil {
		logger.Error(err, "failed to render component workflow")
		return ctrl.Result{Requeue: true}, nil
	}

	runResNamespace, err := extractRunResourceNamespace(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract namespace from rendered resource")
		return ctrl.Result{Requeue: true}, nil
	}

	return r.ensureRunResource(ctx, componentWorkflowRun, output, runResNamespace, bpClient), nil
}

func (r *ComponentWorkflowRunReconciler) handleWorkloadCreation(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	bpClient client.Client) ctrl.Result {
	logger := log.FromContext(ctx)

	shouldRequeue, err := r.createWorkloadFromComponentWorkflowRun(ctx, componentWorkflowRun, bpClient)
	if err != nil {
		logger.Error(err, "failed to create workload CR")
		if shouldRequeue {
			return ctrl.Result{Requeue: true}
		}
		setWorkloadUpdateFailedCondition(componentWorkflowRun)
		return ctrl.Result{}
	}

	setWorkloadUpdatedCondition(componentWorkflowRun)
	return ctrl.Result{}
}

func (r *ComponentWorkflowRunReconciler) ensureRunResource(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	output *componentworkflowpipeline.RenderOutput,
	runResNamespace string,
	bpClient client.Client,
) ctrl.Result {
	logger := log.FromContext(ctx)

	serviceAccountName, err := extractServiceAccountName(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract service account name from rendered resource")
		return ctrl.Result{Requeue: true}
	}

	// Ensure prerequisite resources (namespace, RBAC) are created in the build plane
	if err := r.ensurePrerequisites(ctx, runResNamespace, serviceAccountName, bpClient); err != nil {
		logger.Error(err, "failed to ensure prerequisite resources")
		return ctrl.Result{Requeue: true}
	}

	// Apply additional resources (e.g., secrets, configmaps) before the main workflow
	if err := r.applyRenderedResources(ctx, componentWorkflowRun, output.Resources, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered resources")
		return ctrl.Result{Requeue: true}
	}

	if err := r.applyRenderedRunResource(ctx, componentWorkflowRun, output.Resource, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered run resource")
		return ctrl.Result{Requeue: true}
	}

	return ctrl.Result{Requeue: true}
}

func (r *ComponentWorkflowRunReconciler) syncWorkflowRunStatus(
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	runResource *argoproj.Workflow,
) ctrl.Result {
	switch runResource.Status.Phase {
	case argoproj.WorkflowRunning:
		setWorkflowRunningCondition(componentWorkflowRun)
		return ctrl.Result{RequeueAfter: 20 * time.Second}
	case argoproj.WorkflowSucceeded:
		setWorkflowSucceededCondition(componentWorkflowRun)
		if pushStep := getStepByTemplateName(runResource.Status.Nodes, engines.StepPush); pushStep != nil {
			if image := getImageNameFromRunResource(*pushStep.Outputs); image != "" {
				componentWorkflowRun.Status.ImageStatus.Image = string(image)
			}
		}
		return ctrl.Result{Requeue: true}
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		setWorkflowFailedCondition(componentWorkflowRun)
		return ctrl.Result{}
	default:
		return ctrl.Result{Requeue: true}
	}
}

func (r *ComponentWorkflowRunReconciler) applyRenderedRunResource(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	resource map[string]any,
	bpClient client.Client,
) error {
	resource = convertParameterValuesToStrings(resource)

	unstructuredResource := &unstructured.Unstructured{Object: resource}

	resourceNamespace := unstructuredResource.GetNamespace()
	if resourceNamespace == componentWorkflowRun.Namespace || resourceNamespace == "" {
		if err := ctrl.SetControllerReference(componentWorkflowRun, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	} else {
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/componentworkflowrun"] = componentWorkflowRun.Name
		labels["openchoreo.dev/componentworkflowrun-namespace"] = componentWorkflowRun.Namespace
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
			componentWorkflowRun.Status.RunReference.Name = name
			componentWorkflowRun.Status.RunReference.Namespace = namespace
			return nil
		}
		return fmt.Errorf("failed to get existing resource: %w", err)
	}

	unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
	if err := bpClient.Update(ctx, unstructuredResource); err != nil {
		return err
	}

	componentWorkflowRun.Status.RunReference.Name = name
	componentWorkflowRun.Status.RunReference.Namespace = namespace
	return nil
}

// applyRenderedResources applies additional rendered resources (e.g., secrets, configmaps) to the build plane.
func (r *ComponentWorkflowRunReconciler) applyRenderedResources(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	resources []componentworkflowpipeline.RenderedResource,
	bpClient client.Client,
) error {
	logger := log.FromContext(ctx)

	for _, res := range resources {
		unstructuredResource := &unstructured.Unstructured{Object: res.Resource}

		// Add labels to track ownership
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/componentworkflowrun"] = componentWorkflowRun.Name
		labels["openchoreo.dev/componentworkflowrun-namespace"] = componentWorkflowRun.Namespace
		labels["openchoreo.dev/resource-id"] = res.ID
		unstructuredResource.SetLabels(labels)

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
					return fmt.Errorf("failed to create resource %q: %w", res.ID, err)
				}
				logger.Info("created resource", "id", res.ID, "name", name, "namespace", namespace)
				continue
			}
			return fmt.Errorf("failed to get existing resource %q: %w", res.ID, err)
		}

		unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
		if err := bpClient.Update(ctx, unstructuredResource); err != nil {
			return fmt.Errorf("failed to update resource %q: %w", res.ID, err)
		}
		logger.Info("updated resource", "id", res.ID, "name", name, "namespace", namespace)
	}

	return nil
}

func (r *ComponentWorkflowRunReconciler) createWorkloadFromComponentWorkflowRun(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	bpClient client.Client,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("componentworkflowrun", componentWorkflowRun.Name)

	// Use the stored RunReference to retrieve the run resource
	if componentWorkflowRun.Status.RunReference.Name == "" || componentWorkflowRun.Status.RunReference.Namespace == "" {
		logger.Error(nil, "run resource reference not found in status")
		return true, fmt.Errorf("run resource reference not set in status")
	}

	runResource := &argoproj.Workflow{}
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      componentWorkflowRun.Status.RunReference.Name,
		Namespace: componentWorkflowRun.Status.RunReference.Namespace,
	}, runResource); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("run resource not found, skipping workload creation")
			return false, fmt.Errorf("run resource not found: %w", err)
		}
		return true, fmt.Errorf("failed to get run resource: %w", err)
	}

	workloadCR := extractWorkloadCRFromRunResource(runResource)
	if workloadCR == "" {
		logger.Info("no workload CR found in run resource outputs")
		return false, fmt.Errorf("no workload CR found in run resource outputs")
	}

	workload := &openchoreodevv1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadCR), workload); err != nil {
		return true, fmt.Errorf("failed to unmarshal workload CR: %w", err)
	}

	// Set the namespace to match the componentworkflowrun
	workload.Namespace = componentWorkflowRun.Namespace

	if err := r.Patch(ctx, workload, client.Apply, client.FieldOwner("componentworkflowrun-controller"), client.ForceOwnership); err != nil {
		return true, fmt.Errorf("failed to apply workload CR: %w", err)
	}

	return false, nil
}

func (r *ComponentWorkflowRunReconciler) getBuildPlaneClient(buildPlane *openchoreodevv1alpha1.BuildPlane) (client.Client, error) {
	bpClient, err := kubernetesClient.GetK8sClientFromBuildPlane(r.K8sClientMgr, buildPlane, r.GatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}
	return bpClient, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentWorkflowRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.K8sClientMgr == nil {
		r.K8sClientMgr = kubernetesClient.NewManager()
	}

	if r.Pipeline == nil {
		r.Pipeline = componentworkflowpipeline.NewPipeline()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.ComponentWorkflowRun{}).
		Named("componentworkflowrun").
		Complete(r)
}

// extractWorkloadCRFromRunResource extracts workload CR from run resource outputs
func extractWorkloadCRFromRunResource(runResource *argoproj.Workflow) string {
	for _, node := range runResource.Status.Nodes {
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

// getImageNameFromRunResource extracts the image name from run resource outputs
func getImageNameFromRunResource(output argoproj.Outputs) argoproj.AnyString {
	for _, param := range output.Parameters {
		if param.Name == "image" && param.Value != nil {
			return *param.Value
		}
	}
	return ""
}

// extractServiceAccountName extracts the service account name from the rendered run resource
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

// extractRunResourceNamespace extracts the namespace from rendered resource metadata
func extractRunResourceNamespace(resource map[string]any) (string, error) {
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
