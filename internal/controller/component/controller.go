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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a Component object
type Reconciler struct {
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
	// In GitOps mode, the controller will not create or update resources directly in the cluster,
	// but will instead generate the necessary manifests and creates GitCommitRequests to update the Git repository.
	IsGitOpsMode bool
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=traits,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots,verbs=get;list;watch;create;update;patch;delete
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
	// TODO: Add watch for DeploymentPipeline in SetupWithManager.
	// If the DeploymentPipeline's promotion paths are reordered after Component creation,
	// the root environment might change, requiring the snapshot to be regenerated.
	// Currently, Components won't be re-reconciled when this happens.
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

	if err := r.createOrUpdateSnapshot(ctx, comp, ct, workload, traits, firstEnv); err != nil {
		msg := fmt.Sprintf("Failed to create/update ComponentEnvSnapshot: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonSnapshotCreationFailed, msg)
		logger.Error(err, "Failed to create/update ComponentEnvSnapshot")
		return ctrl.Result{}, err
	}

	// Success - mark as ready
	msg := fmt.Sprintf("ComponentEnvSnapshot successfully created/updated for environment %q", firstEnv)
	controller.MarkTrueCondition(comp, ConditionReady, ReasonSnapshotReady, msg)
	logger.Info("Successfully reconciled Component with ComponentType",
		"component", comp.Name,
		"environment", firstEnv)

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

// createOrUpdateSnapshot creates or updates a ComponentEnvSnapshot with embedded copies
func (r *Reconciler) createOrUpdateSnapshot(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ct *openchoreov1alpha1.ComponentType,
	workload *openchoreov1alpha1.Workload,
	traits []openchoreov1alpha1.Trait,
	environment string,
) error {
	logger := log.FromContext(ctx)

	snapshotName := fmt.Sprintf("%s-%s", comp.Name, environment)
	snapshot := &openchoreov1alpha1.ComponentEnvSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: comp.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, snapshot, func() error {
		if r.Scheme == nil {
			return fmt.Errorf("reconciler scheme is nil")
		}

		snapshot.Spec.Owner = openchoreov1alpha1.ComponentEnvSnapshotOwner{
			ProjectName:   comp.Spec.Owner.ProjectName,
			ComponentName: comp.Name,
		}
		snapshot.Spec.Environment = environment
		sanitizedCT := sanitizeComponentType(ct)
		sanitizedComponent := sanitizeComponent(comp)
		sanitizedWorkload := sanitizeWorkload(workload)
		sanitizedTraits := sanitizeTraits(traits)

		if sanitizedCT != nil {
			snapshot.Spec.ComponentType = *sanitizedCT
		}
		if sanitizedComponent != nil {
			snapshot.Spec.Component = *sanitizedComponent
		}
		if len(sanitizedTraits) > 0 {
			snapshot.Spec.Traits = sanitizedTraits
		} else {
			snapshot.Spec.Traits = nil
		}
		if sanitizedWorkload != nil {
			snapshot.Spec.Workload = *sanitizedWorkload
		}

		return controllerutil.SetControllerReference(comp, snapshot, r.Scheme)
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		logger.Info("Reconciled ComponentEnvSnapshot",
			"snapshot", snapshotName,
			"component", comp.Name,
			"environment", environment,
			"operation", op)
	}
	return nil
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

func sanitizeComponentType(ct *openchoreov1alpha1.ComponentType) *openchoreov1alpha1.ComponentType {
	if ct == nil {
		return nil
	}
	copy := ct.DeepCopy()
	sanitizeObjectMeta(&copy.ObjectMeta)
	copy.Status = openchoreov1alpha1.ComponentTypeStatus{}
	return copy
}

func sanitizeComponent(comp *openchoreov1alpha1.Component) *openchoreov1alpha1.Component {
	if comp == nil {
		return nil
	}
	copy := comp.DeepCopy()
	sanitizeObjectMeta(&copy.ObjectMeta)
	copy.Status = openchoreov1alpha1.ComponentStatus{}
	return copy
}

func sanitizeWorkload(workload *openchoreov1alpha1.Workload) *openchoreov1alpha1.Workload {
	if workload == nil {
		return nil
	}
	copy := workload.DeepCopy()
	sanitizeObjectMeta(&copy.ObjectMeta)
	copy.Status = openchoreov1alpha1.WorkloadStatus{}
	return copy
}

func sanitizeTraits(traits []openchoreov1alpha1.Trait) []openchoreov1alpha1.Trait {
	if len(traits) == 0 {
		return nil
	}
	sanitized := make([]openchoreov1alpha1.Trait, 0, len(traits))
	for i := range traits {
		copy := traits[i].DeepCopy()
		sanitizeObjectMeta(&copy.ObjectMeta)
		copy.Status = openchoreov1alpha1.TraitStatus{}
		sanitized = append(sanitized, *copy)
	}
	return sanitized
}

func sanitizeObjectMeta(meta *metav1.ObjectMeta) {
	if meta == nil {
		return
	}

	// Preserve identity fields that are required for templating.
	name := meta.Name
	namespace := meta.Namespace
	labels := meta.Labels
	generateName := meta.GenerateName

	// Filter out kubectl-specific annotations
	filteredAnnotations := make(map[string]string)
	for k, v := range meta.Annotations {
		// Skip kubectl.kubernetes.io/* annotations
		if !strings.HasPrefix(k, "kubectl.kubernetes.io/") {
			filteredAnnotations[k] = v
		}
	}

	*meta = metav1.ObjectMeta{
		Name:         name,
		Namespace:    namespace,
		Labels:       labels,
		Annotations:  filteredAnnotations,
		GenerateName: generateName,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.IsGitOpsMode = true

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

	// TODO: Add watch for DeploymentPipeline.
	// Components depend on the DeploymentPipeline's promotion paths to determine the root environment.
	// If the promotion path order changes after Component creation, Components won't be re-reconciled
	// and may continue using the wrong environment for snapshots.
	// Need to implement:
	// - Watches(&openchoreov1alpha1.DeploymentPipeline{}, handler.EnqueueRequestsFromMapFunc(...))
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Component{}).
		Owns(&openchoreov1alpha1.ComponentEnvSnapshot{}).
		Watches(&openchoreov1alpha1.ComponentType{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForComponentType)).
		Watches(&openchoreov1alpha1.Trait{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsUsingTrait)).
		Watches(&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForWorkload)).
		Named("component").
		Complete(r)
}
