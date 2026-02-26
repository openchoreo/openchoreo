// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/networkpolicy"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

const (
	httpRouteKind   = "HTTPRoute"
	gatewayAPIGroup = "gateway.networking.k8s.io"
)

// Reconciler reconciles a ReleaseBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline is the component rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different component types and reconciliations.
	Pipeline *componentpipeline.Pipeline

	// EnableNetworkPolicy enables NetworkPolicy generation for endpoint visibility enforcement.
	EnableNetworkPolicy bool
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=openchoreo.dev,resources=secretreferences,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Fetch ReleaseBinding (primary resource)
	releaseBinding := &openchoreov1alpha1.ReleaseBinding{}
	if err := r.Get(ctx, req.NamespacedName, releaseBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ReleaseBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Keep a copy for comparison
	old := releaseBinding.DeepCopy()

	// Handle deletion - run finalizer logic
	if !releaseBinding.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing releaseBinding")
		return r.finalize(ctx, old, releaseBinding)
	}

	// Ensure finalizer is added
	if finalizerAdded, err := r.ensureFinalizer(ctx, releaseBinding); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Deferred status update
	defer func() {
		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, releaseBinding.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, releaseBinding); err != nil {
			logger.Error(err, "Failed to update ReleaseBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Fetch ComponentRelease
	componentRelease := &openchoreov1alpha1.ComponentRelease{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      releaseBinding.Spec.ReleaseName,
		Namespace: releaseBinding.Namespace,
	}, componentRelease); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentRelease %q not found", releaseBinding.Spec.ReleaseName)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonComponentReleaseNotFound, msg)
			logger.Info(msg, "componentRelease", releaseBinding.Spec.ReleaseName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ComponentRelease", "componentRelease", releaseBinding.Spec.ReleaseName)
		return ctrl.Result{}, err
	}

	// Validate ComponentRelease configuration
	if err := r.validateComponentRelease(componentRelease, releaseBinding); err != nil {
		msg := fmt.Sprintf("Invalid ComponentRelease configuration: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonInvalidReleaseConfiguration, msg)
		logger.Error(err, "ComponentRelease validation failed")
		return ctrl.Result{}, nil
	}

	// Fetch Environment object
	environment := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      releaseBinding.Spec.Environment,
		Namespace: releaseBinding.Namespace,
	}, environment); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Environment %q not found", releaseBinding.Spec.Environment)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonEnvironmentNotFound, msg)
			logger.Info("Environment not found", "environment", releaseBinding.Spec.Environment)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Environment", "environment", releaseBinding.Spec.Environment)
		return ctrl.Result{}, err
	}

	// Fetch DataPlane or ClusterDataPlane object using the resolution function
	dataPlaneResult, err := controller.GetDataPlaneOrClusterDataPlaneOfEnv(ctx, r.Client, environment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("DataPlane not found for environment %q", environment.Name)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonDataPlaneNotFound, msg)
			logger.Info("DataPlane not found", "environment", environment.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to resolve DataPlane", "environment", environment.Name)
		return ctrl.Result{}, err
	}

	// Fetch Component object
	component := &openchoreov1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      componentRelease.Spec.Owner.ComponentName,
		Namespace: releaseBinding.Namespace,
	}, component); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Component %q not found", componentRelease.Spec.Owner.ComponentName)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonComponentNotFound, msg)
			logger.Info("Component not found", "component", componentRelease.Spec.Owner.ComponentName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Component", "component", componentRelease.Spec.Owner.ComponentName)
		return ctrl.Result{}, err
	}

	// Fetch Project object
	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      componentRelease.Spec.Owner.ProjectName,
		Namespace: releaseBinding.Namespace,
	}, project); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Project %q not found", componentRelease.Spec.Owner.ProjectName)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonProjectNotFound, msg)
			logger.Info("Project not found", "project", componentRelease.Spec.Owner.ProjectName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Project", "project", componentRelease.Spec.Owner.ProjectName)
		return ctrl.Result{}, err
	}

	return r.reconcileRelease(ctx, releaseBinding, componentRelease, environment, dataPlaneResult, component, project)
}

// validateComponentRelease validates the ComponentRelease configuration
func (r *Reconciler) validateComponentRelease(componentRelease *openchoreov1alpha1.ComponentRelease,
	releaseBinding *openchoreov1alpha1.ReleaseBinding) error {
	// Check ComponentType has resources
	if componentRelease.Spec.ComponentType.Resources == nil {
		return fmt.Errorf("component type has no resources")
	}

	// Check required owner fields
	if componentRelease.Spec.Owner.ProjectName == "" {
		return fmt.Errorf("component release owner missing required field: projectName")
	}
	if componentRelease.Spec.Owner.ComponentName == "" {
		return fmt.Errorf("component release owner missing required field: componentName")
	}

	// Check if the owners are matching in componentRelease and releaseBinding
	if releaseBinding.Spec.Owner.ProjectName != componentRelease.Spec.Owner.ProjectName ||
		releaseBinding.Spec.Owner.ComponentName != componentRelease.Spec.Owner.ComponentName {
		return fmt.Errorf("release binding owner (project: %q, component: %q) does not match "+
			"component release owner (project: %q, component: %q)",
			releaseBinding.Spec.Owner.ProjectName, releaseBinding.Spec.Owner.ComponentName,
			componentRelease.Spec.Owner.ProjectName, componentRelease.Spec.Owner.ComponentName)
	}

	return nil
}

