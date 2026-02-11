// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// Reconciler reconciles a ComponentWorkflowRun object
type Reconciler struct {
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=secretreferences,verbs=get;list;watch
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
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

	// Handle deletion - finalize before anything else
	if !componentWorkflowRun.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing ComponentWorkflowRun")
		return r.finalize(ctx, componentWorkflowRun)
	}

	// Ensure finalizer is added
	if finalizerAdded, err := r.ensureFinalizer(ctx, componentWorkflowRun); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

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
		logger.Error(err, "failed to get build plane",
			"workflowrun", componentWorkflowRun.Name,
			"namespace", componentWorkflowRun.Namespace)
		return ctrl.Result{Requeue: true}, nil
	}

	bpClient, err := r.getBuildPlaneClient(buildPlane)
	if err != nil {
		logger.Error(err, "failed to get build plane client",
			"buildplane", buildPlane.Name,
			"workflowrun", componentWorkflowRun.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	if isWorkflowCompleted(componentWorkflowRun) {
		if isWorkflowSucceeded(componentWorkflowRun) {
			return r.handleWorkloadCreation(ctx, componentWorkflowRun, bpClient), nil
		}
		return ctrl.Result{}, nil
	}

	if componentWorkflowRun.Status.RunReference != nil && componentWorkflowRun.Status.RunReference.Name != "" && componentWorkflowRun.Status.RunReference.Namespace != "" {
		runResource := &argoproj.Workflow{}
		err = bpClient.Get(ctx, types.NamespacedName{
			Name:      componentWorkflowRun.Status.RunReference.Name,
			Namespace: componentWorkflowRun.Status.RunReference.Namespace,
		}, runResource)

		if err == nil {
			return r.syncWorkflowRunStatus(componentWorkflowRun, runResource), nil
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "failed to get run resource",
				"runName", componentWorkflowRun.Status.RunReference.Name,
				"runNamespace", componentWorkflowRun.Status.RunReference.Namespace)
			return ctrl.Result{Requeue: true}, nil
		}
		setWorkflowNotFoundCondition(componentWorkflowRun)
		return ctrl.Result{}, nil
	}

	// Validate ComponentWorkflow against ComponentType allowedWorkflows
	componentWorkflow, err := r.validateComponentWorkflow(ctx, componentWorkflowRun)
	if err != nil {
		return ctrl.Result{}, err
	}
	if componentWorkflow == nil {
		// Validation failed, condition already set
		return ctrl.Result{}, nil
	}

	// Resolve git secret if secretRef is provided
	var gitSecret *componentworkflowpipeline.GitSecretInfo
	if secretRef := componentWorkflowRun.Spec.Workflow.SystemParameters.Repository.SecretRef; secretRef != "" {
		gitSecretInfo, err := r.resolveGitSecret(ctx, componentWorkflowRun.Namespace, secretRef)
		if err != nil {
			logger.Error(err, "failed to resolve git secret",
				"secretRef", secretRef,
				"namespace", componentWorkflowRun.Namespace)
			// If SecretReference CR not found, set failure condition and don't requeue
			if errors.IsNotFound(err) {
				setSecretResolutionFailedCondition(componentWorkflowRun, err.Error())
				return ctrl.Result{}, nil
			}
			// For other transient errors (validation failures, empty data), requeue
			return ctrl.Result{Requeue: true}, nil
		}
		gitSecret = gitSecretInfo
	}

	renderInput := &componentworkflowpipeline.RenderInput{
		ComponentWorkflowRun: componentWorkflowRun,
		ComponentWorkflow:    componentWorkflow,
		Context: componentworkflowpipeline.ComponentWorkflowContext{
			NamespaceName:   componentWorkflowRun.Namespace,
			ProjectName:     componentWorkflowRun.Spec.Owner.ProjectName,
			ComponentName:   componentWorkflowRun.Spec.Owner.ComponentName,
			WorkflowRunName: componentWorkflowRun.Name,
			GitSecret:       gitSecret,
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

// validateComponentWorkflow validates that the referenced ComponentWorkflow exists
// and is in the allowedWorkflows list of the ComponentType.
// Returns the ComponentWorkflow on success, or nil with no error if validation failed
func (r *Reconciler) validateComponentWorkflow(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
) (*openchoreodevv1alpha1.ComponentWorkflow, error) {
	logger := log.FromContext(ctx)

	workflowName := componentWorkflowRun.Spec.Workflow.Name

	// Fetch the Component
	component := &openchoreodevv1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      componentWorkflowRun.Spec.Owner.ComponentName,
		Namespace: componentWorkflowRun.Namespace,
	}, component); err != nil {
		logger.Error(err, "failed to get Component", "component", componentWorkflowRun.Spec.Owner.ComponentName)
		return nil, err
	}

	// Parse componentType: {workloadType}/{componentTypeName}
	_, ctName, err := parseComponentType(component.Spec.ComponentType)
	if err != nil {
		msg := fmt.Sprintf("Invalid componentType format: %v", err)
		setWorkflowNotAllowedCondition(componentWorkflowRun, msg)
		logger.Error(err, "Failed to parse componentType")
		return nil, nil
	}

	// Fetch the ComponentType
	componentType := &openchoreodevv1alpha1.ComponentType{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      ctName,
		Namespace: componentWorkflowRun.Namespace,
	}, componentType); err != nil {
		if errors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentType %q not found", ctName)
			setWorkflowNotAllowedCondition(componentWorkflowRun, msg)
			logger.Info(msg, "workflowrun", componentWorkflowRun.Name)
			return nil, nil
		}
		// Transient error - requeue
		logger.Error(err, "failed to get ComponentType", "componentType", ctName)
		return nil, err
	}

	// Performance optimization: Check allowedWorkflows list first
	// This avoids fetching the ComponentWorkflow if it's not allowed
	if len(componentType.Spec.AllowedWorkflows) > 0 {
		allowedSet := make(map[string]bool, len(componentType.Spec.AllowedWorkflows))
		for _, name := range componentType.Spec.AllowedWorkflows {
			allowedSet[name] = true
		}

		if !allowedSet[workflowName] {
			msg := fmt.Sprintf("ComponentWorkflow %q is not allowed by ComponentType %q; allowed workflows: %v",
				workflowName, componentType.Name, componentType.Spec.AllowedWorkflows)
			setWorkflowNotAllowedCondition(componentWorkflowRun, msg)
			logger.Info(msg, "workflowrun", componentWorkflowRun.Name)
			return nil, nil
		}
	} else {
		// If allowedWorkflows is empty, no workflows are allowed
		msg := fmt.Sprintf("No ComponentWorkflows are allowed by ComponentType %q, but workflow run specifies workflow %q",
			componentType.Name, workflowName)
		setWorkflowNotAllowedCondition(componentWorkflowRun, msg)
		logger.Info(msg, "workflowrun", componentWorkflowRun.Name)
		return nil, nil
	}

	// Now check if the ComponentWorkflow actually exists
	componentWorkflow := &openchoreodevv1alpha1.ComponentWorkflow{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflowName,
		Namespace: componentWorkflowRun.Namespace,
	}, componentWorkflow); err != nil {
		if errors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentWorkflow %q not found", workflowName)
			setWorkflowNotAllowedCondition(componentWorkflowRun, msg)
			logger.Info(msg, "workflowrun", componentWorkflowRun.Name)
			return nil, nil
		}
		// Transient error - requeue
		logger.Error(err, "failed to get ComponentWorkflow", "workflow", workflowName)
		return nil, err
	}

	return componentWorkflow, nil
}

