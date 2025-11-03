// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"context"
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	componentpipeline "github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline/context"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles an ComponentDeployment object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline is the component rendering pipeline, shared across all reconciliations.
	// This enables CEL environment caching across different component types and reconciliations.
	Pipeline *componentpipeline.Pipeline
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Fetch ComponentDeployment (primary resource)
	componentDeployment := &openchoreov1alpha1.ComponentDeployment{}
	if err := r.Get(ctx, req.NamespacedName, componentDeployment); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ComponentDeployment")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling ComponentDeployment",
		"name", componentDeployment.Name,
		"component", componentDeployment.Spec.Owner.ComponentName,
		"environment", componentDeployment.Spec.Environment)

	// Keep a copy for comparison
	old := componentDeployment.DeepCopy()

	// Deferred status update
	defer func() {
		// Update observed generation
		componentDeployment.Status.ObservedGeneration = componentDeployment.Generation

		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, componentDeployment.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, componentDeployment); err != nil {
			logger.Error(err, "Failed to update ComponentDeployment status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Find the corresponding ComponentEnvSnapshot
	snapshot, err := r.findSnapshot(ctx, componentDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Snapshot not found - cannot create Release without snapshot
			msg := fmt.Sprintf("ComponentEnvSnapshot for component %q in environment %q not found",
				componentDeployment.Spec.Owner.ComponentName,
				componentDeployment.Spec.Environment)
			controller.MarkFalseCondition(componentDeployment, ConditionReady,
				ReasonComponentEnvSnapshotNotFound, msg)
			logger.Info(msg,
				"component", componentDeployment.Spec.Owner.ComponentName,
				"environment", componentDeployment.Spec.Environment,
				"project", componentDeployment.Spec.Owner.ProjectName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ComponentEnvSnapshot")
		return ctrl.Result{}, err
	}

	// Validate snapshot configuration
	if err := r.validateSnapshot(snapshot); err != nil {
		msg := fmt.Sprintf("Invalid snapshot configuration: %v", err)
		controller.MarkFalseCondition(componentDeployment, ConditionReady,
			ReasonInvalidSnapshotConfiguration, msg)
		logger.Error(err, "Snapshot validation failed")
		return ctrl.Result{}, nil
	}

	// Create or update Release
	if err := r.reconcileRelease(ctx, componentDeployment, snapshot); err != nil {
		logger.Error(err, "Failed to reconcile Release")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// findSnapshot finds the ComponentEnvSnapshot for the given ComponentDeployment by owner fields
func (r *Reconciler) findSnapshot(ctx context.Context, componentDeployment *openchoreov1alpha1.ComponentDeployment) (*openchoreov1alpha1.ComponentEnvSnapshot, error) {
	logger := log.FromContext(ctx)

	// Build the owner key: projectName/componentName/environment
	ownerKey := fmt.Sprintf("%s/%s/%s",
		componentDeployment.Spec.Owner.ProjectName,
		componentDeployment.Spec.Owner.ComponentName,
		componentDeployment.Spec.Environment)

	// Query snapshots by owner index
	var snapshotList openchoreov1alpha1.ComponentEnvSnapshotList
	err := r.List(ctx, &snapshotList,
		client.InNamespace(componentDeployment.Namespace),
		client.MatchingFields{snapshotOwnerIndex: ownerKey})
	if err != nil {
		logger.Error(err, "Failed to list ComponentEnvSnapshot by owner")
		return nil, err
	}

	// Handle no snapshots found
	if len(snapshotList.Items) == 0 {
		return nil, apierrors.NewNotFound(
			openchoreov1alpha1.GroupVersion.WithResource("componentenvsnapshots").GroupResource(),
			ownerKey)
	}

	// Handle multiple snapshots (should not happen but check anyway)
	if len(snapshotList.Items) > 1 {
		logger.Error(fmt.Errorf("multiple snapshots found"), "Multiple ComponentEnvSnapshots found",
			"ownerKey", ownerKey,
			"count", len(snapshotList.Items))
		return nil, fmt.Errorf("multiple ComponentEnvSnapshots found for owner %q (expected exactly 1)", ownerKey)
	}

	return &snapshotList.Items[0], nil
}

// validateSnapshot validates the ComponentEnvSnapshot configuration
func (r *Reconciler) validateSnapshot(snapshot *openchoreov1alpha1.ComponentEnvSnapshot) error {
	// Check ComponentTypeDefinition exists and has resources
	if snapshot.Spec.ComponentTypeDefinition.Spec.Resources == nil {
		return fmt.Errorf("component type definition has no resources")
	}

	// Check Component is present
	if snapshot.Spec.Component.Name == "" {
		return fmt.Errorf("component name is empty")
	}

	// Check Workload is present
	if snapshot.Spec.Workload.Name == "" {
		return fmt.Errorf("workload name is empty")
	}

	// Check required owner fields
	if snapshot.Spec.Owner.ProjectName == "" {
		return fmt.Errorf("snapshot owner missing required field: projectName")
	}
	if snapshot.Spec.Owner.ComponentName == "" {
		return fmt.Errorf("snapshot owner missing required field: componentName")
	}

	return nil
}

// buildMetadataContext creates the MetadataContext from snapshot.
// This is where the controller computes K8s resource names and namespaces.
func (r *Reconciler) buildMetadataContext(
	snapshot *openchoreov1alpha1.ComponentEnvSnapshot,
) pipelinecontext.MetadataContext {
	// Extract information
	organizationName := snapshot.Namespace
	projectName := snapshot.Spec.Owner.ProjectName
	componentName := snapshot.Spec.Owner.ComponentName
	environment := snapshot.Spec.Environment

	// Generate base name using platform naming conventions
	// Format: {component}-{env}-{hash}
	// Example: "payment-service-dev-a1b2c3d4"
	baseName := dpkubernetes.GenerateK8sName(componentName, environment)

	// Generate namespace using platform naming conventions
	// Format: dp-{org}-{project}-{env}-{hash}
	// Example: "dp-acme-corp-payment-dev-x1y2z3w4"
	namespace := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environment,
	)

	// Build standard labels
	standardLabels := map[string]string{
		labels.LabelKeyOrganizationName: organizationName,
		labels.LabelKeyProjectName:      projectName,
		labels.LabelKeyComponentName:    componentName,
		labels.LabelKeyEnvironmentName:  environment,
	}

	// Build pod selectors (used for Deployment selectors, Service selectors, etc.)
	podSelectors := map[string]string{
		"openchoreo.org/component":   componentName,
		"openchoreo.org/environment": environment,
		"openchoreo.org/project":     projectName,
	}

	return pipelinecontext.MetadataContext{
		Name:         baseName,
		Namespace:    namespace,
		Labels:       standardLabels,
		Annotations:  map[string]string{}, // Can be extended later
		PodSelectors: podSelectors,
	}
}

// reconcileRelease creates or updates the Release resource
func (r *Reconciler) reconcileRelease(ctx context.Context, componentDeployment *openchoreov1alpha1.ComponentDeployment, snapshot *openchoreov1alpha1.ComponentEnvSnapshot) error {
	logger := log.FromContext(ctx)

	// Build MetadataContext with computed names
	metadataContext := r.buildMetadataContext(snapshot)

	// Prepare RenderInput
	renderInput := &componentpipeline.RenderInput{
		Snapshot:            snapshot,
		ComponentDeployment: componentDeployment,
		Metadata:            metadataContext,
	}

	// Render resources using the shared pipeline instance
	// The pipeline caches CEL environments, so subsequent reconciliations benefit from warm cache
	renderOutput, err := r.Pipeline.Render(renderInput)
	if err != nil {
		msg := fmt.Sprintf("Failed to render resources: %v", err)
		controller.MarkFalseCondition(componentDeployment, ConditionReady,
			ReasonRenderingFailed, msg)
		logger.Error(err, "Failed to render resources")
		return fmt.Errorf("failed to render resources: %w", err)
	}

	// Log warnings if any
	if len(renderOutput.Metadata.Warnings) > 0 {
		logger.Info("Rendering completed with warnings",
			"warnings", renderOutput.Metadata.Warnings)
	}

	// Convert rendered resources to Release format
	releaseResources := r.convertToReleaseResources(renderOutput.Resources)

	// Create or update Release
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentDeployment.Name,
			Namespace: componentDeployment.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, release, func() error {
		// Check if we own this Release (only for existing releases)
		if release.UID != "" {
			hasOwner, err := controllerutil.HasOwnerReference(release.GetOwnerReferences(), componentDeployment, r.Scheme)
			if err != nil {
				return fmt.Errorf("failed to check owner reference: %w", err)
			}
			if !hasOwner {
				// Release exists but not owned by us
				return fmt.Errorf("release exists but is not owned by this ComponentDeployment")
			}
		}

		// Set labels (replace entire map to ensure old labels don't persist)
		release.Labels = map[string]string{
			labels.LabelKeyOrganizationName: componentDeployment.Namespace,
			labels.LabelKeyProjectName:      componentDeployment.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:    componentDeployment.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName:  componentDeployment.Spec.Environment,
		}

		// Set spec
		release.Spec = openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   componentDeployment.Spec.Owner.ProjectName,
				ComponentName: componentDeployment.Spec.Owner.ComponentName,
			},
			EnvironmentName: componentDeployment.Spec.Environment,
			Resources:       releaseResources,
		}

		return controllerutil.SetControllerReference(componentDeployment, release, r.Scheme)
	})

	if err != nil {
		// Check for ownership conflict (permanent error - don't retry)
		if strings.Contains(err.Error(), "not owned by") {
			msg := fmt.Sprintf("Release %q exists but is owned by another resource", release.Name)
			controller.MarkFalseCondition(componentDeployment, ConditionReady,
				ReasonReleaseOwnershipConflict, msg)
			logger.Error(err, msg)
			return nil
		}

		// Transient errors - return error to trigger automatic retry
		var reason controller.ConditionReason
		if op == controllerutil.OperationResultCreated {
			reason = ReasonReleaseCreationFailed
		} else {
			reason = ReasonReleaseUpdateFailed
		}
		msg := fmt.Sprintf("Failed to reconcile Release: %v", err)
		controller.MarkFalseCondition(componentDeployment, ConditionReady, reason, msg)
		logger.Error(err, "Failed to reconcile Release", "release", release.Name)
		return err
	}

	// Success - mark as ready
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		msg := fmt.Sprintf("Release %q successfully %s with %d resources",
			release.Name, op, len(releaseResources))
		controller.MarkTrueCondition(componentDeployment, ConditionReady, ReasonReleaseReady, msg)
		logger.Info("Successfully reconciled Release",
			"release", release.Name,
			"operation", op,
			"resourceCount", len(releaseResources))
	}

	return nil
}