// buildMetadataContext creates the MetadataContext from ComponentRelease, component, project, dataplane, and environment.
func (r *Reconciler) buildMetadataContext(
	componentRelease *openchoreov1alpha1.ComponentRelease,
	component *openchoreov1alpha1.Component,
	project *openchoreov1alpha1.Project,
	dataPlane *openchoreov1alpha1.DataPlane,
	environment *openchoreov1alpha1.Environment,
	environmentName string,
) pipelinecontext.MetadataContext {
	// Extract information
	namespaceName := componentRelease.Namespace
	projectName := componentRelease.Spec.Owner.ProjectName
	componentName := componentRelease.Spec.Owner.ComponentName
	componentUID := string(component.UID)
	projectUID := string(project.UID)
	dataPlaneName := dataPlane.Name
	dataPlaneUID := string(dataPlane.UID)
	environmentUID := string(environment.UID)

	// Generate base name using platform naming conventions
	// Format: {component}-{env}-{hash}
	baseName := dpkubernetes.GenerateK8sName(componentName, environmentName)

	// Generate namespace using platform naming conventions
	// Format: dp-{namespace}-{project}-{env}-{hash}
	namespace := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", namespaceName, projectName, environmentName,
	)

	// Build standard labels
	standardLabels := map[string]string{
		labels.LabelKeyNamespaceName:   namespaceName,
		labels.LabelKeyProjectName:     projectName,
		labels.LabelKeyComponentName:   componentName,
		labels.LabelKeyEnvironmentName: environmentName,
		labels.LabelKeyComponentUID:    componentUID,
		labels.LabelKeyEnvironmentUID:  environmentUID,
		labels.LabelKeyProjectUID:      projectUID,
	}

	// Build pod selectors
	podSelectors := map[string]string{
		labels.LabelKeyNamespaceName:   namespaceName,
		labels.LabelKeyProjectName:     projectName,
		labels.LabelKeyComponentName:   componentName,
		labels.LabelKeyEnvironmentName: environmentName,
		labels.LabelKeyComponentUID:    componentUID,
		labels.LabelKeyEnvironmentUID:  environmentUID,
		labels.LabelKeyProjectUID:      projectUID,
	}

	return pipelinecontext.MetadataContext{
		Name:               baseName,
		Namespace:          namespace,
		ComponentNamespace: namespaceName,
		Labels:             standardLabels,
		Annotations:        map[string]string{},
		PodSelectors:       podSelectors,
		ComponentName:      componentName,
		ComponentUID:       componentUID,
		ProjectName:        projectName,
		ProjectUID:         projectUID,
		DataPlaneName:      dataPlaneName,
		DataPlaneUID:       dataPlaneUID,
		EnvironmentName:    environmentName,
		EnvironmentUID:     environmentUID,
	}
}

// collectSecretReferences collects all SecretReferences needed for rendering from workload and releaseBinding.
func (r *Reconciler) collectSecretReferences(ctx context.Context, workload *openchoreov1alpha1.Workload, releaseBinding *openchoreov1alpha1.ReleaseBinding) (map[string]*openchoreov1alpha1.SecretReference, error) {
	secretRefs := make(map[string]*openchoreov1alpha1.SecretReference)

	// Helper function to collect secret reference
	collectSecretRef := func(refName string, namespace string) error {
		if refName == "" {
			return nil
		}
		if _, exists := secretRefs[refName]; !exists {
			secretRef := &openchoreov1alpha1.SecretReference{}
			if err := r.Get(ctx, client.ObjectKey{
				Name:      refName,
				Namespace: namespace,
			}, secretRef); err != nil {
				return fmt.Errorf("failed to get SecretReference %s: %w", refName, err)
			}
			secretRefs[refName] = secretRef
		}
		return nil
	}

	if workload != nil {
		container := workload.Spec.Container
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
				if err := collectSecretRef(env.ValueFrom.SecretRef.Name, workload.Namespace); err != nil {
					return nil, err
				}
			}
		}

		for _, file := range container.Files {
			if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
				if err := collectSecretRef(file.ValueFrom.SecretRef.Name, workload.Namespace); err != nil {
					return nil, err
				}
			}
		}
	}

	// Collect from releaseBinding workload overrides if present
	if releaseBinding.Spec.WorkloadOverrides != nil && releaseBinding.Spec.WorkloadOverrides.Container != nil {
		container := releaseBinding.Spec.WorkloadOverrides.Container
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
				if err := collectSecretRef(env.ValueFrom.SecretRef.Name, releaseBinding.Namespace); err != nil {
					return nil, err
				}
			}
		}

		for _, file := range container.Files {
			if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
				if err := collectSecretRef(file.ValueFrom.SecretRef.Name, releaseBinding.Namespace); err != nil {
					return nil, err
				}
			}
		}
	}

	return secretRefs, nil
}

