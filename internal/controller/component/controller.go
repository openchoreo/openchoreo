// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a Component object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=traits,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentworkflowruns,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=deploymentpipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=gitcommitrequests,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Component instance
	comp := &openchoreov1alpha1.Component{}
	if err := r.Get(ctx, req.NamespacedName, comp); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Component resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Component")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := comp.DeepCopy()

	// Handle deletion - run finalizer logic
	if !comp.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing component")
		return r.finalize(ctx, old, comp)
	}

	// Ensure finalizer is added
	if finalizerAdded, err := r.ensureFinalizer(ctx, comp); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Detect mode based on which fields are set
	// Note: API-level validation ensures at least one of type or componentType is set
	if comp.Spec.ComponentType != "" {
		// New ComponentType mode
		logger.Info("Reconciling Component with ComponentType mode",
			"component", comp.Name,
			"componentType", comp.Spec.ComponentType)
		return r.reconcileWithComponentType(ctx, comp)
	}

	// Legacy mode - no action needed for now
	logger.Info("Component using legacy mode - no snapshot management",
		"component", comp.Name,
		"type", comp.Spec.Type)
	return ctrl.Result{}, nil
}

// reconcileWithComponentType handles components using ComponentTypes
func (r *Reconciler) reconcileWithComponentType(ctx context.Context, comp *openchoreov1alpha1.Component) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Keep a copy for comparison
	old := comp.DeepCopy()

	// Deferred status update
	defer func() {
		// Update observed generation
		comp.Status.ObservedGeneration = comp.Generation

		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, comp.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, comp); err != nil {
			logger.Error(err, "Failed to update Component status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Validate and fetch ComponentType
	ct, err := r.validateAndFetchComponentType(ctx, comp)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ct == nil {
		// Validation error, condition already set
		return ctrl.Result{}, nil
	}

	// Validate ComponentWorkflow (if specified)
	componentWorkflow, err := r.validateComponentWorkflow(ctx, comp, ct)
	if err != nil {
		return ctrl.Result{}, err
	}
	if componentWorkflow == nil && comp.Spec.Workflow != nil {
		// Validation failed, condition already set
		return ctrl.Result{}, nil
	}

	// Validate and fetch Workload
	workload, err := r.validateAndFetchWorkload(ctx, comp)
	if err != nil {
		return ctrl.Result{}, err
	}
	if workload == nil {
		// Validation error, condition already set
		return ctrl.Result{}, nil
	}

	// Validate traits
	if !r.areValidTraits(ctx, comp, ct) {
		// Validation failed, condition already set
		return ctrl.Result{}, nil
	}

	// Fetch all referenced Traits: both embedded (from ComponentType) and component-level
	traits, err := r.fetchAllTraits(ctx, ct, comp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := "One or more Traits not found"
			var traitErr *traitFetchError
			if errors.As(err, &traitErr) {
				msg = fmt.Sprintf("Trait %q not found", traitErr.traitName)
			}
			controller.MarkFalseCondition(comp, ConditionReady, ReasonTraitNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch Traits")
		return ctrl.Result{}, err
	}

	// Validate and fetch deployment pipeline
	firstEnv, err := r.validateAndFetchDeploymentPipeline(ctx, comp)
	if err != nil {
		return ctrl.Result{}, err
	}
	if firstEnv == "" {
		// Validation error, condition already set
		return ctrl.Result{}, nil
	}

	// Handle autoDeploy if enabled
	if comp.Spec.AutoDeploy {
		if err := r.handleAutoDeploy(ctx, comp, ct, workload, traits, firstEnv); err != nil {
			msg := fmt.Sprintf("Failed to handle autoDeploy: %v", err)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonAutoDeployFailed, msg)
			logger.Error(err, "Failed to handle autoDeploy")
			return ctrl.Result{}, err
		}
	}

	// Success - mark as ready
	if comp.Spec.AutoDeploy {
		// AutoDeploy enabled - ComponentRelease and ReleaseBinding were handled
		releaseName := comp.Status.LatestRelease.Name
		bindingName := fmt.Sprintf("%s-%s", comp.Name, firstEnv)
		msg := fmt.Sprintf("ComponentRelease %q and ReleaseBinding %q successfully managed for environment %q",
			releaseName, bindingName, firstEnv)
		controller.MarkTrueCondition(comp, ConditionReady, ReasonComponentReleaseReady, msg)
		logger.Info("Successfully reconciled Component with autoDeploy enabled",
			"component", comp.Name,
			"release", releaseName,
			"binding", bindingName,
			"environment", firstEnv)
	} else {
		// AutoDeploy disabled - only validation was performed
		msg := "Component validated successfully"
		controller.MarkTrueCondition(comp, ConditionReady, ReasonReconciled, msg)
		logger.Info("Successfully reconciled Component with autoDeploy disabled",
			"component", comp.Name)
	}

	return ctrl.Result{}, nil
}

// validateAndFetchComponentType parses, fetches, and validates the ComponentType.
// Returns the ComponentType on success, or nil with no error if validation failed (condition already set).
func (r *Reconciler) validateAndFetchComponentType(ctx context.Context, comp *openchoreov1alpha1.Component) (*openchoreov1alpha1.ComponentType, error) {
	logger := log.FromContext(ctx)

	// Parse componentType: {workloadType}/{componentTypeName}
	workloadType, ctName, err := parseComponentType(comp.Spec.ComponentType)
	if err != nil {
		msg := fmt.Sprintf("Invalid componentType format: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(err, "Failed to parse componentType")
		return nil, nil
	}

	// Fetch ComponentType (in the same namespace as the Component)
	ct := &openchoreov1alpha1.ComponentType{}
	if err := r.Get(ctx, types.NamespacedName{Name: ctName, Namespace: comp.Namespace}, ct); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentType %q not found", ctName)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentTypeNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return nil, nil
		}
		logger.Error(err, "Failed to fetch ComponentType", "name", ctName)
		return nil, err
	}

	// Verify workloadType matches
	if ct.Spec.WorkloadType != workloadType {
		msg := fmt.Sprintf("WorkloadType mismatch: component specifies %s but ComponentType has %s",
			workloadType, ct.Spec.WorkloadType)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(fmt.Errorf("%s", msg), "WorkloadType mismatch")
		return nil, nil
	}

	return ct, nil
}

