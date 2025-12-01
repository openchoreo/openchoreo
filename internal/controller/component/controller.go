// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
)

// Reconciler reconciles a Component object
type Reconciler struct {
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
	// In GitOps mode, the controller will not create or update resources directly in the cluster,
	// but will instead generate the necessary manifests and creates GitCommitRequests to update the Git repository.
	IsGitOpsMode   bool
	Scheme         *runtime.Scheme
	GitProvider    git.Provider
	WebhookBaseURL string
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=gitrepositorywebhooks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=gitrepositorywebhooks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;delete

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

	// Handle autoBuild webhook registration
	if err := r.reconcileWebhook(ctx, comp); err != nil {
		logger.Error(err, "Failed to reconcile webhook", "component", comp.Name)
		// Don't fail the entire reconciliation, just log the error
		// The webhook registration will be retried in the next reconciliation
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

	// Parse componentType: {workloadType}/{componentTypeName}
	workloadType, ctName, err := parseComponentType(comp.Spec.ComponentType)
	if err != nil {
		msg := fmt.Sprintf("Invalid componentType format: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(err, "Failed to parse componentType")
		return ctrl.Result{}, nil
	}

	// Fetch ComponentType (in the same namespace as the Component)
	ct := &openchoreov1alpha1.ComponentType{}
	if err := r.Get(ctx, types.NamespacedName{Name: ctName, Namespace: comp.Namespace}, ct); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentType %q not found", ctName)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentTypeNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch ComponentType", "name", ctName)
		return ctrl.Result{}, err
	}

	// Verify workloadType matches
	if ct.Spec.WorkloadType != workloadType {
		msg := fmt.Sprintf("WorkloadType mismatch: component specifies %s but ComponentType has %s",
			workloadType, ct.Spec.WorkloadType)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(fmt.Errorf("%s", msg), "WorkloadType mismatch")
		return ctrl.Result{}, nil
	}

	// Fetch Workload by owner reference (supports any naming convention)
	ownerKey := fmt.Sprintf("%s/%s", comp.Spec.Owner.ProjectName, comp.Name)
	var workloadList openchoreov1alpha1.WorkloadList
	err = r.List(ctx, &workloadList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{workloadOwnerIndex: ownerKey})
	if err != nil {
		logger.Error(err, "Failed to list Workloads by owner")
		return ctrl.Result{}, err
	}

	if len(workloadList.Items) == 0 {
		msg := fmt.Sprintf("Workload for component %q not found, waiting for workload to be created", comp.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonWorkloadNotFound, msg)
		logger.Info(msg, "component", comp.Name, "ownerKey", ownerKey)
		return ctrl.Result{}, nil
	}

	if len(workloadList.Items) > 1 {
		msg := fmt.Sprintf("Multiple Workloads found for component %q (expected exactly 1)", comp.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(fmt.Errorf("multiple workloads found"), msg, "count", len(workloadList.Items))
		return ctrl.Result{}, nil
	}

	workload := &workloadList.Items[0]

	// Fetch all referenced Traits (in the same namespace as the Component)
	traits, err := r.fetchTraits(ctx, comp.Spec.Traits, comp.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Extract trait name from custom error type
			var traitErr *traitFetchError
			if errors.As(err, &traitErr) {
				msg := fmt.Sprintf("Trait %q not found", traitErr.traitName)
				controller.MarkFalseCondition(comp, ConditionReady, ReasonTraitNotFound, msg)
				logger.Info(msg, "component", comp.Name)
				return ctrl.Result{}, nil
			}
			// Fallback if error type doesn't match
			msg := "One or more Traits not found"
			controller.MarkFalseCondition(comp, ConditionReady, ReasonTraitNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch Traits")
		return ctrl.Result{}, err
	}

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
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Project")
		return ctrl.Result{}, err
	}

	// Validate that the project has a deployment pipeline reference
	if project.Spec.DeploymentPipelineRef == "" {
		msg := fmt.Sprintf("Project %q has empty deploymentPipelineRef", project.Name)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Info(msg, "component", comp.Name)
		return ctrl.Result{}, nil
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
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get DeploymentPipeline")
		return ctrl.Result{}, err
	}

	// Find the root environment using pure function
	firstEnv, err := findRootEnvironment(pipeline)
	if err != nil {
		// Configuration errors are non-retryable
		msg := fmt.Sprintf("Invalid deployment pipeline configuration: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Info(msg, "component", comp.Name, "pipeline", pipeline.Name)
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

// fetchTraits fetches all Trait resources referenced by the component
func (r *Reconciler) fetchTraits(ctx context.Context, traitRefs []openchoreov1alpha1.ComponentTrait, namespace string) ([]openchoreov1alpha1.Trait, error) {
	traits := make([]openchoreov1alpha1.Trait, 0, len(traitRefs))

	for _, ref := range traitRefs {
		trait := &openchoreov1alpha1.Trait{}
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, trait); err != nil {
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

// buildComponentProfile extracts the ComponentProfile from the Component
func buildComponentProfile(comp *openchoreov1alpha1.Component) openchoreov1alpha1.ComponentProfile {
	return openchoreov1alpha1.ComponentProfile{
		Parameters: comp.Spec.Parameters,
		Traits:     comp.Spec.Traits,
	}
}

// reconcileWebhook handles webhook registration/deregistration based on autoBuild flag
func (r *Reconciler) reconcileWebhook(ctx context.Context, comp *openchoreov1alpha1.Component) error {
	logger := log.FromContext(ctx)

	// Skip if GitProvider is not configured
	if r.GitProvider == nil {
		return nil
	}

	autoBuildEnabled := comp.Spec.AutoBuild != nil && *comp.Spec.AutoBuild
	webhookRegistered := comp.Status.WebhookRegistered != nil && *comp.Status.WebhookRegistered

	if autoBuildEnabled && !webhookRegistered {
		// AutoBuild enabled but webhook not registered - register it
		logger.Info("AutoBuild enabled, registering webhook", "component", comp.Name)
		return r.registerWebhook(ctx, comp)
	} else if !autoBuildEnabled && webhookRegistered {
		// AutoBuild disabled but webhook is registered - deregister it
		logger.Info("AutoBuild disabled, deregistering webhook", "component", comp.Name)
		return r.deregisterWebhook(ctx, comp)
	}

	// No action needed
	return nil
}

// registerWebhook registers a webhook for the component's repository
func (r *Reconciler) registerWebhook(ctx context.Context, comp *openchoreov1alpha1.Component) error {
	logger := log.FromContext(ctx)

	// Extract repository URL from component workflow schema
	repoURL, err := extractRepoURLFromComponent(comp)
	if err != nil {
		logger.Error(err, "Failed to extract repository URL from component")
		return fmt.Errorf("failed to extract repository URL: %w", err)
	}

	// Normalize repository URL
	normalizedRepoURL := normalizeRepoURL(repoURL)

	// Generate GitRepositoryWebhook resource name
	webhookResourceName := generateWebhookResourceName(normalizedRepoURL)

	// Check if GitRepositoryWebhook already exists for this repository
	gitWebhook := &openchoreov1alpha1.GitRepositoryWebhook{}
	gitWebhookKey := client.ObjectKey{
		Name: webhookResourceName,
	}
	err = r.Get(ctx, gitWebhookKey, gitWebhook)

	if err == nil {
		// GitRepositoryWebhook exists, add this component to references
		logger.Info("GitRepositoryWebhook already exists, adding component reference",
			"webhook", webhookResourceName, "component", comp.Name)

		// Check if component is already in references
		componentRef := openchoreov1alpha1.ComponentReference{
			Namespace:   comp.Namespace,
			Name:        comp.Name,
			OrgName:     comp.Namespace, // Assuming namespace is the org name
			ProjectName: comp.Spec.Owner.ProjectName,
		}

		alreadyExists := false
		for _, ref := range gitWebhook.Spec.ComponentReferences {
			if ref.Namespace == componentRef.Namespace && ref.Name == componentRef.Name {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists {
			gitWebhook.Spec.ComponentReferences = append(gitWebhook.Spec.ComponentReferences, componentRef)
			if err := r.Update(ctx, gitWebhook); err != nil {
				logger.Error(err, "Failed to update GitRepositoryWebhook")
				return fmt.Errorf("failed to update GitRepositoryWebhook: %w", err)
			}

			// Update status
			gitWebhook.Status.ReferenceCount = len(gitWebhook.Spec.ComponentReferences)
			if err := r.Status().Update(ctx, gitWebhook); err != nil {
				logger.Error(err, "Failed to update GitRepositoryWebhook status")
			}
		}

		// Update component status
		registered := true
		comp.Status.WebhookRegistered = &registered
		comp.Status.WebhookID = gitWebhook.Spec.WebhookID
		comp.Status.WebhookProvider = gitWebhook.Spec.Provider

		if err := r.Status().Update(ctx, comp); err != nil {
			logger.Error(err, "Failed to update component status")
			return fmt.Errorf("failed to update component status: %w", err)
		}

		logger.Info("Component added to existing webhook", "component", comp.Name, "webhookID", gitWebhook.Spec.WebhookID)
		return nil

	} else if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to get GitRepositoryWebhook")
		return fmt.Errorf("failed to get GitRepositoryWebhook: %w", err)
	}

	// GitRepositoryWebhook doesn't exist, create webhook with git provider
	logger.Info("Creating new webhook for repository", "repo", normalizedRepoURL)

	// Generate webhook URL
	webhookURL := fmt.Sprintf("%s/api/v1/webhooks/github", r.WebhookBaseURL)

	// Generate a cryptographically secure random webhook secret
	webhookSecret, err := generateWebhookSecret(32)
	if err != nil {
		logger.Error(err, "Failed to generate webhook secret")
		return fmt.Errorf("failed to generate webhook secret: %w", err)
	}

	// Create Kubernetes Secret to store the webhook secret
	secretName := fmt.Sprintf("webhook-secret-%s", webhookResourceName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "openchoreo-control-plane",
			Labels: map[string]string{
				"openchoreo.dev/webhook": webhookResourceName,
				"openchoreo.dev/managed": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"secret": []byte(webhookSecret),
		},
	}

	if err := r.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "Failed to create webhook secret")
			return fmt.Errorf("failed to create webhook secret: %w", err)
		}
		// Secret already exists, read it to get the existing secret value
		logger.Info("Webhook secret already exists, using existing secret", "secret", secretName)
		existingSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: "openchoreo-control-plane"}, existingSecret); err != nil {
			logger.Error(err, "Failed to get existing webhook secret")
			return fmt.Errorf("failed to get existing webhook secret: %w", err)
		}
		// Use the existing secret value for webhook registration
		existingSecretData, ok := existingSecret.Data["secret"]
		if !ok || len(existingSecretData) == 0 {
			return fmt.Errorf("existing secret %s is invalid or empty", secretName)
		}
		webhookSecret = string(existingSecretData)
	} else {
		logger.Info("Created webhook secret", "secret", secretName)
	}

	// Register webhook with git provider using the generated secret
	webhookID, err := r.GitProvider.RegisterWebhook(ctx, repoURL, webhookURL, webhookSecret)
	if err != nil {
		logger.Error(err, "Failed to register webhook with git provider")
		// Clean up the secret since webhook creation failed
		_ = r.Delete(ctx, secret)
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	// Create GitRepositoryWebhook resource
	componentRef := openchoreov1alpha1.ComponentReference{
		Namespace:   comp.Namespace,
		Name:        comp.Name,
		OrgName:     comp.Namespace,
		ProjectName: comp.Spec.Owner.ProjectName,
	}

	gitWebhook = &openchoreov1alpha1.GitRepositoryWebhook{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookResourceName,
		},
		Spec: openchoreov1alpha1.GitRepositoryWebhookSpec{
			RepositoryURL:       normalizedRepoURL,
			Provider:            "github",
			WebhookID:           webhookID,
			ComponentReferences: []openchoreov1alpha1.ComponentReference{componentRef},
			WebhookSecretRef: &corev1.SecretReference{
				Name:      secretName,
				Namespace: "openchoreo-control-plane",
			},
		},
	}

	if err := r.Create(ctx, gitWebhook); err != nil {
		logger.Error(err, "Failed to create GitRepositoryWebhook")
		// Try to deregister the webhook since we couldn't create the CR
		_ = r.GitProvider.DeregisterWebhook(ctx, repoURL, webhookID)
		return fmt.Errorf("failed to create GitRepositoryWebhook: %w", err)
	}

	// Update GitRepositoryWebhook status
	gitWebhook.Status.Registered = true
	gitWebhook.Status.ReferenceCount = 1
	now := metav1.Now()
	gitWebhook.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, gitWebhook); err != nil {
		logger.Error(err, "Failed to update GitRepositoryWebhook status")
	}

	// Update component status
	registered := true
	comp.Status.WebhookRegistered = &registered
	comp.Status.WebhookID = webhookID
	comp.Status.WebhookProvider = "github"

	if err := r.Status().Update(ctx, comp); err != nil {
		logger.Error(err, "Failed to update component status")
		// Try to clean up the GitRepositoryWebhook and webhook
		_ = r.Delete(ctx, gitWebhook)
		_ = r.GitProvider.DeregisterWebhook(ctx, repoURL, webhookID)
		return fmt.Errorf("failed to update component status: %w", err)
	}

	logger.Info("Webhook registered successfully", "component", comp.Name, "webhookID", webhookID)
	return nil
}

// deregisterWebhook removes a webhook for the component's repository
func (r *Reconciler) deregisterWebhook(ctx context.Context, comp *openchoreov1alpha1.Component) error {
	logger := log.FromContext(ctx)

	// Extract repository URL from component workflow schema
	repoURL, err := extractRepoURLFromComponent(comp)
	if err != nil {
		logger.Error(err, "Failed to extract repository URL from component")
		return fmt.Errorf("failed to extract repository URL: %w", err)
	}

	// Normalize repository URL
	normalizedRepoURL := normalizeRepoURL(repoURL)

	// Find the GitRepositoryWebhook for this repository
	webhookResourceName := generateWebhookResourceName(normalizedRepoURL)
	gitWebhook := &openchoreov1alpha1.GitRepositoryWebhook{}
	gitWebhookKey := client.ObjectKey{
		Name: webhookResourceName,
	}
	err = r.Get(ctx, gitWebhookKey, gitWebhook)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("GitRepositoryWebhook not found", "webhook", webhookResourceName)
			// Update component status anyway
			registered := false
			comp.Status.WebhookRegistered = &registered
			comp.Status.WebhookID = ""
			comp.Status.WebhookProvider = ""
			_ = r.Status().Update(ctx, comp)
			return nil
		}
		logger.Error(err, "Failed to get GitRepositoryWebhook")
		return fmt.Errorf("failed to get GitRepositoryWebhook: %w", err)
	}

	// Remove this component from references
	updatedReferences := make([]openchoreov1alpha1.ComponentReference, 0)
	for _, ref := range gitWebhook.Spec.ComponentReferences {
		if ref.Namespace != comp.Namespace || ref.Name != comp.Name {
			updatedReferences = append(updatedReferences, ref)
		}
	}

	gitWebhook.Spec.ComponentReferences = updatedReferences

	// Check if there are any remaining references
	if len(updatedReferences) == 0 {
		// No more components use this webhook, delete it from git provider and remove the CR
		logger.Info("No more components using webhook, deleting from git provider", "webhookID", gitWebhook.Spec.WebhookID)

		if err := r.GitProvider.DeregisterWebhook(ctx, repoURL, gitWebhook.Spec.WebhookID); err != nil {
			logger.Error(err, "Failed to deregister webhook with git provider")
			// Continue with cleanup even if git provider deletion fails
		}

		// Delete the webhook secret if it exists
		if gitWebhook.Spec.WebhookSecretRef != nil {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitWebhook.Spec.WebhookSecretRef.Name,
					Namespace: gitWebhook.Spec.WebhookSecretRef.Namespace,
				},
			}
			if err := r.Delete(ctx, secret); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed to delete webhook secret", "secret", secret.Name)
					// Continue with cleanup even if secret deletion fails
				}
			} else {
				logger.Info("Deleted webhook secret", "secret", secret.Name)
			}
		}

		// Delete the GitRepositoryWebhook CR
		if err := r.Delete(ctx, gitWebhook); err != nil {
			logger.Error(err, "Failed to delete GitRepositoryWebhook")
			return fmt.Errorf("failed to delete GitRepositoryWebhook: %w", err)
		}

		logger.Info("GitRepositoryWebhook deleted", "webhook", webhookResourceName)
	} else {
		// Still have components using this webhook, just update the references
		logger.Info("Other components still using webhook, updating references", "remainingCount", len(updatedReferences))

		if err := r.Update(ctx, gitWebhook); err != nil {
			logger.Error(err, "Failed to update GitRepositoryWebhook")
			return fmt.Errorf("failed to update GitRepositoryWebhook: %w", err)
		}

		// Update status
		gitWebhook.Status.ReferenceCount = len(updatedReferences)
		if err := r.Status().Update(ctx, gitWebhook); err != nil {
			logger.Error(err, "Failed to update GitRepositoryWebhook status")
		}
	}

	// Update component status
	registered := false
	comp.Status.WebhookRegistered = &registered
	comp.Status.WebhookID = ""
	comp.Status.WebhookProvider = ""

	if err := r.Status().Update(ctx, comp); err != nil {
		logger.Error(err, "Failed to update component status")
		return fmt.Errorf("failed to update component status: %w", err)
	}

	logger.Info("Webhook deregistered successfully for component", "component", comp.Name)
	return nil
}