// reconcileRelease creates or updates the Release resource and sets appropriate status conditions.
func (r *Reconciler) reconcileRelease(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease, environment *openchoreov1alpha1.Environment,
	dataPlaneResult *controller.DataPlaneResult, component *openchoreov1alpha1.Component, project *openchoreov1alpha1.Project) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Handle undeploy state - delete Release resources if they exist
	if releaseBinding.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		releaseBinding.Status.Endpoints = nil
		return r.handleUndeploy(ctx, releaseBinding, componentRelease)
	}

	// Build a facade DataPlane from the result for use by the pipeline and helper functions.
	// This works because ClusterDataPlane has the same spec fields (Gateway, SecretStoreRef, etc.).
	dataPlane := dataPlaneResult.ToDataPlane()

	// Build MetadataContext with computed names
	metadataContext := r.buildMetadataContext(componentRelease, component, project, dataPlane, environment, releaseBinding.Spec.Environment)

	// Fetch default notification channel for the environment if available
	// This will be passed to the rendering pipeline and made available in the trait CEL context
	defaultNotificationChannel, err := r.getDefaultNotificationChannelName(ctx, releaseBinding.Namespace, releaseBinding.Spec.Environment)
	if err != nil {
		if strings.Contains(err.Error(), "no default ObservabilityAlertsNotificationChannel found for environment") {
			logger.V(1).Info("Default notification channel not configured", "environment", releaseBinding.Spec.Environment)
			defaultNotificationChannel = ""
		} else {
			logger.Error(err, "Failed to resolve default notification channel", "environment", releaseBinding.Spec.Environment)
			return ctrl.Result{}, err
		}
	}

	// Build Component from ComponentRelease for rendering
	// The pipeline expects a Component object, so we need to reconstruct it from the ComponentRelease
	snapshotComponent := buildComponentFromRelease(componentRelease)
	snapshotComponentType := buildComponentTypeFromRelease(componentRelease)
	snapshotTraits := buildTraitsFromRelease(componentRelease)
	snapshotWorkload := buildWorkloadFromRelease(componentRelease)

	// Collect all SecretReferences needed for rendering (must be done after workload merge)
	secretReferences, err := r.collectSecretReferences(ctx, snapshotWorkload, releaseBinding)
	if err != nil {
		msg := fmt.Sprintf("Failed to collect SecretReferences: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to collect SecretReferences")
		return ctrl.Result{}, fmt.Errorf("failed to collect SecretReferences: %w", err)
	}

	// Prepare RenderInput
	renderInput := &componentpipeline.RenderInput{
		ComponentType:              snapshotComponentType,
		Component:                  snapshotComponent,
		Traits:                     snapshotTraits,
		Workload:                   snapshotWorkload,
		Environment:                environment,
		ReleaseBinding:             releaseBinding,
		DataPlane:                  dataPlane,
		SecretReferences:           secretReferences,
		Metadata:                   metadataContext,
		DefaultNotificationChannel: defaultNotificationChannel,
	}

	// Render resources using the shared pipeline instance
	renderOutput, err := r.Pipeline.Render(renderInput)
	if err != nil {
		msg := fmt.Sprintf("Failed to render resources: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to render resources")
		return ctrl.Result{}, fmt.Errorf("failed to render resources: %w", err)
	}

	// Log warnings if any
	if len(renderOutput.Metadata.Warnings) > 0 {
		logger.Info("Rendering completed with warnings",
			"warnings", renderOutput.Metadata.Warnings)
	}

	// Filter resources by target plane
	dataPlaneResources := make([]map[string]any, 0, len(renderOutput.Resources))
	observabilityPlaneResources := make([]map[string]any, 0, len(renderOutput.Resources))

	for _, renderedResource := range renderOutput.Resources {
		switch renderedResource.TargetPlane {
		case openchoreov1alpha1.TargetPlaneDataPlane:
			dataPlaneResources = append(dataPlaneResources, renderedResource.Resource)
		case openchoreov1alpha1.TargetPlaneObservabilityPlane:
			observabilityPlaneResources = append(observabilityPlaneResources, renderedResource.Resource)
		}
	}

	// Inject per-component NetworkPolicies into dataplane resources if enabled
	if r.EnableNetworkPolicy {
		componentNetpols := networkpolicy.MakeComponentPolicies(networkpolicy.ComponentPolicyParams{
			Namespace:     metadataContext.Namespace,
			CPNamespace:   metadataContext.ComponentNamespace,
			Environment:   metadataContext.EnvironmentName,
			ComponentName: metadataContext.ComponentName,
			PodSelectors:  metadataContext.PodSelectors,
			Endpoints:     snapshotWorkload.Spec.Endpoints,
		})
		dataPlaneResources = append(dataPlaneResources, componentNetpols...)
	}

	// Convert filtered dataplane resources to Release format
	dataPlaneReleaseResources, err := r.convertToReleaseResources(dataPlaneResources)
	if err != nil {
		msg := fmt.Sprintf("Failed to convert dataplane resources: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to convert dataplane resources to Release format")
		return ctrl.Result{}, fmt.Errorf("failed to convert dataplane resources: %w", err)
	}

	// Convert filtered observability plane resources to Release format
	observabilityPlaneReleaseResources, err := r.convertToReleaseResources(observabilityPlaneResources)
	if err != nil {
		msg := fmt.Sprintf("Failed to convert observability plane resources: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to convert observability plane resources to Release format")
		return ctrl.Result{}, fmt.Errorf("failed to convert observability plane resources: %w", err)
	}

	// Create or update dataplane Release
	dpReleaseName := makeDataPlaneReleaseName(componentRelease, releaseBinding)
	dataPlaneRelease := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpReleaseName,
			Namespace: releaseBinding.Namespace,
		},
	}

	dpOp, err := controllerutil.CreateOrUpdate(ctx, r.Client, dataPlaneRelease, func() error {
		// Check if we own this Release (only for existing releases)
		if dataPlaneRelease.UID != "" {
			hasOwner, err := controllerutil.HasOwnerReference(dataPlaneRelease.GetOwnerReferences(), releaseBinding, r.Scheme)
			if err != nil {
				return fmt.Errorf("failed to check owner reference: %w", err)
			}
			if !hasOwner {
				return fmt.Errorf("release exists but is not owned by this ReleaseBinding")
			}
		}

		dataPlaneRelease.Labels = map[string]string{
			labels.LabelKeyNamespaceName:   releaseBinding.Namespace,
			labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
		}

		dataPlaneRelease.Spec = openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   releaseBinding.Spec.Owner.ProjectName,
				ComponentName: releaseBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: releaseBinding.Spec.Environment,
			TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			Resources:       dataPlaneReleaseResources,
		}

		return controllerutil.SetControllerReference(releaseBinding, dataPlaneRelease, r.Scheme)
	})

	if err != nil {
		// Check for ownership conflict
		if strings.Contains(err.Error(), "not owned by") {
			msg := fmt.Sprintf("Release %q exists but is owned by another resource", dataPlaneRelease.Name)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonReleaseOwnershipConflict, msg)
			logger.Error(err, msg)
			return ctrl.Result{}, nil
		}

		// Transient errors
		msg := fmt.Sprintf("Failed to reconcile dataplane Release: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonReleaseUpdateFailed, msg)
		logger.Error(err, "Failed to reconcile dataplane Release", "release", dataPlaneRelease.Name)
		return ctrl.Result{}, err
	}

	// Reconcile observability plane Release (create, update, or cleanup)
	obsResult, err := r.reconcileObservabilityRelease(ctx, releaseBinding, componentRelease, dataPlaneResult, observabilityPlaneReleaseResources)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Check if we should return early due to ownership conflict
	if obsResult.ownershipConflict {
		return ctrl.Result{}, nil
	}

	// Resolve per-endpoint invoke URLs by matching HTTPRoute backendRef ports to workload endpoints.
	releaseBinding.Status.Endpoints = resolveEndpointURLStatuses(
		ctx,
		dataPlaneReleaseResources,
		componentRelease.Spec.Workload.Endpoints,
		environment,
		dataPlane,
	)

	// Resolve in-cluster Service URLs for all endpoints (including non-HTTP types like TCP, gRPC).
	releaseBinding.Status.Endpoints = resolveServiceURLs(
		ctx,
		dataPlaneReleaseResources,
		componentRelease.Spec.Workload.Endpoints,
		releaseBinding.Status.Endpoints,
	)

	// Set ReleaseSynced condition based on operation results.
	r.setReleaseSyncedCondition(releaseBinding, dataPlaneRelease.Name, dpOp, len(dataPlaneReleaseResources), obsResult)
	if dpOp == controllerutil.OperationResultCreated || dpOp == controllerutil.OperationResultUpdated {
		logger.Info("Releases reconciled",
			"dataplaneReleaseOp", dpOp,
			"dataplaneRelease", dataPlaneRelease.Name,
			"dataplaneResourceCount", len(dataPlaneReleaseResources),
			"observabilityReleaseOp", obsResult.operation,
			"observabilityReleaseName", obsResult.releaseName,
			"observabilityResourceCount", obsResult.resourceCount,
			"observabilityReleaseManaged", obsResult.managed)
		return ctrl.Result{Requeue: true}, nil
	}

	// Evaluate resource readiness from dataplane Release status (with component for workload type)
	if err := r.setResourcesReadyStatus(ctx, releaseBinding, dataPlaneRelease, component); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set resources ready status: %w", err)
	}

	// Set overall Ready condition based on ReleaseSynced and ResourcesReady
	r.setReadyCondition(releaseBinding)

	return ctrl.Result{}, nil
}