// validateAndFetchWorkload fetches and validates the Workload for the component.
// Returns the Workload on success, or nil with no error if validation failed (condition already set).
func (r *Reconciler) validateAndFetchWorkload(ctx context.Context, comp *openchoreov1alpha1.Component) (*openchoreov1alpha1.Workload, error) {
	logger := log.FromContext(ctx)

	// Fetch Workload by owner reference (supports any naming convention)
	ownerKey := fmt.Sprintf("%s/%s", comp.Spec.Owner.ProjectName, comp.Name)
	var workloadList openchoreov1alpha1.WorkloadList
	err := r.List(ctx, &workloadList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{workloadOwnerIndex: ownerKey})
	if err != nil {
		logger.Error(err, "Failed to list Workloads by owner")
		return nil, err
	}

	if len(workloadList.Items) == 0 {
		msg := fmt.Sprintf("Workload for component %q not found, waiting for workload to be created", comp.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonWorkloadNotFound, msg)
		logger.Info(msg, "component", comp.Name, "ownerKey", ownerKey)
		return nil, nil
	}

	if len(workloadList.Items) > 1 {
		msg := fmt.Sprintf("Multiple Workloads found for component %q (expected exactly 1)", comp.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(fmt.Errorf("multiple workloads found"), msg, "count", len(workloadList.Items))
		return nil, nil
	}

	return &workloadList.Items[0], nil
}

// areValidTraits validates trait configuration and instance name uniqueness.
// Returns true if validation passes, false if it fails (with condition set).
func (r *Reconciler) areValidTraits(ctx context.Context, comp *openchoreov1alpha1.Component, ct *openchoreov1alpha1.ComponentType) bool {
	logger := log.FromContext(ctx)

	// Validate allowedTraits: ensure developer's traits are in the allowed list
	if len(ct.Spec.AllowedTraits) > 0 {
		allowedSet := make(map[string]bool, len(ct.Spec.AllowedTraits))
		for _, name := range ct.Spec.AllowedTraits {
			allowedSet[name] = true
		}
		var disallowedTraits []string
		for _, trait := range comp.Spec.Traits {
			if !allowedSet[trait.Name] {
				disallowedTraits = append(disallowedTraits, trait.Name)
			}
		}
		if len(disallowedTraits) > 0 {
			msg := fmt.Sprintf("Traits %v are not allowed by ComponentType %q; allowed traits: %v",
				disallowedTraits, ct.Name, ct.Spec.AllowedTraits)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
			logger.Info(msg, "component", comp.Name)
			return false
		}
	} else {
		// If allowedTraits is empty, no traits are allowed
		if len(comp.Spec.Traits) > 0 {
			msg := fmt.Sprintf("No traits are allowed by ComponentType %q, but component has %d trait(s)",
				ct.Name, len(comp.Spec.Traits))
			controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
			logger.Info(msg, "component", comp.Name)
			return false
		}
	}

	// Validate instance name uniqueness across embedded and component-level traits
	if len(ct.Spec.Traits) > 0 && len(comp.Spec.Traits) > 0 {
		embeddedNames := make(map[string]bool, len(ct.Spec.Traits))
		for _, et := range ct.Spec.Traits {
			embeddedNames[et.InstanceName] = true
		}
		var collidingNames []string
		for _, t := range comp.Spec.Traits {
			if embeddedNames[t.InstanceName] {
				collidingNames = append(collidingNames, t.InstanceName)
			}
		}
		if len(collidingNames) > 0 {
			msg := fmt.Sprintf("Trait instance names %v collide with embedded traits in ComponentType %q",
				collidingNames, ct.Name)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
			logger.Info(msg, "component", comp.Name)
			return false
		}
	}

	return true
}

// validateComponentWorkflow validates that the referenced ComponentWorkflow exists
// and is in the allowedWorkflows list of the ComponentType.
// Returns the ComponentWorkflow on success, or nil with no error if validation failed
// (condition already set).
func (r *Reconciler) validateComponentWorkflow(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ct *openchoreov1alpha1.ComponentType,
) (*openchoreov1alpha1.ComponentWorkflow, error) {
	logger := log.FromContext(ctx)

	// If no workflow is specified, validation passes (workflows are optional)
	if comp.Spec.Workflow == nil {
		return nil, nil
	}

	workflowName := comp.Spec.Workflow.Name

	// Performance optimization: Check allowedWorkflows list first
	// This avoids fetching the ComponentWorkflow if it's not allowed
	if len(ct.Spec.AllowedWorkflows) > 0 {
		allowedSet := make(map[string]bool, len(ct.Spec.AllowedWorkflows))
		for _, name := range ct.Spec.AllowedWorkflows {
			allowedSet[name] = true
		}

		if !allowedSet[workflowName] {
			msg := fmt.Sprintf("ComponentWorkflow %q is not allowed by ComponentType %q; allowed workflows: %v",
				workflowName, ct.Name, ct.Spec.AllowedWorkflows)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentWorkflowNotAllowed, msg)
			logger.Info(msg, "component", comp.Name)
			return nil, nil
		}
	} else {
		// If allowedWorkflows is empty, no workflows are allowed
		msg := fmt.Sprintf("No ComponentWorkflows are allowed by ComponentType %q, but component specifies workflow %q",
			ct.Name, workflowName)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentWorkflowNotAllowed, msg)
		logger.Info(msg, "component", comp.Name)
		return nil, nil
	}

	// Now check if the ComponentWorkflow actually exists
	workflow := &openchoreov1alpha1.ComponentWorkflow{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflowName,
		Namespace: comp.Namespace,
	}, workflow); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentWorkflow %q not found", workflowName)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentWorkflowNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return nil, nil
		}
		logger.Error(err, "Failed to fetch ComponentWorkflow", "name", workflowName)
		return nil, err
	}

	return workflow, nil
}

