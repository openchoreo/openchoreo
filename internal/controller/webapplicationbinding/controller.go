// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplicationbinding

import (
	"context"
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/webapplicationbinding/render"
)

// Reconciler reconciles a WebApplicationBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (rResult ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Fetch the WebApplicationBinding instance
	webApplicationBinding := &openchoreov1alpha1.WebApplicationBinding{}
	if err := r.Get(ctx, req.NamespacedName, webApplicationBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get WebApplicationBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	old := webApplicationBinding.DeepCopy()

	defer func() {
		// Skip update if nothing changed
		if apiequality.Semantic.DeepEqual(old.Status, webApplicationBinding.Status) {
			return
		}

		// Update the status
		if err := r.Status().Update(ctx, webApplicationBinding); err != nil {
			logger.Error(err, "Failed to update WebApplicationBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Fetch the associated WebApplicationClass
	webApplicationClass := &openchoreov1alpha1.WebApplicationClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: webApplicationBinding.Namespace,
		Name:      webApplicationBinding.Spec.ClassName,
	}, webApplicationClass); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("WebApplicationClass %q not found", webApplicationBinding.Spec.ClassName)
			controller.MarkFalseCondition(webApplicationBinding, ConditionReady, ReasonWebApplicationClassNotFound, msg)
			logger.Error(err, msg, "webApplicationClassName", webApplicationBinding.Spec.ClassName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get WebApplicationClass", "WebApplicationClass", webApplicationBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	if res, err := r.reconcileRelease(ctx, webApplicationBinding, webApplicationClass); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileRelease reconciles the Release associated with the WebApplicationBinding.
func (r *Reconciler) reconcileRelease(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding, webApplicationClass *openchoreov1alpha1.WebApplicationClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rCtx := render.Context{
		WebApplicationBinding: webApplicationBinding,
		WebApplicationClass:   webApplicationClass,
	}

	release := r.makeRelease(rCtx)
	if len(rCtx.Errors()) > 0 {
		return ctrl.Result{}, rCtx.Error()
	}

	if err := controllerutil.SetControllerReference(webApplicationBinding, release, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	found := &openchoreov1alpha1.Release{}
	err := r.Get(ctx, client.ObjectKey{Name: release.Name, Namespace: release.Namespace}, found)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, release); err != nil {
			err = fmt.Errorf("failed to create release %q: %w", release.Name, err)
			controller.MarkFalseCondition(webApplicationBinding, ConditionReady, ReasonReleaseCreationFailed, err.Error())
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
			controller.MarkFalseCondition(webApplicationBinding, ConditionReady, ReasonReleaseUpdateFailed, err.Error())
			return ctrl.Result{}, err
		}
		logger.Info("Release updated", "Release", release.Name, "patch", string(patchData))
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.setReadyStatus(ctx, webApplicationBinding, found); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set ready status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.WebApplicationBinding.Name,
			Namespace: rCtx.WebApplicationBinding.Namespace,
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   rCtx.WebApplicationBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.WebApplicationBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.WebApplicationBinding.Spec.Environment,
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

	// Add HTTPRoute resources (to act as ingress)
	if res := render.HTTPRoutes(rCtx); res != nil {
		for _, httpRoute := range res {
			resources = append(resources, *httpRoute)
		}
	}

	release.Spec.Resources = resources
	return release
}

// setReadyStatus sets the WebApplicationBinding status to ready if all conditions are met in the Release.
func (r *Reconciler) setReadyStatus(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding, release *openchoreov1alpha1.Release) error {
	// Count resources by health status
	totalResources := len(release.Status.Resources)
	
	// Handle the case where there are no resources
	if totalResources == 0 {
		message := "No resources to deploy"
		controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonAllResourcesReady, message)
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
			controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonResourcesReadyWithSuspended, message)
		} else {
			message := fmt.Sprintf("All %d resources are deployed and healthy", totalResources)
			controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonAllResourcesReady, message)
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
		controller.MarkFalseCondition(webApplicationBinding, ConditionReady, reason, message)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for web application class reference
	if err := r.setupWebApplicationClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup web application class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.WebApplicationBinding{}).
		Owns(&openchoreov1alpha1.Release{}).
		Watches(
			&openchoreov1alpha1.WebApplicationClass{},
			handler.EnqueueRequestsFromMapFunc(r.listWebApplicationBindingsForWebApplicationClass),
		).
		Named("webapplicationbinding").
		Complete(r)
}