// handleUndeploy deletes the Release resources when ReleaseState is Undeploy.
// If the Release doesn't exist, marks the binding as undeployed.
// If the Release exists but is being deleted, reports "being undeployed".
func (r *Reconciler) handleUndeploy(ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get release names
	dpReleaseName := makeDataPlaneReleaseName(componentRelease, releaseBinding)
	obsReleaseName := makeObservabilityReleaseName(componentRelease, releaseBinding)

	// Track release existence and deletion state
	releaseFound := false
	deletionPending := false

	// Try to delete dataplane Release
	dataPlaneRelease := &openchoreov1alpha1.Release{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      dpReleaseName,
		Namespace: releaseBinding.Namespace,
	}, dataPlaneRelease)

	if err == nil {
		releaseFound = true
		if dataPlaneRelease.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, dataPlaneRelease); err != nil {
				err = fmt.Errorf("failed to delete dataplane release %q: %w", dpReleaseName, err)
				controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
					ReasonReleaseDeletionFailed, err.Error())
				return ctrl.Result{}, err
			}
			logger.Info("Deleted dataplane Release for undeploy", "release", dpReleaseName)
		}
		deletionPending = true
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get dataplane Release for undeploy: %w", err)
	}

	// Try to delete observability Release
	observabilityRelease := &openchoreov1alpha1.Release{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      obsReleaseName,
		Namespace: releaseBinding.Namespace,
	}, observabilityRelease)

	if err == nil {
		releaseFound = true
		if observabilityRelease.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, observabilityRelease); err != nil {
				err = fmt.Errorf("failed to delete observability release %q: %w", obsReleaseName, err)
				controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
					ReasonReleaseDeletionFailed, err.Error())
				return ctrl.Result{}, err
			}
			logger.Info("Deleted observability Release for undeploy", "release", obsReleaseName)
		}
		deletionPending = true
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get observability Release for undeploy: %w", err)
	}

	// Set conditions based on Release state
	if deletionPending {
		// Release(s) exist but being deleted
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonResourcesUndeployed, "Resources being undeployed")
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesUndeployed, "Resources being undeployed")
		controller.MarkFalseCondition(releaseBinding, ConditionReady,
			ReasonResourcesUndeployed, "Resources being undeployed")
		return ctrl.Result{}, nil
	}

	if !releaseFound {
		// All Releases are gone - mark as undeployed
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonResourcesUndeployed, "Resources undeployed")
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesUndeployed, "Resources undeployed")
		controller.MarkFalseCondition(releaseBinding, ConditionReady,
			ReasonResourcesUndeployed, "Resources undeployed")
		return ctrl.Result{}, nil
	}

	// Releases were just deleted, requeue to check completion
	return ctrl.Result{Requeue: true}, nil
}