// validateAndFetchDeploymentPipeline fetches and validates the Project, DeploymentPipeline, and finds the root environment.
// Returns the root environment name on success, or empty string with no error if validation failed (condition already set).
func (r *Reconciler) validateAndFetchDeploymentPipeline(ctx context.Context, comp *openchoreov1alpha1.Component) (string, error) {
	logger := log.FromContext(ctx)

	// Get the Project to find the DeploymentPipeline reference
	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      comp.Spec.Owner.ProjectName,
		Namespace: comp.Namespace,
	}, project); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Project %q not found", comp.Spec.Owner.ProjectName)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonProjectNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return "", nil
		}
		logger.Error(err, "Failed to get Project")
		return "", err
	}

	// Validate that the project has a deployment pipeline reference
	if project.Spec.DeploymentPipelineRef == "" {
		msg := fmt.Sprintf("Project %q has empty deploymentPipelineRef", project.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Info(msg, "component", comp.Name)
		return "", nil
	}

	// Get the DeploymentPipeline
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      project.Spec.DeploymentPipelineRef,
		Namespace: project.Namespace,
	}, pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("DeploymentPipeline %q not found", project.Spec.DeploymentPipelineRef)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonDeploymentPipelineNotFound, msg)
			logger.Info(msg, "component", comp.Name, "project", project.Name)
			return "", nil
		}
		logger.Error(err, "Failed to get DeploymentPipeline")
		return "", err
	}

	// Find the root environment using pure function
	firstEnv, err := findRootEnvironment(pipeline)
	if err != nil {
		// Configuration errors are non-retryable
		msg := fmt.Sprintf("Invalid deployment pipeline configuration: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Info(msg, "component", comp.Name, "pipeline", pipeline.Name)
		return "", nil
	}

	return firstEnv, nil
}

