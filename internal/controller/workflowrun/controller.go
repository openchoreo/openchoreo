// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

// Reconciler reconciles a WorkflowRun object
type Reconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	K8sClientMgr *kubernetesClient.KubeMultiClientManager

	// Pipeline is the workflow rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different workflow runs and reconciliations.
	Pipeline   *workflowpipeline.Pipeline
	GatewayURL string
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx).WithValues("workflowrun", req.NamespacedName)

	workflowRun := &openchoreodevv1alpha1.WorkflowRun{}
	if err := r.Get(ctx, req.NamespacedName, workflowRun); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	// Keep a copy for comparison
	old := workflowRun.DeepCopy()

	// Handle deletion - finalize before anything else
	if !workflowRun.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing WorkflowRun")
		return r.finalize(ctx, workflowRun)
	}

	// Ensure finalizer is added
	if finalizerAdded, err := r.ensureFinalizer(ctx, workflowRun); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Check if TTL has expired
	if shouldReturn, result, err := r.checkTTLExpiration(ctx, workflowRun); shouldReturn {
		return result, err
	}

	// Deferred status update
	defer func() {
		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, workflowRun.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, workflowRun); err != nil {
			logger.Error(err, "Failed to update WorkflowRun status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	if !isWorkflowInitiated(workflowRun) {
		setStartedAtIfNeeded(workflowRun)
		setWorkflowPendingCondition(workflowRun)
		return ctrl.Result{Requeue: true}, nil
	}

	buildPlaneResult, err := controller.ResolveBuildPlane(ctx, r.Client, workflowRun)
	if err != nil {
		logger.Error(err, "failed to get build plane",
			"workflowrun", workflowRun.Name,
			"namespace", workflowRun.Namespace)
		setBuildPlaneResolutionFailedCondition(workflowRun, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if buildPlaneResult == nil {
		logger.Info("No build plane found for project",
			"workflowrun", workflowRun.Name)
		setBuildPlaneNotFoundCondition(workflowRun)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	bpClient, err := r.getBuildPlaneClient(buildPlaneResult)
	if err != nil {
		logger.Error(err, "failed to get build plane client",
			"buildplane", buildPlaneResult.GetName(),
			"workflowrun", workflowRun.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	if isWorkflowCompleted(workflowRun) {
		// Set CompletedAt timestamp if not already set
		setCompletedAtIfNeeded(workflowRun)

		return ctrl.Result{}, nil
	}

	if workflowRun.Status.RunReference != nil && workflowRun.Status.RunReference.Name != "" && workflowRun.Status.RunReference.Namespace != "" {
		runResource := &argoproj.Workflow{}
		err = bpClient.Get(ctx, types.NamespacedName{
			Name:      workflowRun.Status.RunReference.Name,
			Namespace: workflowRun.Status.RunReference.Namespace,
		}, runResource)

		if err == nil {
			return r.syncWorkflowRunStatus(workflowRun, runResource), nil
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "failed to get run resource",
				"runName", workflowRun.Status.RunReference.Name,
				"runNamespace", workflowRun.Status.RunReference.Namespace)
			return ctrl.Result{Requeue: true}, nil
		}
		setWorkflowNotFoundCondition(workflowRun)
		return ctrl.Result{}, nil
	}

	workflow := &openchoreodevv1alpha1.Workflow{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflowRun.Spec.Workflow.Name,
		Namespace: workflowRun.Namespace,
	}, workflow); err != nil {
		logger.Error(err, "failed to get Workflow",
			"workflow", workflowRun.Spec.Workflow.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// Copy TTL from Workflow to WorkflowRun if not already set
	if workflowRun.Spec.TTLAfterCompletion == "" && workflow.Spec.TTLAfterCompletion != "" {
		workflowRun.Spec.TTLAfterCompletion = workflow.Spec.TTLAfterCompletion
		if err := r.Update(ctx, workflowRun); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	renderInput := &workflowpipeline.RenderInput{
		WorkflowRun: workflowRun,
		Workflow:    workflow,
		Context: workflowpipeline.WorkflowContext{
			NamespaceName:   workflowRun.Namespace,
			WorkflowRunName: workflowRun.Name,
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

	return r.ensureRunResource(ctx, workflowRun, output, runResNamespace, bpClient), nil
}

func (r *Reconciler) ensureRunResource(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	output *workflowpipeline.RenderOutput,
	runResNamespace string,
	bpClient client.Client,
) ctrl.Result {
	logger := log.FromContext(ctx)

	serviceAccountName, err := extractServiceAccountName(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract service account name from rendered resource",
			"workflowrun", workflowRun.Name,
			"namespace", workflowRun.Namespace)
		return ctrl.Result{Requeue: true}
	}

	// Ensure prerequisite resources (namespace, RBAC) are created in the build plane
	if err := r.ensurePrerequisites(ctx, runResNamespace, serviceAccountName, bpClient); err != nil {
		logger.Error(err, "failed to ensure prerequisite resources",
			"workflowrun", workflowRun.Name)
		return ctrl.Result{Requeue: true}
	}

	// Apply additional resources (e.g., secrets, configmaps) before the main workflow
	appliedResources, err := r.applyRenderedResources(ctx, workflowRun, output.Resources, bpClient)
	if err != nil {
		logger.Error(err, "failed to apply rendered resources",
			"workflowrun", workflowRun.Name)
		return ctrl.Result{Requeue: true}
	}
	workflowRun.Status.Resources = appliedResources

	if err := r.applyRenderedRunResource(ctx, workflowRun, output.Resource, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered run resource",
			"workflowrun", workflowRun.Name,
			"targetNamespace", runResNamespace)
		return ctrl.Result{Requeue: true}
	}

	return ctrl.Result{Requeue: true}
}

func (r *Reconciler) syncWorkflowRunStatus(
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	runResource *argoproj.Workflow,
) ctrl.Result {
	// Extract and update tasks from argo workflow nodes
	// This should be extended to support other workflow engines in the future
	workflowRun.Status.Tasks = extractArgoTasksFromWorkflowNodes(runResource.Status.Nodes)

	switch runResource.Status.Phase {
	case argoproj.WorkflowRunning:
		setWorkflowRunningCondition(workflowRun)
		return ctrl.Result{RequeueAfter: 20 * time.Second}
	case argoproj.WorkflowSucceeded:
		setWorkflowSucceededCondition(workflowRun)
		return ctrl.Result{Requeue: true}
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		setWorkflowFailedCondition(workflowRun)
		return ctrl.Result{}
	default:
		return ctrl.Result{Requeue: true}
	}
}

func (r *Reconciler) applyRenderedRunResource(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	resource map[string]any,
	bpClient client.Client,
) error {
	logger := log.FromContext(ctx)

	resource = convertParameterValuesToStrings(resource)
	unstructuredResource := &unstructured.Unstructured{Object: resource}

	name := unstructuredResource.GetName()
	namespace := unstructuredResource.GetNamespace()
	kind := unstructuredResource.GetKind()

	// Set ownership tracking via controller reference or labels
	if namespace == workflowRun.Namespace || namespace == "" {
		if err := ctrl.SetControllerReference(workflowRun, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for %s %q in namespace %q: %w", kind, name, namespace, err)
		}
	} else {
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/workflowrun"] = workflowRun.Name
		labels["openchoreo.dev/workflowrun-namespace"] = workflowRun.Namespace
		labels["openchoreo.dev/managed-by"] = "workflowrun-controller"
		unstructuredResource.SetLabels(labels)
	}

	// Check if resource already exists
	existingResource := &unstructured.Unstructured{}
	existingResource.SetGroupVersionKind(unstructuredResource.GroupVersionKind())

	err := bpClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existingResource)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get %s %q in namespace %q: %w", kind, name, namespace, err)
		}
		// Resource doesn't exist, create it
		if err := bpClient.Create(ctx, unstructuredResource); err != nil {
			return fmt.Errorf("failed to create %s %q in namespace %q: %w", kind, name, namespace, err)
		}
		logger.Info("created run resource", "kind", kind, "name", name, "namespace", namespace)
	} else {
		// Resource exists, update it
		unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
		if err := bpClient.Update(ctx, unstructuredResource); err != nil {
			return fmt.Errorf("failed to update %s %q in namespace %q: %w", kind, name, namespace, err)
		}
		logger.Info("updated run resource", "kind", kind, "name", name, "namespace", namespace)
	}

	// Update status with run resource reference
	workflowRun.Status.RunReference = &openchoreodevv1alpha1.ResourceReference{
		APIVersion: unstructuredResource.GetAPIVersion(),
		Kind:       unstructuredResource.GetKind(),
		Name:       name,
		Namespace:  namespace,
	}

	return nil
}

// applyRenderedResources applies additional rendered resources (e.g., secrets, configmaps) to the build plane.
func (r *Reconciler) applyRenderedResources(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	resources []workflowpipeline.RenderedResource,
	bpClient client.Client,
) (*[]openchoreodevv1alpha1.ResourceReference, error) {
	logger := log.FromContext(ctx)

	if len(resources) == 0 {
		return nil, nil
	}

	appliedResources := make([]openchoreodevv1alpha1.ResourceReference, 0, len(resources))

	for _, res := range resources {
		unstructuredResource := &unstructured.Unstructured{Object: res.Resource}

		// Add labels to track ownership
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/workflowrun"] = workflowRun.Name
		labels["openchoreo.dev/workflowrun-namespace"] = workflowRun.Namespace
		labels["openchoreo.dev/managed-by"] = "workflowrun-controller"
		unstructuredResource.SetLabels(labels)

		existingResource := &unstructured.Unstructured{}
		existingResource.SetGroupVersionKind(unstructuredResource.GroupVersionKind())

		namespace := unstructuredResource.GetNamespace()
		name := unstructuredResource.GetName()
		kind := unstructuredResource.GetKind()

		err := bpClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, existingResource)

		if err != nil {
			if errors.IsNotFound(err) {
				if err := bpClient.Create(ctx, unstructuredResource); err != nil {
					return nil, fmt.Errorf("failed to create %s %q in namespace %q: %w", kind, name, namespace, err)
				}
				logger.Info("created resource", "id", res.ID, "kind", kind, "name", name, "namespace", namespace)
			} else {
				return nil, fmt.Errorf("failed to get %s %q in namespace %q: %w", kind, name, namespace, err)
			}
		} else {
			unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())
			if err := bpClient.Update(ctx, unstructuredResource); err != nil {
				return nil, fmt.Errorf("failed to update %s %q in namespace %q: %w", kind, name, namespace, err)
			}
			logger.Info("updated resource", "id", res.ID, "kind", kind, "name", name, "namespace", namespace)
		}

		// Track the applied resource for status update
		gvk := unstructuredResource.GroupVersionKind()
		appliedResources = append(appliedResources, openchoreodevv1alpha1.ResourceReference{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       name,
			Namespace:  namespace,
		})
	}

	return &appliedResources, nil
}

func (r *Reconciler) getBuildPlaneClient(buildPlaneResult *controller.BuildPlaneResult) (client.Client, error) {
	bpClient, err := buildPlaneResult.GetK8sClient(r.K8sClientMgr, r.GatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}
	return bpClient, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.K8sClientMgr == nil {
		r.K8sClientMgr = kubernetesClient.NewManager()
	}

	if r.Pipeline == nil {
		r.Pipeline = workflowpipeline.NewPipeline()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.WorkflowRun{}).
		Named("workflowrun").
		Complete(r)
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

// taskWithOrder holds a task with its execution order for sorting.
type taskWithOrder struct {
	task  openchoreodevv1alpha1.WorkflowTask
	order int
}

// extractArgoTasksFromWorkflowNodes extracts workflow tasks from Argo Workflow nodes.
// It filters nodes by type "Pod" (actual step executions) and orders them by their
// step index extracted from the node name (e.g., "workflow-name[0].step-name").
func extractArgoTasksFromWorkflowNodes(nodes argoproj.Nodes) []openchoreodevv1alpha1.WorkflowTask {
	if nodes == nil {
		return nil
	}

	// Collect Pod nodes with their order index
	tasksWithOrder := make([]taskWithOrder, 0, len(nodes))

	for _, node := range nodes {
		// Only consider Pod nodes - these are the actual step executions
		if node.Type != argoproj.NodeTypePod {
			continue
		}

		// Extract order from node name (e.g., "workflow-name[0].step-name" -> 0)
		order := extractArgoStepOrderFromNodeName(node.Name)

		task := openchoreodevv1alpha1.WorkflowTask{
			Name:    node.TemplateName,
			Phase:   string(node.Phase),
			Message: node.Message,
		}

		// Set timestamps if available
		if !node.StartedAt.IsZero() {
			startedAt := node.StartedAt
			task.StartedAt = &startedAt
		}
		if !node.FinishedAt.IsZero() {
			finishedAt := node.FinishedAt
			task.CompletedAt = &finishedAt
		}

		tasksWithOrder = append(tasksWithOrder, taskWithOrder{task: task, order: order})
	}

	// Sort by order using insertion sort
	for i := 1; i < len(tasksWithOrder); i++ {
		key := tasksWithOrder[i]
		j := i - 1
		for j >= 0 && tasksWithOrder[j].order > key.order {
			tasksWithOrder[j+1] = tasksWithOrder[j]
			j--
		}
		tasksWithOrder[j+1] = key
	}

	// Extract sorted tasks
	tasks := make([]openchoreodevv1alpha1.WorkflowTask, len(tasksWithOrder))
	for i, t := range tasksWithOrder {
		tasks[i] = t.task
	}

	return tasks
}

// extractArgoStepOrderFromNodeName extracts the step order from a node name.
// Node names follow the pattern: "workflow-name[N].step-name" where N is the order.
// Returns -1 if the order cannot be extracted.
func extractArgoStepOrderFromNodeName(nodeName string) int {
	// Find the bracket containing the order number
	startIdx := -1
	endIdx := -1
	for i := len(nodeName) - 1; i >= 0; i-- {
		if nodeName[i] == ']' && endIdx == -1 {
			endIdx = i
		}
		if nodeName[i] == '[' && endIdx != -1 {
			startIdx = i
			break
		}
	}

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return -1
	}

	// Parse the number between brackets
	orderStr := nodeName[startIdx+1 : endIdx]
	var order int
	if _, err := fmt.Sscanf(orderStr, "%d", &order); err != nil {
		return -1
	}

	return order
}

// checkTTLExpiration checks if the TTL has expired and deletes the WorkflowRun if needed.
// Returns (shouldReturn, result, error) where shouldReturn indicates if the caller should return immediately.
func (r *Reconciler) checkTTLExpiration(
	ctx context.Context,
	wfRun *openchoreodevv1alpha1.WorkflowRun,
) (bool, ctrl.Result, error) {
	if wfRun.Status.CompletedAt == nil || wfRun.Spec.TTLAfterCompletion == "" {
		return false, ctrl.Result{}, nil
	}

	logger := log.FromContext(ctx)
	ttlDuration, err := cmdutil.ParseDuration(wfRun.Spec.TTLAfterCompletion)
	if err != nil {
		logger.Error(err, "Invalid TTL duration", "ttl", wfRun.Spec.TTLAfterCompletion)
		return false, ctrl.Result{}, nil
	}

	expirationTime := wfRun.Status.CompletedAt.Add(ttlDuration)
	if time.Now().After(expirationTime) {
		logger.Info("TTL expired, deleting WorkflowRun",
			"completedAt", wfRun.Status.CompletedAt.Time,
			"ttl", wfRun.Spec.TTLAfterCompletion,
			"expiredAt", expirationTime)
		if err := r.Delete(ctx, wfRun); err != nil {
			if !errors.IsNotFound(err) {
				return true, ctrl.Result{}, err
			}
		}
		return true, ctrl.Result{}, nil
	}

	// Requeue to check again when TTL expires
	requeueAfter := time.Until(expirationTime)
	if requeueAfter > 0 {
		logger.V(1).Info("Requeuing for TTL check", "requeueAfter", requeueAfter)
		return true, ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return false, ctrl.Result{}, nil
}

// setStartedAtIfNeeded sets StartedAt when the controller starts processing the workflow run, if it hasn't been set already.
func setStartedAtIfNeeded(cwRun *openchoreodevv1alpha1.WorkflowRun) {
	if cwRun.Status.StartedAt != nil {
		return
	}
	now := metav1.Now()
	cwRun.Status.StartedAt = &now
}

// setCompletedAtIfNeeded sets the CompletedAt timestamp when the workflow completes.
func setCompletedAtIfNeeded(wfRun *openchoreodevv1alpha1.WorkflowRun) {
	if wfRun.Status.CompletedAt != nil {
		return
	}

	now := metav1.Now()
	wfRun.Status.CompletedAt = &now
	log.Log.Info("Workflow completed", "completedAt", now, "workflowrun", wfRun.Name)
}