// observabilityReleaseResult holds the result of reconciling an observability Release.
type observabilityReleaseResult struct {
	releaseName       string
	operation         controllerutil.OperationResult
	resourceCount     int
	managed           bool
	ownershipConflict bool
}

// reconcileObservabilityRelease handles the creation, update, or cleanup of the observability plane Release.
// It returns the result of the operation and any error encountered.
func (r *Reconciler) reconcileObservabilityRelease(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease,
	dataPlaneResult *controller.DataPlaneResult,
	observabilityPlaneReleaseResources []openchoreov1alpha1.Resource,
) (observabilityReleaseResult, error) {
	logger := log.FromContext(ctx)

	// Determine if we should create/manage an observability Release:
	// 1. ObservabilityPlane must exist (with default fallback)
	// 2. There must be observability plane resources to deploy
	shouldManage := false
	if len(observabilityPlaneReleaseResources) > 0 {
		// Try to resolve the ObservabilityPlane - this will use "default" if not explicitly specified
		_, err := dataPlaneResult.GetObservabilityPlane(ctx, r.Client)
		shouldManage = (err == nil)
	}

	releaseName := makeObservabilityReleaseName(componentRelease, releaseBinding)

	result := observabilityReleaseResult{
		releaseName:   releaseName,
		resourceCount: len(observabilityPlaneReleaseResources),
		managed:       shouldManage,
	}

	if shouldManage {
		// Create or update observability plane Release
		observabilityRelease := &openchoreov1alpha1.Release{
			ObjectMeta: metav1.ObjectMeta{
				Name:      releaseName,
				Namespace: releaseBinding.Namespace,
			},
		}

		opOp, err := controllerutil.CreateOrUpdate(ctx, r.Client, observabilityRelease, func() error {
			// Check if we own this Release (only for existing releases)
			if observabilityRelease.UID != "" {
				hasOwner, err := controllerutil.HasOwnerReference(observabilityRelease.GetOwnerReferences(), releaseBinding, r.Scheme)
				if err != nil {
					return fmt.Errorf("failed to check owner reference: %w", err)
				}
				if !hasOwner {
					return fmt.Errorf("release exists but is not owned by this ReleaseBinding")
				}
			}

			observabilityRelease.Labels = map[string]string{
				labels.LabelKeyNamespaceName:   releaseBinding.Namespace,
				labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
				labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
				labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
			}

			observabilityRelease.Spec = openchoreov1alpha1.ReleaseSpec{
				Owner: openchoreov1alpha1.ReleaseOwner{
					ProjectName:   releaseBinding.Spec.Owner.ProjectName,
					ComponentName: releaseBinding.Spec.Owner.ComponentName,
				},
				EnvironmentName: releaseBinding.Spec.Environment,
				TargetPlane:     openchoreov1alpha1.TargetPlaneObservabilityPlane,
				Resources:       observabilityPlaneReleaseResources,
			}

			return controllerutil.SetControllerReference(releaseBinding, observabilityRelease, r.Scheme)
		})

		if err != nil {
			// Check for ownership conflict
			if strings.Contains(err.Error(), "not owned by") {
				msg := fmt.Sprintf("Release %q exists but is owned by another resource", releaseName)
				controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
					ReasonReleaseOwnershipConflict, msg)
				logger.Error(err, msg)
				result.ownershipConflict = true
				return result, nil
			}

			// Transient errors
			msg := fmt.Sprintf("Failed to reconcile observability plane Release: %v", err)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonReleaseUpdateFailed, msg)
			logger.Error(err, "Failed to reconcile observability plane Release", "release", releaseName)
			return result, err
		}

		result.operation = opOp
		return result, nil
	}

	// Clean up existing observability Release if it exists but we no longer need it
	// (e.g., ObservabilityPlaneRef was removed or no more observability resources)
	existingObsRelease := &openchoreov1alpha1.Release{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      releaseName,
		Namespace: releaseBinding.Namespace,
	}, existingObsRelease); err == nil {
		// Check if we own this release before deleting
		hasOwner, ownerErr := controllerutil.HasOwnerReference(existingObsRelease.GetOwnerReferences(), releaseBinding, r.Scheme)
		if ownerErr == nil && hasOwner {
			if deleteErr := r.Delete(ctx, existingObsRelease); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				logger.Error(deleteErr, "Failed to delete stale observability Release", "release", releaseName)
				return result, deleteErr
			}
			logger.Info("Deleted stale observability Release",
				"release", releaseName,
				"reason", "no ObservabilityPlaneRef configured or no observability resources")
		}
	}

	// Mark as "skipped" for logging purposes
	result.operation = controllerutil.OperationResultNone
	return result, nil
}