func (r *Reconciler) handleWorkloadCreation(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	bpClient client.Client) ctrl.Result {
	logger := log.FromContext(ctx)

	shouldRequeue, err := r.createWorkloadFromComponentWorkflowRun(ctx, componentWorkflowRun, bpClient)
	if err != nil {
		logger.Error(err, "failed to create workload CR",
			"workflowrun", componentWorkflowRun.Name,
			"namespace", componentWorkflowRun.Namespace)
		if shouldRequeue {
			return ctrl.Result{Requeue: true}
		}
		setWorkloadUpdateFailedCondition(componentWorkflowRun)
		return ctrl.Result{}
	}

	setWorkloadUpdatedCondition(componentWorkflowRun)
	return ctrl.Result{}
}

func (r *Reconciler) ensureRunResource(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	output *componentworkflowpipeline.RenderOutput,
	runResNamespace string,
	bpClient client.Client,
) ctrl.Result {
	logger := log.FromContext(ctx)

	serviceAccountName, err := extractServiceAccountName(output.Resource)
	if err != nil {
		logger.Error(err, "failed to extract service account name from rendered resource",
			"workflowrun", componentWorkflowRun.Name,
			"namespace", componentWorkflowRun.Namespace)
		return ctrl.Result{Requeue: true}
	}

	// Ensure prerequisite resources (namespace, RBAC) are created in the build plane
	if err := r.ensurePrerequisites(ctx, runResNamespace, serviceAccountName, bpClient); err != nil {
		logger.Error(err, "failed to ensure prerequisite resources",
			"workflowrun", componentWorkflowRun.Name)
		return ctrl.Result{Requeue: true}
	}

	// Apply additional resources (e.g., secrets, configmaps) before the main workflow
	appliedResources, err := r.applyRenderedResources(ctx, componentWorkflowRun, output.Resources, bpClient)
	if err != nil {
		logger.Error(err, "failed to apply rendered resources",
			"workflowrun", componentWorkflowRun.Name)
		return ctrl.Result{Requeue: true}
	}
	componentWorkflowRun.Status.Resources = appliedResources

	if err := r.applyRenderedRunResource(ctx, componentWorkflowRun, output.Resource, bpClient); err != nil {
		logger.Error(err, "failed to apply rendered run resource",
			"workflowrun", componentWorkflowRun.Name,
			"targetNamespace", runResNamespace)
		return ctrl.Result{Requeue: true}
	}

	return ctrl.Result{Requeue: true}
}