// parseComponentType parses the componentType format: {workloadType}/{componentTypeName}
func parseComponentType(componentType string) (workloadType string, ctName string, err error) {
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid componentType format: expected {workloadType}/{name}, got %s", componentType)
	}
	return parts[0], parts[1], nil
}

// traitFetchError wraps an error from fetching an trait with the trait name
type traitFetchError struct {
	traitName string
	err       error
}

func (e *traitFetchError) Error() string {
	return fmt.Sprintf("failed to fetch trait %q: %v", e.traitName, e.err)
}

func (e *traitFetchError) Unwrap() error {
	return e.err
}

// fetchAllTraits fetches all unique Trait resources referenced by both embedded traits
// (from ComponentType) and component-level traits, deduplicating by trait name.
func (r *Reconciler) fetchAllTraits(ctx context.Context, ct *openchoreov1alpha1.ComponentType, comp *openchoreov1alpha1.Component) ([]openchoreov1alpha1.Trait, error) {
	seen := make(map[string]bool)
	traits := make([]openchoreov1alpha1.Trait, 0, len(ct.Spec.Traits)+len(comp.Spec.Traits))

	// Collect from embedded traits
	for _, et := range ct.Spec.Traits {
		if seen[et.Name] {
			continue
		}
		seen[et.Name] = true
		trait := &openchoreov1alpha1.Trait{}
		if err := r.Get(ctx, types.NamespacedName{Name: et.Name, Namespace: comp.Namespace}, trait); err != nil {
			return nil, &traitFetchError{traitName: et.Name, err: err}
		}
		traits = append(traits, *trait)
	}

	// Collect from component-level traits
	for _, ref := range comp.Spec.Traits {
		if seen[ref.Name] {
			continue
		}
		seen[ref.Name] = true
		trait := &openchoreov1alpha1.Trait{}
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: comp.Namespace}, trait); err != nil {
			return nil, &traitFetchError{traitName: ref.Name, err: err}
		}
		traits = append(traits, *trait)
	}

	return traits, nil
}