// setReleaseSyncedCondition sets the ReleaseSynced condition based on operation results.
func (r *Reconciler) setReleaseSyncedCondition(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	dpReleaseName string,
	dpOp controllerutil.OperationResult,
	dpResourceCount int,
	obsResult observabilityReleaseResult,
) {
	switch dpOp {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		var msg string
		if obsResult.managed {
			msg = fmt.Sprintf("Dataplane Release %q %s with %d resources; observability Release %q %s with %d resources",
				dpReleaseName, dpOp, dpResourceCount,
				obsResult.releaseName, obsResult.operation, obsResult.resourceCount)
		} else {
			msg = fmt.Sprintf("Dataplane Release %q %s with %d resources (observability release skipped: no ObservabilityPlaneRef or no resources)",
				dpReleaseName, dpOp, dpResourceCount)
		}
		controller.MarkTrueCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseCreated, msg)

	case controllerutil.OperationResultNone:
		var msg string
		if obsResult.managed {
			msg = fmt.Sprintf("Dataplane Release %q is up to date; observability Release %q is %s",
				dpReleaseName, obsResult.releaseName, obsResult.operation)
		} else {
			msg = fmt.Sprintf("Dataplane Release %q is up to date (observability release skipped: no ObservabilityPlaneRef or no resources)",
				dpReleaseName)
		}
		controller.MarkTrueCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseSynced, msg)
	}
}

// Release naming helper functions

// makeDataPlaneReleaseName returns the name for a dataplane Release.
// Format: {componentName}-{environment}
func makeDataPlaneReleaseName(componentRelease *openchoreov1alpha1.ComponentRelease, releaseBinding *openchoreov1alpha1.ReleaseBinding) string {
	return fmt.Sprintf("%s-%s", componentRelease.Spec.Owner.ComponentName, releaseBinding.Spec.Environment)
}

// makeObservabilityReleaseName returns the name for an observability plane Release.
// Format: {componentName}-{environment}-observability
func makeObservabilityReleaseName(componentRelease *openchoreov1alpha1.ComponentRelease, releaseBinding *openchoreov1alpha1.ReleaseBinding) string {
	return fmt.Sprintf("%s-%s-observability", componentRelease.Spec.Owner.ComponentName, releaseBinding.Spec.Environment)
}

// Helper functions to build snapshot structures from ComponentRelease

func buildComponentFromRelease(componentRelease *openchoreov1alpha1.ComponentRelease) *openchoreov1alpha1.Component {
	var parameters *runtime.RawExtension
	var traits []openchoreov1alpha1.ComponentTrait

	if componentRelease.Spec.ComponentProfile != nil {
		parameters = componentRelease.Spec.ComponentProfile.Parameters
		traits = componentRelease.Spec.ComponentProfile.Traits
	}

	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentRelease.Spec.Owner.ComponentName,
			Namespace: componentRelease.Namespace,
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: componentRelease.Spec.Owner.ProjectName,
			},
			Parameters: parameters,
			Traits:     traits,
		},
	}
}

func buildComponentTypeFromRelease(componentRelease *openchoreov1alpha1.ComponentRelease) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "from-release", // Name doesn't matter for rendering
			Namespace: componentRelease.Namespace,
		},
		Spec: componentRelease.Spec.ComponentType,
	}
}

func buildTraitsFromRelease(componentRelease *openchoreov1alpha1.ComponentRelease) []openchoreov1alpha1.Trait {
	if len(componentRelease.Spec.Traits) == 0 {
		return nil
	}

	traits := make([]openchoreov1alpha1.Trait, 0, len(componentRelease.Spec.Traits))
	for name, spec := range componentRelease.Spec.Traits {
		traits = append(traits, openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: componentRelease.Namespace,
			},
			Spec: spec,
		})
	}
	return traits
}

func buildWorkloadFromRelease(componentRelease *openchoreov1alpha1.ComponentRelease) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "from-release", // Name doesn't matter for rendering
			Namespace: componentRelease.Namespace,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   componentRelease.Spec.Owner.ProjectName,
				ComponentName: componentRelease.Spec.Owner.ComponentName,
			},
			WorkloadTemplateSpec: componentRelease.Spec.Workload,
		},
	}
}

// convertToReleaseResources converts unstructured resources to Release.Resource format
func (r *Reconciler) convertToReleaseResources(
	resources []map[string]any,
) ([]openchoreov1alpha1.Resource, error) {
	releaseResources := make([]openchoreov1alpha1.Resource, 0, len(resources))

	for i, resource := range resources {
		// Generate resource ID
		id := r.generateResourceID(resource, i)

		// Marshal to JSON bytes
		rawJSON, err := json.Marshal(resource)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource to JSON (resourceID: %s): %w", id, err)
		}

		releaseResources = append(releaseResources, openchoreov1alpha1.Resource{
			ID: id,
			Object: &runtime.RawExtension{
				Raw: rawJSON,
			},
		})
	}
	return releaseResources, nil
}

// generateResourceID creates a unique ID for a resource
func (r *Reconciler) generateResourceID(resource map[string]any, index int) string {
	kind, _ := resource["kind"].(string)
	metadata, _ := resource["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)

	if kind != "" && name != "" {
		resourceID := fmt.Sprintf("%s-%s", strings.ToLower(kind), name)
		if len(resourceID) > dpkubernetes.MaxLabelNameLength {
			return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxLabelNameLength,
				strings.ToLower(kind),
				name)
		}
		return resourceID
	}

	// Fallback: use index
	return fmt.Sprintf("resource-%d", index)
}