func (r *Reconciler) syncWorkflowRunStatus(
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	runResource *argoproj.Workflow,
) ctrl.Result {
	// Extract and update tasks from argo workflow nodes
	// This should be extended to support other workflow engines in the future
	componentWorkflowRun.Status.Tasks = extractArgoTasksFromWorkflowNodes(runResource.Status.Nodes)

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

func (r *Reconciler) applyRenderedRunResource(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	resource map[string]any,
	bpClient client.Client,
) error {
	logger := log.FromContext(ctx)

	resource = convertParameterValuesToStrings(resource)
	unstructuredResource := &unstructured.Unstructured{Object: resource}

	// Enforce namespace isolation: override namespace to openchoreo-ci-{namespaceName}
	// This ensures build resources are always created in the correct namespace,
	// regardless of what the ComponentWorkflow template specifies
	enforcedNamespace := fmt.Sprintf("openchoreo-ci-%s", componentWorkflowRun.Namespace)
	unstructuredResource.SetNamespace(enforcedNamespace)

	name := unstructuredResource.GetName()
	namespace := unstructuredResource.GetNamespace()
	kind := unstructuredResource.GetKind()

	// Set ownership tracking via controller reference or labels
	if namespace == componentWorkflowRun.Namespace || namespace == "" {
		if err := ctrl.SetControllerReference(componentWorkflowRun, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for %s %q in namespace %q: %w", kind, name, namespace, err)
		}
	} else {
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/componentworkflowrun"] = componentWorkflowRun.Name
		labels["openchoreo.dev/componentworkflowrun-namespace"] = componentWorkflowRun.Namespace
		labels["openchoreo.dev/managed-by"] = "componentworkflowrun-controller"
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
	componentWorkflowRun.Status.RunReference = &openchoreodevv1alpha1.ResourceReference{
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
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	resources []componentworkflowpipeline.RenderedResource,
	bpClient client.Client,
) (*[]openchoreodevv1alpha1.ResourceReference, error) {
	logger := log.FromContext(ctx)

	if len(resources) == 0 {
		return nil, nil
	}

	appliedResources := make([]openchoreodevv1alpha1.ResourceReference, 0, len(resources))

	for _, res := range resources {
		unstructuredResource := &unstructured.Unstructured{Object: res.Resource}

		// Enforce namespace isolation: override namespace to openchoreo-ci-{namespaceName}
		// This ensures build resources are always created in the correct namespace,
		// regardless of what the ComponentWorkflow template specifies
		enforcedNamespace := fmt.Sprintf("openchoreo-ci-%s", componentWorkflowRun.Namespace)
		unstructuredResource.SetNamespace(enforcedNamespace)

		// Add labels to track ownership
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/componentworkflowrun"] = componentWorkflowRun.Name
		labels["openchoreo.dev/componentworkflowrun-namespace"] = componentWorkflowRun.Namespace
		labels["openchoreo.dev/managed-by"] = "componentworkflowrun-controller"
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

func (r *Reconciler) createWorkloadFromComponentWorkflowRun(
	ctx context.Context,
	componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun,
	bpClient client.Client,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("componentworkflowrun", componentWorkflowRun.Name)

	// Use the stored RunReference to retrieve the run resource
	if componentWorkflowRun.Status.RunReference == nil || componentWorkflowRun.Status.RunReference.Name == "" || componentWorkflowRun.Status.RunReference.Namespace == "" {
		logger.Error(nil, "run resource reference not found in status")
		return true, fmt.Errorf("run resource reference not set in status")
	}

	runRefName := componentWorkflowRun.Status.RunReference.Name
	runRefNamespace := componentWorkflowRun.Status.RunReference.Namespace

	runResource := &argoproj.Workflow{}
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      runRefName,
		Namespace: runRefNamespace,
	}, runResource); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("run resource not found, skipping workload creation",
				"runName", runRefName,
				"runNamespace", runRefNamespace)
			return false, fmt.Errorf("run resource %q in namespace %q not found: %w", runRefName, runRefNamespace, err)
		}
		return true, fmt.Errorf("failed to get run resource %q in namespace %q: %w", runRefName, runRefNamespace, err)
	}

	workloadCR := extractWorkloadCRFromRunResource(runResource)
	if workloadCR == "" {
		logger.Info("no workload CR found in run resource outputs",
			"runName", runRefName,
			"runNamespace", runRefNamespace)
		return false, fmt.Errorf("no workload CR found in run resource %q outputs", runRefName)
	}

	workload := &openchoreodevv1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadCR), workload); err != nil {
		return true, fmt.Errorf("failed to unmarshal workload CR from run resource %q: %w", runRefName, err)
	}

	// Set the namespace to match the componentworkflowrun
	workload.Namespace = componentWorkflowRun.Namespace

	if err := r.Patch(ctx, workload, client.Apply, client.FieldOwner("componentworkflowrun-controller"), client.ForceOwnership); err != nil {
		return true, fmt.Errorf("failed to apply workload %q in namespace %q: %w", workload.Name, workload.Namespace, err)
	}

	return false, nil
}