// findRootEnvironment finds the root environment in a deployment pipeline.
// The root environment is the source environment that never appears as a target,
// representing the initial environment where components are first deployed.
// This is a pure function for easier testing.
func findRootEnvironment(pipeline *openchoreov1alpha1.DeploymentPipeline) (string, error) {
	if len(pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline %s has no promotion paths defined", pipeline.Name)
	}

	// Build a set of all target environments
	targets := make(map[string]bool)
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find source environment that's never a target (the root)
	var rootEnv string
	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef == "" {
			continue
		}
		if !targets[path.SourceEnvironmentRef] {
			rootEnv = path.SourceEnvironmentRef
			break
		}
	}

	if rootEnv == "" {
		return "", fmt.Errorf("deployment pipeline %s has no root environment (all sources are also targets)", pipeline.Name)
	}

	return rootEnv, nil
}

// handleAutoDeploy handles automatic deployment when autoDeploy is enabled.
// It computes the hash of the current release spec and creates/updates ComponentRelease
// and ReleaseBinding if the hash has changed.
func (r *Reconciler) handleAutoDeploy(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ct *openchoreov1alpha1.ComponentType,
	workload *openchoreov1alpha1.Workload,
	traits []openchoreov1alpha1.Trait,
	firstEnv string,
) error {
	logger := log.FromContext(ctx)

	// ReleaseBinding name to create releaseBinding if not exits
	bindingName := fmt.Sprintf("%s-%s", comp.Name, firstEnv)

	releaseSpec, err := BuildReleaseSpec(ct, traits, comp, workload)
	if err != nil {
		return fmt.Errorf("failed to build ReleaseSpec: %w", err)
	}

	currentHash := ComputeReleaseHash(releaseSpec, nil)

	// Check if hash exists in status.LatestRelease and it hasn't changed
	if comp.Status.LatestRelease != nil && comp.Status.LatestRelease.ReleaseHash == currentHash {
		// Hash matches, verify the ComponentRelease exists and recreate if needed
		releaseName := comp.Status.LatestRelease.Name
		exists, err := r.ensureComponentRelease(ctx, comp, ct, workload, traits, releaseName, currentHash)
		if err != nil {
			return err
		}
		if exists {
			// ComponentRelease already existed, nothing more to do
			logger.Info("ComponentRelease hash unchanged and release exists",
				"component", comp.Name,
				"hash", currentHash,
				"release", releaseName)
			return r.ensureReleaseBinding(ctx, comp, releaseName, firstEnv, bindingName)
		}
		// ComponentRelease was recreated, continue to update status and binding
		logger.Info("ComponentRelease recreated after being missing",
			"component", comp.Name,
			"hash", currentHash,
			"release", releaseName)
	} else {
		// Hash is different, create new ComponentRelease
		logger.Info("ComponentRelease hash diff detected, creating new release",
			"component", comp.Name,
			"oldHash", func() string {
				if comp.Status.LatestRelease != nil {
					return comp.Status.LatestRelease.ReleaseHash
				}
				return "none"
			}(),
			"newHash", currentHash)

		releaseName := fmt.Sprintf("%s-%s", comp.Name, currentHash)
		if _, err := r.ensureComponentRelease(ctx, comp, ct, workload, traits, releaseName, currentHash); err != nil {
			return err
		}
	}

	// Generate release name for status update
	releaseName := fmt.Sprintf("%s-%s", comp.Name, currentHash)

	// Update status.LatestRelease
	comp.Status.LatestRelease = &openchoreov1alpha1.LatestRelease{
		Name:        releaseName,
		ReleaseHash: currentHash,
	}

	return r.ensureReleaseBinding(ctx, comp, releaseName, firstEnv, bindingName)
}

