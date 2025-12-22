// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// Reconciler reconciles a ReleaseBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline is the component rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different component types and reconciliations.
	Pipeline *componentpipeline.Pipeline
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
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

	// Check if DataPlaneRef is configured in the Environment
	if environment.Spec.DataPlaneRef == "" {
		msg := fmt.Sprintf("Environment %q has no DataPlaneRef configured", environment.Name)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonDataPlaneNotConfigured, msg)
		logger.Info("Environment has no DataPlaneRef", "environment", environment.Name)
		return ctrl.Result{}, nil
	}

	// Fetch DataPlane object
	dataPlane := &openchoreov1alpha1.DataPlane{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      environment.Spec.DataPlaneRef,
		Namespace: releaseBinding.Namespace,
	}, dataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("DataPlane %q not found", environment.Spec.DataPlaneRef)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonDataPlaneNotFound, msg)
			logger.Info("DataPlane not found", "dataPlane", environment.Spec.DataPlaneRef)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get DataPlane", "dataPlane", environment.Spec.DataPlaneRef)
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

	return r.reconcileRelease(ctx, releaseBinding, componentRelease, environment, dataPlane, component, project)
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
	organizationName := componentRelease.Namespace
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
	// Format: dp-{org}-{project}-{env}-{hash}
	namespace := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName,
	)

	// Build standard labels
	standardLabels := map[string]string{
		labels.LabelKeyOrganizationName: organizationName,
		labels.LabelKeyProjectName:      projectName,
		labels.LabelKeyComponentName:    componentName,
		labels.LabelKeyEnvironmentName:  environmentName,
	}

	// Build pod selectors
	podSelectors := map[string]string{
		labels.LabelKeyComponentUID:   componentUID,
		labels.LabelKeyEnvironmentUID: environmentUID,
		labels.LabelKeyProjectUID:     projectUID,
	}

	return pipelinecontext.MetadataContext{
		Name:            baseName,
		Namespace:       namespace,
		Labels:          standardLabels,
		Annotations:     map[string]string{},
		PodSelectors:    podSelectors,
		ComponentName:   componentName,
		ComponentUID:    componentUID,
		ProjectName:     projectName,
		ProjectUID:      projectUID,
		DataPlaneName:   dataPlaneName,
		DataPlaneUID:    dataPlaneUID,
		EnvironmentName: environmentName,
		EnvironmentUID:  environmentUID,
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
		for _, container := range workload.Spec.Containers {
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
	}

	// Collect from releaseBinding workload overrides if present
	if releaseBinding.Spec.WorkloadOverrides != nil {
		for _, container := range releaseBinding.Spec.WorkloadOverrides.Containers {
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
	}

	return secretRefs, nil
}

// reconcileRelease creates or updates the Release resource and sets appropriate status conditions.
func (r *Reconciler) reconcileRelease(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease, environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane, component *openchoreov1alpha1.Component, project *openchoreov1alpha1.Project) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build MetadataContext with computed names
	metadataContext := r.buildMetadataContext(componentRelease, component, project, dataPlane, environment, releaseBinding.Spec.Environment)

	// Prepare a render-time copy of the ReleaseBinding with defaults injected (e.g., alert notification channel).
	renderBinding := releaseBinding.DeepCopy()
	if err := r.applyDefaultNotificationChannel(ctx, renderBinding, componentRelease); err != nil {
		msg := fmt.Sprintf("Failed to apply default notification channel: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to apply default notification channel")
		return ctrl.Result{}, fmt.Errorf("failed to apply default notification channel: %w", err)
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
		ComponentType:    snapshotComponentType,
		Component:        snapshotComponent,
		Traits:           snapshotTraits,
		Workload:         snapshotWorkload,
		Environment:      environment,
		ReleaseBinding:   renderBinding,
		DataPlane:        dataPlane,
		SecretReferences: secretReferences,
		Metadata:         metadataContext,
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
	// Release name format: {component}-{environment}
	dataPlaneReleaseName := fmt.Sprintf("%s-%s", componentRelease.Spec.Owner.ComponentName, releaseBinding.Spec.Environment)
	dataPlaneRelease := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPlaneReleaseName,
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
			labels.LabelKeyOrganizationName: releaseBinding.Namespace,
			labels.LabelKeyProjectName:      releaseBinding.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:    releaseBinding.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName:  releaseBinding.Spec.Environment,
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

	// Create or update observability plane Release
	// Release name format: {component}-{environment}-observability
	observabilityReleaseName := fmt.Sprintf("%s-%s-observability", componentRelease.Spec.Owner.ComponentName, releaseBinding.Spec.Environment)
	observabilityRelease := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observabilityReleaseName,
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
			labels.LabelKeyOrganizationName: releaseBinding.Namespace,
			labels.LabelKeyProjectName:      releaseBinding.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:    releaseBinding.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName:  releaseBinding.Spec.Environment,
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
			msg := fmt.Sprintf("Release %q exists but is owned by another resource", observabilityRelease.Name)
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
				ReasonReleaseOwnershipConflict, msg)
			logger.Error(err, msg)
			return ctrl.Result{}, nil
		}

		// Transient errors
		msg := fmt.Sprintf("Failed to reconcile observability plane Release: %v", err)
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced,
			ReasonReleaseUpdateFailed, msg)
		logger.Error(err, "Failed to reconcile observability plane Release", "release", observabilityRelease.Name)
		return ctrl.Result{}, err
	}

	// Set ReleaseSynced condition based on operation results.
	// Prioritize dataplane operation result for readiness and requeue decisions.
	switch dpOp {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		msg := fmt.Sprintf("Dataplane Release %q %s with %d resources; observability Release %q %s with %d resources",
			dataPlaneRelease.Name, dpOp, len(dataPlaneReleaseResources),
			observabilityRelease.Name, opOp, len(observabilityPlaneReleaseResources))
		controller.MarkTrueCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseCreated, msg)
		logger.Info("Releases reconciled",
			"dataplaneReleaseOp", dpOp,
			"dataplaneRelease", dataPlaneRelease.Name,
			"dataplaneResourceCount", len(dataPlaneReleaseResources),
			"observabilityReleaseOp", opOp,
			"observabilityRelease", observabilityRelease.Name,
			"observabilityResourceCount", len(observabilityPlaneReleaseResources))
		return ctrl.Result{Requeue: true}, nil

	case controllerutil.OperationResultNone:
		msg := fmt.Sprintf("Dataplane Release %q is up to date; observability Release %q is %s",
			dataPlaneRelease.Name, observabilityRelease.Name, opOp)
		controller.MarkTrueCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseSynced, msg)
	}

	// Evaluate resource readiness from dataplane Release status (with component for workload type)
	if err := r.setResourcesReadyStatus(ctx, releaseBinding, dataPlaneRelease, component); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set resources ready status: %w", err)
	}

	// Set overall Ready condition based on ReleaseSynced and ResourcesReady
	r.setReadyCondition(releaseBinding)

	return ctrl.Result{}, nil
}

