// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package servicebinding

import (
	"context"
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
	"github.com/openchoreo/openchoreo/internal/controller/servicebinding/render"
)

// Reconciler reconciles a ServiceBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=serviceclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Fetch the ServiceBinding instance
	serviceBinding := &openchoreov1alpha1.ServiceBinding{}
	if err := r.Get(ctx, req.NamespacedName, serviceBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ServiceBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	old := serviceBinding.DeepCopy()

	defer func() {
		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, serviceBinding.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, serviceBinding); err != nil {
			logger.Error(err, "Failed to update ServiceBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Fetch the associated ServiceClass
	serviceClass := &openchoreov1alpha1.ServiceClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: serviceBinding.Namespace,
		Name:      serviceBinding.Spec.ClassName,
	}, serviceClass); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("ServiceClass %q not found", serviceBinding.Spec.ClassName)
			controller.MarkFalseCondition(serviceBinding, ConditionReady, ReasonServiceClassNotFound, msg)
			logger.Error(err, msg)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ServiceClass", "ServiceClass", serviceBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	// Fetch all associated APIClasses from the APIs map
	apiClasses := make(map[string]*openchoreov1alpha1.APIClass)
	for apiName, serviceAPI := range serviceBinding.Spec.APIs {
		if serviceAPI != nil && serviceAPI.ClassName != "" {
			apiClass := &openchoreov1alpha1.APIClass{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: serviceBinding.Namespace,
				Name:      serviceAPI.ClassName,
			}, apiClass); err != nil {
				if apierrors.IsNotFound(err) {
					msg := fmt.Sprintf("APIClass %q not found for API %q", serviceAPI.ClassName, apiName)
					controller.MarkFalseCondition(serviceBinding, ConditionReady, ReasonAPIClassNotFound, msg)
					logger.Error(err, msg)
					return ctrl.Result{}, nil
				}
				logger.Error(err, "Failed to get APIClass", "APIClass", serviceAPI.ClassName, "API", apiName)
				return ctrl.Result{}, err
			}
			apiClasses[apiName] = apiClass
		}
	}

	if res, err := r.reconcileRelease(ctx, serviceBinding, serviceClass, apiClasses); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileRelease reconciles the Release associated with the ServiceBinding.
func (r *Reconciler) reconcileRelease(ctx context.Context, serviceBinding *openchoreov1alpha1.ServiceBinding,
	serviceClass *openchoreov1alpha1.ServiceClass, apiClasses map[string]*openchoreov1alpha1.APIClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rCtx := render.Context{
		ServiceBinding: serviceBinding,
		ServiceClass:   serviceClass,
		APIClasses:     apiClasses,
	}
	release := r.makeRelease(rCtx)
	if len(rCtx.Errors()) > 0 {
		return ctrl.Result{}, rCtx.Error()
	}

	if err := controllerutil.SetControllerReference(serviceBinding, release, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	found := &openchoreov1alpha1.Release{}
	err := r.Get(ctx, types.NamespacedName{Name: release.Name, Namespace: release.Namespace}, found)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, release); err != nil {
			err = fmt.Errorf("failed to create release %q: %w", release.Name, err)
			controller.MarkFalseCondition(serviceBinding, ConditionReady, ReasonReleaseCreationFailed, err.Error())
			return ctrl.Result{}, err
		}
		logger.Info("Release created", "Release", release.Name)
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to retrieve Release: %w", err)
	}

	desired := found.DeepCopy()
	desired.Spec = release.Spec

	changed, patchData, err := controller.HasPatchChanges(found, desired)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check Release changes: %w", err)
	}

	if changed {
		if err := r.Update(ctx, desired); err != nil {
			err = fmt.Errorf("failed to update Release %q: %w", release.Name, err)
			controller.MarkFalseCondition(serviceBinding, ConditionReady, ReasonReleaseUpdateFailed, err.Error())
			return ctrl.Result{}, err
		}
		logger.Info("Release updated", "Release", release.Name, "patch", string(patchData))
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.setReadyStatus(ctx, serviceBinding, found); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set ready status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.ServiceBinding.Name,
			Namespace: rCtx.ServiceBinding.Namespace,
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   rCtx.ServiceBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.ServiceBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.ServiceBinding.Spec.Environment,
		},
	}

	var resources []openchoreov1alpha1.Resource

	// Add Deployment resource
	if res := render.Deployment(rCtx); res != nil {
		resources = append(resources, *res)
	}

	// Add Service resource
	if res := render.Service(rCtx); res != nil {
		resources = append(resources, *res)
	}

	// Add HTTPRoute resources for REST APIs
	if res := render.HTTPRoutes(rCtx); res != nil {
		for _, httpRoute := range res {
			resources = append(resources, *httpRoute)
		}
	}

	// Add SecurityPolicy resources
	if res := render.SecurityPolicies(rCtx); res != nil {
		for _, policy := range res {
			resources = append(resources, *policy)
		}
	}

	// Add BackendTrafficPolicy resources
	if res := render.BackendTrafficPolicies(rCtx); res != nil {
		for _, policy := range res {
			resources = append(resources, *policy)
		}
	}

	release.Spec.Resources = resources
	return release
}