// endpointMeta holds the type and visibility configuration of a workload endpoint.
type endpointMeta struct {
	endpointType openchoreov1alpha1.EndpointType
	visibility   []openchoreov1alpha1.EndpointVisibility
}

// endpointRoutes holds the indexed HTTPRoute objects for a single endpoint,
// split into external and internal buckets.
type endpointRoutes struct {
	external *unstructured.Unstructured
	internal *unstructured.Unstructured
}

// resolveEndpointURLStatuses matches each rendered HTTPRoute to a named workload endpoint
// using the openchoreo.dev/endpoint-name and openchoreo.dev/endpoint-visibility labels,
// then derives the invoke URL from the HTTPRoute hostname and optional path prefix.
// The gateway port is selected based on the endpoint visibility: external visibility uses
// the external ingress gateway; all other visibilities use the internal ingress gateway.
// Environment-level gateway configuration takes precedence over dataplane-level configuration.
func resolveEndpointURLStatuses(
	ctx context.Context,
	resources []openchoreov1alpha1.Resource,
	endpoints map[string]openchoreov1alpha1.WorkloadEndpoint,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
) []openchoreov1alpha1.EndpointURLStatus {
	logger := log.FromContext(ctx).WithName("endpoint-resolver")

	if len(endpoints) == 0 {
		logger.Info("No workload endpoints defined, skipping endpoint URL resolution")
		return nil
	}

	// Build a map of endpoint name → endpointMeta for HTTP-compatible endpoint types.
	// Only HTTP, GraphQL and Websocket endpoints are exposed via HTTPRoutes.
	httpEndpoints := make(map[string]endpointMeta, len(endpoints))
	for name, ep := range endpoints {
		switch ep.Type {
		case openchoreov1alpha1.EndpointTypeHTTP,
			openchoreov1alpha1.EndpointTypeGraphQL,
			openchoreov1alpha1.EndpointTypeWebsocket:
			httpEndpoints[name] = endpointMeta{
				endpointType: ep.Type,
				visibility:   ep.Visibility,
			}
			logger.Info("Registered HTTP-compatible endpoint", "name", name, "type", ep.Type)
		default:
			logger.Info("Skipping non-HTTP endpoint", "name", name, "type", ep.Type)
		}
	}

	// First pass: index HTTPRoutes by endpoint name into external/internal buckets.
	// No URL resolution happens here — just collect the objects.
	routeIndex := make(map[string]*endpointRoutes)
	for i := range resources {
		res := &resources[i]
		if res.Object == nil || len(res.Object.Raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(res.Object.Raw); err != nil {
			logger.Error(err, "Failed to unmarshal resource", "resourceID", res.ID)
			continue
		}

		if obj.GetKind() != httpRouteKind {
			continue
		}
		if obj.GetObjectKind().GroupVersionKind().Group != gatewayAPIGroup {
			continue
		}

		objLabels := obj.GetLabels()
		endpointName := objLabels[labels.LabelKeyEndpointName]
		if endpointName == "" {
			logger.Info("HTTPRoute missing endpoint-name label, skipping", "httpRouteName", obj.GetName())
			continue
		}

		if _, ok := httpEndpoints[endpointName]; !ok {
			logger.Info("HTTPRoute endpoint name not in supported HTTP endpoints, skipping",
				"httpRouteName", obj.GetName(),
				"endpointName", endpointName,
			)
			continue
		}

		if _, ok := routeIndex[endpointName]; !ok {
			routeIndex[endpointName] = &endpointRoutes{}
		}

		visibility := openchoreov1alpha1.EndpointVisibility(objLabels[labels.LabelKeyEndpointVisibility])
		if visibility == openchoreov1alpha1.EndpointVisibilityExternal {
			routeIndex[endpointName].external = obj
		} else {
			routeIndex[endpointName].internal = obj
		}
	}

	// Second pass: iterate endpoints in sorted order and build one EndpointURLStatus per endpoint.
	endpointNames := make([]string, 0, len(httpEndpoints))
	for name := range httpEndpoints {
		endpointNames = append(endpointNames, name)
	}
	sort.Strings(endpointNames)

	result := make([]openchoreov1alpha1.EndpointURLStatus, 0, len(endpointNames))
	for _, name := range endpointNames {
		routes, ok := routeIndex[name]
		if !ok {
			continue
		}

		status := openchoreov1alpha1.EndpointURLStatus{
			Name: name,
			Type: httpEndpoints[name].endpointType,
		}

		if routes.external != nil {
			hostname := extractFirstHostname(routes.external)
			gwEndpoint := resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, environment, dataPlane)
			if hostname == "" || gwEndpoint == nil {
				logger.Info("No external gateway endpoint configured, skipping", "endpointName", name)
			} else {
				status.ExternalURLs = buildGatewayURLs(hostname, extractFirstPathValue(routes.external), gwEndpoint)
				logger.Info("Resolved external endpoint URLs", "endpointName", name, "hostname", hostname)
			}
		}

		if routes.internal != nil {
			hostname := extractFirstHostname(routes.internal)
			visibilityStr := routes.internal.GetLabels()[labels.LabelKeyEndpointVisibility]
			gwEndpoint := resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibility(visibilityStr), environment, dataPlane)
			if hostname == "" || gwEndpoint == nil {
				logger.Info("No internal gateway endpoint configured, skipping",
					"endpointName", name, "visibility", visibilityStr)
			} else {
				status.InternalURLs = buildGatewayURLs(hostname, extractFirstPathValue(routes.internal), gwEndpoint)
				logger.Info("Resolved internal endpoint URLs", "endpointName", name, "hostname", hostname, "visibility", visibilityStr)
			}
		}

		result = append(result, status)
	}
	return result
}