// Helper functions to build snapshot structures from ComponentRelease

func buildComponentFromRelease(componentRelease *openchoreov1alpha1.ComponentRelease) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentRelease.Spec.Owner.ComponentName,
			Namespace: componentRelease.Namespace,
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: componentRelease.Spec.Owner.ProjectName,
			},
			Parameters: componentRelease.Spec.ComponentProfile.Parameters,
			Traits:     componentRelease.Spec.ComponentProfile.Traits,
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

// applyDefaultNotificationChannel injects a default notificationChannel override for
// observability alert rule traits when none is provided for the environment.
func (r *Reconciler) applyDefaultNotificationChannel(
	ctx context.Context,
	rb *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease,
) error {
	// Identify observability-alertrule trait instances in the release.
	alertRuleInstances := make([]openchoreov1alpha1.ComponentTrait, 0)
	for _, trait := range componentRelease.Spec.ComponentProfile.Traits {
		if trait.Name == "observability-alertrule" {
			alertRuleInstances = append(alertRuleInstances, trait)
		}
	}

	if len(alertRuleInstances) == 0 {
		return nil
	}

	defaultChannel, err := r.getDefaultNotificationChannelName(ctx, rb.Namespace, rb.Spec.Environment)
	if err != nil {
		return err
	}

	if rb.Spec.TraitOverrides == nil {
		rb.Spec.TraitOverrides = make(map[string]runtime.RawExtension)
	}

	for _, trait := range alertRuleInstances {
		override, ok := rb.Spec.TraitOverrides[trait.InstanceName]
		if ok {
			// Check if notificationChannel already set
			var existing map[string]any
			if len(override.Raw) > 0 {
				if err := json.Unmarshal(override.Raw, &existing); err != nil {
					return fmt.Errorf("failed to unmarshal trait override for %s: %w", trait.InstanceName, err)
				}
				if val, ok := existing["notificationChannel"]; ok && fmt.Sprintf("%v", val) != "" {
					continue
				}
				// inject and re-marshal
				existing["notificationChannel"] = defaultChannel
				updated, err := json.Marshal(existing)
				if err != nil {
					return fmt.Errorf("failed to marshal trait override for %s: %w", trait.InstanceName, err)
				}
				rb.Spec.TraitOverrides[trait.InstanceName] = runtime.RawExtension{Raw: updated}
				continue
			}
		}

		// No override or empty override; create one with default notificationChannel
		payload := map[string]any{"notificationChannel": defaultChannel}
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal default notificationChannel override for %s: %w", trait.InstanceName, err)
		}
		rb.Spec.TraitOverrides[trait.InstanceName] = runtime.RawExtension{Raw: raw}
	}

	return nil
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