// setReadyStatus sets the ServiceBinding status to ready if all conditions are met in the Release.
func (r *Reconciler) setReadyStatus(ctx context.Context, serviceBinding *openchoreov1alpha1.ServiceBinding, release *openchoreov1alpha1.Release) error {
	// Count resources by health status
	totalResources := len(release.Status.Resources)

	// Handle the case where there are no resources
	if totalResources == 0 {
		message := "No resources to deploy"
		controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonAllResourcesReady, message)
		return nil
	}

	healthyCount := 0
	progressingCount := 0
	degradedCount := 0
	suspendedCount := 0

	// Check all resources using their health status
	for _, resource := range release.Status.Resources {
		switch resource.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy:
			healthyCount++
		case openchoreov1alpha1.HealthStatusSuspended:
			suspendedCount++
		case openchoreov1alpha1.HealthStatusProgressing, openchoreov1alpha1.HealthStatusUnknown:
			// Treat both progressing and unknown as progressing
			progressingCount++
		case openchoreov1alpha1.HealthStatusDegraded:
			degradedCount++
		default:
			// Treat any unrecognized health status as progressing
			progressingCount++
		}
	}

	// Check if all resources are ready (healthy or suspended)
	allResourcesReady := (healthyCount + suspendedCount) == totalResources

	// Set the ready condition based on resource health status
	if allResourcesReady {
		// Use appropriate ready reason
		if suspendedCount > 0 {
			message := fmt.Sprintf("All %d resources are ready (%d suspended)", totalResources, suspendedCount)
			controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonResourcesReadyWithSuspended, message)
		} else {
			message := fmt.Sprintf("All %d resources are deployed and healthy", totalResources)
			controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonAllResourcesReady, message)
		}
	} else {
		// Build a status message with counts
		var statusParts []string

		if progressingCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d progressing", progressingCount, totalResources))
		}
		if degradedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d degraded", degradedCount, totalResources))
		}
		if healthyCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d healthy", healthyCount, totalResources))
		}
		if suspendedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d suspended", suspendedCount, totalResources))
		}

		// Determine reason using priority: Progressing > Degraded
		var reason controller.ConditionReason
		var message string

		if progressingCount > 0 {
			// If any resource is progressing, the whole binding is progressing
			reason = ReasonResourceHealthProgressing
		} else {
			// Only degraded resources
			reason = ReasonResourceHealthDegraded
		}

		message = fmt.Sprintf("Resources status: %s", strings.Join(statusParts, ", "))
		controller.MarkFalseCondition(serviceBinding, ConditionReady, reason, message)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for service class reference
	if err := r.setupServiceClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup service class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ServiceBinding{}).
		Owns(&openchoreov1alpha1.Release{}).
		Watches(
			&openchoreov1alpha1.ServiceClass{},
			handler.EnqueueRequestsFromMapFunc(r.listServiceBindingsForServiceClass),
		).
		Named("servicebinding").
		Complete(r)
}