// ensureComponentRelease ensures a ComponentRelease with the given name exists.
// Returns (true, nil) if the release already existed.
// Returns (false, nil) if the release was created.
// Returns (false, error) if there was an error checking or creating the release.
func (r *Reconciler) ensureComponentRelease(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ct *openchoreov1alpha1.ComponentType,
	workload *openchoreov1alpha1.Workload,
	traits []openchoreov1alpha1.Trait,
	releaseName string,
	currentHash string,
) (bool, error) {
	logger := log.FromContext(ctx)

	// Check if ComponentRelease already exists
	existingRelease := &openchoreov1alpha1.ComponentRelease{}
	err := r.Get(ctx, types.NamespacedName{Name: releaseName, Namespace: comp.Namespace}, existingRelease)
	if err == nil {
		// ComponentRelease exists - validate its hash matches expectations
		// Build ReleaseSpec from the existing ComponentRelease to verify integrity
		existingSpec := &ReleaseSpec{
			ComponentType:    existingRelease.Spec.ComponentType,
			Traits:           existingRelease.Spec.Traits,
			ComponentProfile: existingRelease.Spec.ComponentProfile,
			Workload:         existingRelease.Spec.Workload,
		}
		existingHash := ComputeReleaseHash(existingSpec, nil)

		if existingHash != currentHash {
			// Hash mismatch - ComponentRelease was modified or corrupted, restore correct content
			logger.Info("ComponentRelease hash mismatch detected, updating to restore correct content",
				"name", releaseName,
				"expectedHash", currentHash,
				"actualHash", existingHash,
				"reason", "possible manual modification or data corruption")

			// Update the ComponentRelease with the correct content
			existingRelease.Spec = openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   comp.Spec.Owner.ProjectName,
					ComponentName: comp.Name,
				},
				ComponentType:    ct.Spec,
				Traits:           buildTraitsMap(traits),
				ComponentProfile: buildComponentProfile(comp),
				Workload:         workload.Spec.WorkloadTemplateSpec,
			}

			if err := r.Update(ctx, existingRelease); err != nil {
				return false, fmt.Errorf("failed to update corrupted ComponentRelease %q: %w", releaseName, err)
			}

			logger.Info("ComponentRelease updated to restore correct content",
				"name", releaseName,
				"hash", currentHash)
			// Return false to indicate we did work (updated the release)
			return false, nil
		}

		// Hash matches - ComponentRelease is valid
		logger.Info("ComponentRelease exists and hash is valid",
			"name", releaseName,
			"hash", currentHash)
		return true, nil
	}
	if !apierrors.IsNotFound(err) {
		// Unexpected error
		return false, fmt.Errorf("failed to get ComponentRelease %q: %w", releaseName, err)
	}

	// ComponentRelease doesn't exist, create it
	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      releaseName,
			Namespace: comp.Namespace,
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   comp.Spec.Owner.ProjectName,
				ComponentName: comp.Name,
			},
			ComponentType:    ct.Spec,
			Traits:           buildTraitsMap(traits),
			ComponentProfile: buildComponentProfile(comp),
			Workload:         workload.Spec.WorkloadTemplateSpec,
		},
	}

	if err := r.Create(ctx, componentRelease); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("failed to create ComponentRelease: %w", err)
		}
		// Already exists - this is fine, likely a race condition
		logger.Info("ComponentRelease already exists (race condition)", "name", releaseName)
		return true, nil
	}

	logger.Info("Created ComponentRelease", "name", releaseName, "hash", currentHash)
	return false, nil
}

// ensureReleaseBinding ensures a ReleaseBinding exists for the given environment and component.
// If the ReleaseBinding doesn't exist, it creates one.
// If it exists, it updates the release name if different.
// Returns an error if multiple ReleaseBindings are found for the same environment.
// bindingName is the name of the ReleaseBinding to create if it doesn't exist.
func (r *Reconciler) ensureReleaseBinding(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	releaseName string,
	firstEnv string,
	bindingName string,
) error {
	logger := log.FromContext(ctx)

	envKey := makeReleaseBindingIndexKey(comp.Spec.Owner.ProjectName, comp.Name, firstEnv)
	releaseBindingList := openchoreov1alpha1.ReleaseBindingList{}
	err := r.List(ctx, &releaseBindingList, client.InNamespace(comp.Namespace),
		client.MatchingFields{releaseBindingIndex: envKey})
	if err != nil {
		return fmt.Errorf("failed to list ReleaseBinding: %w", err)
	}

	if len(releaseBindingList.Items) == 0 {
		// ReleaseBinding doesn't exist, create it
		releaseBinding := &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: comp.Namespace,
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   comp.Spec.Owner.ProjectName,
					ComponentName: comp.Name,
				},
				ReleaseName: releaseName,
				Environment: firstEnv,
				// No overrides for initial auto-deploy
			},
		}

		if err := r.Create(ctx, releaseBinding); err != nil {
			return fmt.Errorf("failed to create ReleaseBinding: %w", err)
		}

		logger.Info("Created ReleaseBinding", "binding", bindingName, "release", releaseName,
			"environment", firstEnv)
		return nil
	}

	if len(releaseBindingList.Items) > 1 {
		return fmt.Errorf("found multiple ReleaseBinding objects for environment %q", firstEnv)
	}

	releaseBinding := releaseBindingList.Items[0]

	// ReleaseBinding exists, patch the release name if different
	if releaseBinding.Spec.ReleaseName != releaseName {
		releaseBinding.Spec.ReleaseName = releaseName

		if err := r.Update(ctx, &releaseBinding); err != nil {
			return fmt.Errorf("failed to update ReleaseBinding: %w", err)
		}

		logger.Info("Updated ReleaseBinding with new release",
			"binding", bindingName,
			"release", releaseName,
			"environment", firstEnv)
	} else {
		logger.Info("ReleaseBinding already references current release",
			"binding", bindingName,
			"release", releaseName)
	}

	return nil
}

