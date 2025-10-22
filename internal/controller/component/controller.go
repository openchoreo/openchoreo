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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=addons,verbs=get;list;watch
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
		// New ComponentTypeDefinition mode
		logger.Info("Reconciling Component with ComponentTypeDefinition mode",
			"component", comp.Name,
			"componentType", comp.Spec.ComponentType)
		return r.reconcileWithComponentTypeDefinition(ctx, comp)
	}

	// Legacy mode - no action needed for now
	logger.Info("Component using legacy mode - no snapshot management",
		"component", comp.Name,
		"type", comp.Spec.Type)
	return ctrl.Result{}, nil
}

// reconcileWithComponentTypeDefinition handles components using ComponentTypeDefinitions
func (r *Reconciler) reconcileWithComponentTypeDefinition(ctx context.Context, comp *openchoreov1alpha1.Component) (result ctrl.Result, rErr error) {
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

	// Parse componentType: {workloadType}/{componentTypeDefinitionName}
	workloadType, ctdName, err := parseComponentType(comp.Spec.ComponentType)
	if err != nil {
		msg := fmt.Sprintf("Invalid componentType format: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration, msg)
		logger.Error(err, "Failed to parse componentType")
		return ctrl.Result{}, nil
	}

	// Fetch ComponentTypeDefinition (in the same namespace as the Component)
	ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
	if err := r.Get(ctx, types.NamespacedName{Name: ctdName, Namespace: comp.Namespace}, ctd); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ComponentTypeDefinition %q not found", ctdName)
			controller.MarkFalseCondition(comp, ConditionReady, ReasonComponentTypeDefinitionNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch ComponentTypeDefinition", "name", ctdName)
		return ctrl.Result{}, err
	}

	// Verify workloadType matches
	if ctd.Spec.WorkloadType != workloadType {
		msg := fmt.Sprintf("WorkloadType mismatch: component specifies %s but ComponentTypeDefinition has %s",
			workloadType, ctd.Spec.WorkloadType)
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

	// Fetch all referenced Addons (in the same namespace as the Component)
	addons, err := r.fetchAddons(ctx, comp.Spec.Addons, comp.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Extract addon name from custom error type
			var addonErr *addonFetchError
			if errors.As(err, &addonErr) {
				msg := fmt.Sprintf("Addon %q not found", addonErr.addonName)
				controller.MarkFalseCondition(comp, ConditionReady, ReasonAddonNotFound, msg)
				logger.Info(msg, "component", comp.Name)
				return ctrl.Result{}, nil
			}
			// Fallback if error type doesn't match
			msg := "One or more Addons not found"
			controller.MarkFalseCondition(comp, ConditionReady, ReasonAddonNotFound, msg)
			logger.Info(msg, "component", comp.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch Addons")
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

	if err := r.createOrUpdateSnapshot(ctx, comp, ctd, workload, addons, firstEnv); err != nil {
		msg := fmt.Sprintf("Failed to create/update ComponentEnvSnapshot: %v", err)
		controller.MarkFalseCondition(comp, ConditionReady, ReasonSnapshotCreationFailed, msg)
		logger.Error(err, "Failed to create/update ComponentEnvSnapshot")
		return ctrl.Result{}, err
	}

	// Success - mark as ready
	msg := fmt.Sprintf("ComponentEnvSnapshot successfully created/updated for environment %q", firstEnv)
	controller.MarkTrueCondition(comp, ConditionReady, ReasonSnapshotReady, msg)
	logger.Info("Successfully reconciled Component with ComponentTypeDefinition",
		"component", comp.Name,
		"environment", firstEnv)

	return ctrl.Result{}, nil
}

// parseComponentType parses the componentType format: {workloadType}/{componentTypeDefinitionName}
func parseComponentType(componentType string) (workloadType string, ctdName string, err error) {
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid componentType format: expected {workloadType}/{name}, got %s", componentType)
	}
	return parts[0], parts[1], nil
}

// addonFetchError wraps an error from fetching an addon with the addon name
type addonFetchError struct {
	addonName string
	err       error
}

func (e *addonFetchError) Error() string {
	return fmt.Sprintf("failed to fetch addon %q: %v", e.addonName, e.err)
}

func (e *addonFetchError) Unwrap() error {
	return e.err
}

// fetchAddons fetches all Addon resources referenced by the component
func (r *Reconciler) fetchAddons(ctx context.Context, addonRefs []openchoreov1alpha1.ComponentAddon, namespace string) ([]openchoreov1alpha1.Addon, error) {
	addons := make([]openchoreov1alpha1.Addon, 0, len(addonRefs))

	for _, ref := range addonRefs {
		addon := &openchoreov1alpha1.Addon{}
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, addon); err != nil {
			return nil, &addonFetchError{addonName: ref.Name, err: err}
		}
		addons = append(addons, *addon)
	}

	return addons, nil
}

// createOrUpdateSnapshot creates or updates a ComponentEnvSnapshot with embedded copies
func (r *Reconciler) createOrUpdateSnapshot(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ctd *openchoreov1alpha1.ComponentTypeDefinition,
	workload *openchoreov1alpha1.Workload,
	addons []openchoreov1alpha1.Addon,
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
		sanitizedCTD := sanitizeComponentTypeDefinition(ctd)
		sanitizedComponent := sanitizeComponent(comp)
		sanitizedWorkload := sanitizeWorkload(workload)
		sanitizedAddons := sanitizeAddons(addons)

		if sanitizedCTD != nil {
			snapshot.Spec.ComponentTypeDefinition = *sanitizedCTD
		}
		if sanitizedComponent != nil {
			snapshot.Spec.Component = *sanitizedComponent
		}
		if len(sanitizedAddons) > 0 {
			snapshot.Spec.Addons = sanitizedAddons
		} else {
			snapshot.Spec.Addons = nil
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

func sanitizeComponentTypeDefinition(ctd *openchoreov1alpha1.ComponentTypeDefinition) *openchoreov1alpha1.ComponentTypeDefinition {
	if ctd == nil {
		return nil
	}
	copy := ctd.DeepCopy()
	sanitizeObjectMeta(&copy.ObjectMeta)
	copy.Status = openchoreov1alpha1.ComponentTypeDefinitionStatus{}
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

func sanitizeAddons(addons []openchoreov1alpha1.Addon) []openchoreov1alpha1.Addon {
	if len(addons) == 0 {
		return nil
	}
	sanitized := make([]openchoreov1alpha1.Addon, 0, len(addons))
	for i := range addons {
		copy := addons[i].DeepCopy()
		sanitizeObjectMeta(&copy.ObjectMeta)
		copy.Status = openchoreov1alpha1.AddonStatus{}
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

	if err := r.setupAddonsRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup addons reference index: %w", err)
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
		Watches(&openchoreov1alpha1.ComponentTypeDefinition{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForComponentType)).
		Watches(&openchoreov1alpha1.Addon{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsUsingAddon)).
		Watches(&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForWorkload)).
		Named("component").
		Complete(r)
}
