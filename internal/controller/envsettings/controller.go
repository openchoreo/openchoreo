// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package envsettings

import (
	"context"
	"encoding/json"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles an EnvSettings object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=envsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=envsettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=envsettings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentenvsnapshots,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Fetch EnvSettings (primary resource)
	envSettings := &openchoreov1alpha1.EnvSettings{}
	if err := r.Get(ctx, req.NamespacedName, envSettings); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get EnvSettings")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling EnvSettings",
		"name", envSettings.Name,
		"component", envSettings.Spec.Owner.ComponentName,
		"environment", envSettings.Spec.Environment)

	// Keep a copy for comparison
	old := envSettings.DeepCopy()

	// Deferred status update
	defer func() {
		// Update observed generation
		envSettings.Status.ObservedGeneration = envSettings.Generation

		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, envSettings.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, envSettings); err != nil {
			logger.Error(err, "Failed to update EnvSettings status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Find the corresponding ComponentEnvSnapshot
	snapshot, err := r.findSnapshot(ctx, envSettings)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Snapshot not found - cannot create Release without snapshot
			msg := fmt.Sprintf("ComponentEnvSnapshot %q not found", r.buildSnapshotName(envSettings))
			controller.MarkFalseCondition(envSettings, ConditionReady,
				ReasonComponentEnvSnapshotNotFound, msg)
			logger.Info(msg,
				"component", envSettings.Spec.Owner.ComponentName,
				"environment", envSettings.Spec.Environment)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ComponentEnvSnapshot")
		return ctrl.Result{}, err
	}

	// Validate snapshot configuration
	if err := r.validateSnapshot(snapshot); err != nil {
		msg := fmt.Sprintf("Invalid snapshot configuration: %v", err)
		controller.MarkFalseCondition(envSettings, ConditionReady,
			ReasonInvalidSnapshotConfiguration, msg)
		logger.Error(err, "Snapshot validation failed")
		return ctrl.Result{}, nil
	}

	// Create or update Release
	if err := r.reconcileRelease(ctx, envSettings, snapshot); err != nil {
		logger.Error(err, "Failed to reconcile Release")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// findSnapshot finds the ComponentEnvSnapshot for the given EnvSettings
func (r *Reconciler) findSnapshot(ctx context.Context, envSettings *openchoreov1alpha1.EnvSettings) (*openchoreov1alpha1.ComponentEnvSnapshot, error) {
	snapshot := &openchoreov1alpha1.ComponentEnvSnapshot{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      r.buildSnapshotName(envSettings),
		Namespace: envSettings.Namespace,
	}, snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

// buildSnapshotName constructs the ComponentEnvSnapshot name for the given EnvSettings
func (r *Reconciler) buildSnapshotName(envSettings *openchoreov1alpha1.EnvSettings) string {
	// Snapshot name format: {componentName}-{environment}
	return fmt.Sprintf("%s-%s", envSettings.Spec.Owner.ComponentName, envSettings.Spec.Environment)
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

// reconcileRelease creates or updates the Release resource
func (r *Reconciler) reconcileRelease(ctx context.Context, envSettings *openchoreov1alpha1.EnvSettings, snapshot *openchoreov1alpha1.ComponentEnvSnapshot) error { //nolint:unparam // snapshot will be used when rendering pipeline is implemented
	logger := log.FromContext(ctx)

	// TODO: Use envSettings and snapshot data to generate actual resources.
	// This is a simplified implementation that creates a sample ConfigMap.
	// In production, this should use the rendering pipeline to generate resources.

	// Create a sample ConfigMap resource
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", envSettings.Spec.Owner.ComponentName),
			Namespace: envSettings.Namespace,
		},
		Data: map[string]string{
			"component":   envSettings.Spec.Owner.ComponentName,
			"environment": envSettings.Spec.Environment,
			"project":     envSettings.Spec.Owner.ProjectName,
			"message":     "Sample ConfigMap from EnvSettings controller",
		},
	}

	// Marshal the ConfigMap to RawExtension
	configMapBytes, err := json.Marshal(configMap)
	if err != nil {
		msg := fmt.Sprintf("Failed to marshal resources: %v", err)
		controller.MarkFalseCondition(envSettings, ConditionReady,
			ReasonRenderingFailed, msg)
		return fmt.Errorf("failed to marshal configmap: %w", err)
	}

	// Prepare Release resources
	releaseResources := []openchoreov1alpha1.Resource{
		{
			ID:     "sample-configmap",
			Object: &runtime.RawExtension{Raw: configMapBytes},
		},
	}

	// Create or update Release
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envSettings.Name,
			Namespace: envSettings.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, release, func() error {
		// Check if we own this Release
		if !r.isOwnedByEnvSettings(release, envSettings) && release.UID != "" {
			// Release exists but not owned by us
			return fmt.Errorf("release exists but is not owned by this EnvSettings")
		}

		// Set labels (replace entire map to ensure old labels don't persist)
		release.Labels = map[string]string{
			labels.LabelKeyOrganizationName: envSettings.Namespace,
			labels.LabelKeyProjectName:      envSettings.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:    envSettings.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName:  envSettings.Spec.Environment,
		}

		// Set spec
		release.Spec = openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   envSettings.Spec.Owner.ProjectName,
				ComponentName: envSettings.Spec.Owner.ComponentName,
			},
			EnvironmentName: envSettings.Spec.Environment,
			Resources:       releaseResources,
		}

		return controllerutil.SetControllerReference(envSettings, release, r.Scheme)
	})

	if err != nil {
		// Check for ownership conflict (permanent error - don't retry)
		if strings.Contains(err.Error(), "not owned by") {
			msg := fmt.Sprintf("Release %q exists but is owned by another resource", release.Name)
			controller.MarkFalseCondition(envSettings, ConditionReady,
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
		controller.MarkFalseCondition(envSettings, ConditionReady, reason, msg)
		logger.Error(err, "Failed to reconcile Release", "release", release.Name)
		return err
	}

	// Success - mark as ready
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		msg := fmt.Sprintf("Release %q successfully %s with %d resources",
			release.Name, op, len(releaseResources))
		controller.MarkTrueCondition(envSettings, ConditionReady, ReasonReleaseReady, msg)
		logger.Info("Successfully reconciled Release",
			"release", release.Name,
			"operation", op,
			"resourceCount", len(releaseResources))
	}

	return nil
}

// isOwnedByEnvSettings checks if the Release is owned by the given EnvSettings
func (r *Reconciler) isOwnedByEnvSettings(release *openchoreov1alpha1.Release,
	envSettings *openchoreov1alpha1.EnvSettings) bool {
	for _, ref := range release.GetOwnerReferences() {
		if ref.UID == envSettings.UID {
			return true
		}
	}
	return false
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.EnvSettings{}).
		Owns(&openchoreov1alpha1.Release{}).
		Watches(&openchoreov1alpha1.ComponentEnvSnapshot{},
			handler.EnqueueRequestsFromMapFunc(r.listEnvSettingsForSnapshot)).
		Named("envsettings").
		Complete(r)
}