// buildGatewayURLs constructs an EndpointGatewayURLs from the configured listeners in the given
// GatewayEndpointSpec. Each listener URL is only set when the corresponding listener is non-nil.
func buildGatewayURLs(hostname, path string, ep *openchoreov1alpha1.GatewayEndpointSpec) *openchoreov1alpha1.EndpointGatewayURLs {
	if ep == nil {
		return nil
	}
	urls := &openchoreov1alpha1.EndpointGatewayURLs{}
	if ep.HTTP != nil {
		urls.HTTP = buildInvokeURL("http", hostname, path, ep.HTTP.Port)
	}
	if ep.HTTPS != nil {
		urls.HTTPS = buildInvokeURL("https", hostname, path, ep.HTTPS.Port)
	}
	if ep.TLS != nil {
		urls.TLS = buildInvokeURL("https", hostname, path, ep.TLS.Port)
	}
	return urls
}

// buildInvokeURL constructs a single EndpointURL from the given components.
func buildInvokeURL(scheme, hostname, path string, port int32) *openchoreov1alpha1.EndpointURL {
	return &openchoreov1alpha1.EndpointURL{
		Scheme: scheme,
		Host:   hostname,
		Port:   port,
		Path:   path,
	}
}

// extractFirstHostname returns the first entry of spec.hostnames[], or "" if absent.
func extractFirstHostname(obj *unstructured.Unstructured) string {
	hostnames, found, err := unstructured.NestedStringSlice(obj.Object, "spec", "hostnames")
	if err != nil || !found || len(hostnames) == 0 {
		return ""
	}
	return hostnames[0]
}

// extractFirstPathValue walks spec.rules[0].matches[0].path.value and returns the path string.
// Returns "" when the HTTPRoute has no path match rule (e.g. root-path webapps).
func extractFirstPathValue(obj *unstructured.Unstructured) string {
	rules, found, err := unstructured.NestedSlice(obj.Object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return ""
	}

	firstRule, ok := rules[0].(map[string]interface{})
	if !ok {
		return ""
	}

	matches, found, err := unstructured.NestedSlice(firstRule, "matches")
	if err != nil || !found || len(matches) == 0 {
		return ""
	}

	firstMatch, ok := matches[0].(map[string]interface{})
	if !ok {
		return ""
	}

	value, _, _ := unstructured.NestedString(firstMatch, "path", "value")
	return value
}

// resolveGatewayEndpointByVisibility returns the GatewayEndpointSpec for the ingress gateway
// based on endpoint visibility. External visibility maps to the external gateway endpoint;
// all other visibilities (internal, namespace, project) map to the internal gateway endpoint.
// Environment-level gateway configuration takes precedence over dataplane-level configuration.
// Returns nil if no gateway endpoint is configured for the given visibility.
func resolveGatewayEndpointByVisibility(
	visibility openchoreov1alpha1.EndpointVisibility,
	env *openchoreov1alpha1.Environment,
	dp *openchoreov1alpha1.DataPlane,
) *openchoreov1alpha1.GatewayEndpointSpec {
	if dp == nil {
		return nil
	}

	// Use environment-level gateway config if present, otherwise fall back to dataplane.
	spec := dp.Spec.Gateway
	if env != nil && env.Spec.Gateway.Ingress != nil {
		spec = env.Spec.Gateway
	}

	// If no ingress configured, we can't resolve an endpoint as we don't support egress gateways.
	// TODO: (lahirude@wso2.com) Add support for egress gateway endpoints and update this logic accordingly.
	if spec.Ingress == nil {
		return nil
	}

	if visibility == openchoreov1alpha1.EndpointVisibilityExternal {
		return spec.Ingress.External
	}
	return spec.Ingress.Internal
}

// getDefaultNotificationChannelName returns the default ObservabilityAlertsNotificationChannel name for an environment.
func (r *Reconciler) getDefaultNotificationChannelName(ctx context.Context, namespace, environment string) (string, error) {
	var channels openchoreov1alpha1.ObservabilityAlertsNotificationChannelList
	if err := r.List(ctx, &channels, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("failed to list ObservabilityAlertsNotificationChannels: %w", err)
	}

	for _, ch := range channels.Items {
		if ch.Spec.Environment == environment && ch.Spec.IsEnvDefault && ch.DeletionTimestamp.IsZero() {
			return ch.Name, nil
		}
	}

	return "", fmt.Errorf("no default ObservabilityAlertsNotificationChannel found for environment %q", environment)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	// Setup field index for SecretReferences
	if err := r.setupSecretReferencesIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup SecretReferences index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ReleaseBinding{}).
		Owns(&openchoreov1alpha1.Release{}).
		Watches(&openchoreov1alpha1.Component{},
			handler.EnqueueRequestsFromMapFunc(r.findReleaseBindingsForComponent)).
		Watches(
			&openchoreov1alpha1.SecretReference{},
			handler.EnqueueRequestsFromMapFunc(r.listReleaseBindingsForSecretReference),
		).
		Named("releasebinding").
		Complete(r)
}