func (r *Reconciler) getBuildPlaneClient(buildPlane *openchoreodevv1alpha1.BuildPlane) (client.Client, error) {
	bpClient, err := kubernetesClient.GetK8sClientFromBuildPlane(r.K8sClientMgr, buildPlane, r.GatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}
	return bpClient, nil
}

// resolveGitSecret reads the SecretReference CR and extracts git secret information for template rendering.
// Returns an error if the SecretReference CR is not found or has no data sources.
// If fields within the SecretReference have empty values, they are rendered as empty strings,
// allowing the pipeline to skip resources with invalid names during rendering.
func (r *Reconciler) resolveGitSecret(ctx context.Context, namespace, secretRefName string) (*componentworkflowpipeline.GitSecretInfo, error) {
	secretRef := &openchoreodevv1alpha1.SecretReference{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      secretRefName,
		Namespace: namespace,
	}, secretRef); err != nil {
		return nil, fmt.Errorf("failed to get SecretReference %q in namespace %q: %w", secretRefName, namespace, err)
	}

	// Extract all data sources from the SecretReference
	if len(secretRef.Spec.Data) == 0 {
		return nil, fmt.Errorf("SecretReference %q has no data sources", secretRefName)
	}

	// Convert all SecretDataSource entries to SecretDataInfo
	dataInfos := make([]componentworkflowpipeline.SecretDataInfo, len(secretRef.Spec.Data))
	for i, dataSource := range secretRef.Spec.Data {
		dataInfos[i] = componentworkflowpipeline.SecretDataInfo{
			SecretKey: dataSource.SecretKey,
			RemoteRef: componentworkflowpipeline.RemoteRefInfo{
				Key:      dataSource.RemoteRef.Key,
				Property: dataSource.RemoteRef.Property, // Property is optional
			},
		}
	}

	// Extract secret type from spec.template.type
	secretType := string(secretRef.Spec.Template.Type)
	if secretType == "" {
		secretType = "kubernetes.io/basic-auth" //nolint:gosec // False positive: this is a secret type constant, not credentials
	}

	// Return GitSecretInfo with all data sources.
	// The rendering pipeline will check field validity and skip resources via includeWhen.
	return &componentworkflowpipeline.GitSecretInfo{
		Name: secretRefName,
		Type: secretType,
		Data: dataInfos,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		if node.TemplateName == "generate-workload-cr" && node.Phase == argoproj.NodeSucceeded {
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
			task.FinishedAt = &finishedAt
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

// parseComponentType parses the componentType string in format {workloadType}/{componentTypeName}
func parseComponentType(componentType string) (workloadType string, ctName string, err error) {
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid componentType format: expected {workloadType}/{name}, got %s", componentType)
	}
	return parts[0], parts[1], nil
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