// convertToReleaseResources converts unstructured resources to Release.Resource format
func (r *Reconciler) convertToReleaseResources(
	resources []map[string]any,
) []openchoreov1alpha1.Resource {
	releaseResources := make([]openchoreov1alpha1.Resource, 0, len(resources))

	for i, resource := range resources {
		// Generate resource ID
		id := r.generateResourceID(resource, i)

		releaseResources = append(releaseResources, openchoreov1alpha1.Resource{
			ID: id,
			Object: &runtime.RawExtension{
				Object: &unstructured.Unstructured{
					Object: resource,
				},
			},
		})
	}
	return releaseResources
}

// generateResourceID creates a unique ID for a resource
// Format: {kind-lower}-{name}
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := r.setupComponentIndex(ctx, mgr); err != nil {
		return err
	}

	if err := r.setupEnvironmentIndex(ctx, mgr); err != nil {
		return err
	}

	if err := r.setupComponentDeploymentCompositeIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup component deployment composite index: %w", err)
	}

	if err := r.setupSnapshotOwnerIndex(ctx, mgr); err != nil {
		return fmt.Errorf("failed to setup snapshot owner index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ComponentDeployment{}).
		Owns(&openchoreov1alpha1.Release{}).
		Watches(&openchoreov1alpha1.ComponentEnvSnapshot{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentDeploymentForSnapshot)).
		Named("componentdeployment").
		Complete(r)
}