// buildTraitsMap converts a slice of Trait resources to a map of trait name to TraitSpec
func buildTraitsMap(traits []openchoreov1alpha1.Trait) map[string]openchoreov1alpha1.TraitSpec {
	if len(traits) == 0 {
		return nil
	}

	traitsMap := make(map[string]openchoreov1alpha1.TraitSpec, len(traits))
	for _, trait := range traits {
		traitsMap[trait.Name] = trait.Spec
	}
	return traitsMap
}

// buildComponentProfile extracts the ComponentProfile from the Component.
// Returns nil if the component has no parameters and no traits.
func buildComponentProfile(comp *openchoreov1alpha1.Component) *openchoreov1alpha1.ComponentProfile {
	if comp.Spec.Parameters == nil && len(comp.Spec.Traits) == 0 {
		return nil
	}
	return &openchoreov1alpha1.ComponentProfile{
		Parameters: comp.Spec.Parameters,
		Traits:     comp.Spec.Traits,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	// Set up field indexes for efficient lookups
	if err := r.setupComponentTypeRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup component type reference index: %w", err)
	}

	if err := r.setupTraitsRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup traits reference index: %w", err)
	}

	if err := r.setupWorkflowRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup workflow reference index: %w", err)
	}

	if err := r.setupWorkloadOwnerIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup workload owner index: %w", err)
	}

	if err := r.setupReleaseBindingIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup release binding index: %w", err)
	}

	if err := r.setupComponentReleaseOwnerIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup component release owner index: %w", err)
	}

	if err := r.setupComponentWorkflowRunOwnerIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup component workflow run owner index: %w", err)
	}

	// Note: The following shared indexes are set up in controller.SetupSharedIndexes (called from main.go):
	// - ReleaseBinding owner index (used by Component and ReleaseBinding controllers)
	// - Component owner project index (used by Project and Component controllers)
	// - Project deploymentPipelineRef index (used by Component controller)

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Component{}).
		Watches(&openchoreov1alpha1.ComponentRelease{},
			handler.EnqueueRequestsFromMapFunc(r.findComponentsForComponentRelease)).
		Watches(&openchoreov1alpha1.ReleaseBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findComponentsForReleaseBinding)).
		Watches(&openchoreov1alpha1.ComponentWorkflowRun{},
			handler.EnqueueRequestsFromMapFunc(r.findComponentsForComponentWorkflowRun)).
		Watches(&openchoreov1alpha1.ComponentType{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForComponentType)).
		Watches(&openchoreov1alpha1.Trait{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsUsingTrait)).
		Watches(&openchoreov1alpha1.ComponentWorkflow{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForComponentWorkflow)).
		Watches(&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForWorkload)).
		Watches(&openchoreov1alpha1.Project{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForProject)).
		Watches(&openchoreov1alpha1.DeploymentPipeline{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForDeploymentPipeline)).
		Named("component").
		Complete(r)
}