// extractRepoURLFromComponent extracts the repository URL from component workflow system parameters
func extractRepoURLFromComponent(comp *openchoreov1alpha1.Component) (string, error) {
	if comp.Spec.Workflow == nil {
		return "", fmt.Errorf("component has no workflow configuration")
	}

	// Extract repository URL from system parameters
	repoURL := comp.Spec.Workflow.SystemParameters.Repository.URL
	if repoURL == "" {
		return "", fmt.Errorf("repository URL not found in workflow system parameters")
	}

	return repoURL, nil
}

// normalizeRepoURL normalizes repository URLs for comparison
// Converts SSH URLs to HTTPS, removes .git suffix, and converts to lowercase
func normalizeRepoURL(repoURL string) string {
	// Convert SSH to HTTPS
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Convert to lowercase for case-insensitive comparison
	repoURL = strings.ToLower(repoURL)

	return repoURL
}

// generateWebhookResourceName generates a Kubernetes resource name from a repository URL
// Uses a hash-based approach to ensure the name is valid and consistent
func generateWebhookResourceName(normalizedRepoURL string) string {
	// Remove protocol prefix
	url := strings.TrimPrefix(normalizedRepoURL, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Replace invalid characters with hyphens
	url = strings.ReplaceAll(url, "/", "-")
	url = strings.ReplaceAll(url, ".", "-")
	url = strings.ReplaceAll(url, "_", "-")

	// Kubernetes resource names must be lowercase and no more than 253 characters
	// Prefix with "webhook-" for clarity
	resourceName := "webhook-" + url

	// If name is too long, truncate and add a hash
	if len(resourceName) > 253 {
		// Take first 240 characters and add a hash of the full URL
		hash := fmt.Sprintf("%x", strings.ToLower(normalizedRepoURL))
		if len(hash) > 12 {
			hash = hash[:12]
		}
		resourceName = resourceName[:240] + "-" + hash
	}

	return resourceName
}

// generateWebhookSecret generates a cryptographically secure random secret
func generateWebhookSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
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
		Watches(&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForWorkload)).
		Watches(&openchoreov1alpha1.Project{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForProject)).
		Watches(&openchoreov1alpha1.DeploymentPipeline{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForDeploymentPipeline)).
		Named("component").
		Complete(r)
}
