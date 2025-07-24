// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplicationbinding

import (
	"context"
	"fmt"

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
	"github.com/openchoreo/openchoreo/internal/labels"
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings,verbs=get;list;watch

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

	// Resolve API connections
	resolvedConnections, err := r.resolveApiConnections(ctx, webApplicationBinding)
	if err != nil {
		logger.Error(err, "Failed to resolve API connections")
		return ctrl.Result{}, err
	}

	rCtx := render.Context{
		WebApplicationBinding: webApplicationBinding,
		WebApplicationClass:   webApplicationClass,
		ResolvedConnections:   resolvedConnections,
	}

	release := r.makeRelease(rCtx)
	if len(rCtx.Errors()) > 0 {
		return ctrl.Result{}, rCtx.Error()
	}

	if err := controllerutil.SetControllerReference(webApplicationBinding, release, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	found := &openchoreov1alpha1.Release{}
	err = r.Get(ctx, client.ObjectKey{Name: release.Name, Namespace: release.Namespace}, found)
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
	desired.Labels = release.Labels
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

	// Update endpoint status after resources are ready
	if err := r.updateEndpointStatus(ctx, webApplicationBinding); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update endpoint status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.WebApplicationBinding.Name,
			Namespace: rCtx.WebApplicationBinding.Namespace,
			Labels:    r.makeLabels(rCtx.WebApplicationBinding),
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

// makeLabels creates standard labels for Release resources, merging with WebApplicationBinding labels.
func (r *Reconciler) makeLabels(webApplicationBinding *openchoreov1alpha1.WebApplicationBinding) map[string]string {
	// Start with WebApplicationBinding's existing labels
	result := make(map[string]string)
	for k, v := range webApplicationBinding.Labels {
		result[k] = v
	}
	
	// Add/overwrite component-specific labels
	result[labels.LabelKeyOrganizationName] = webApplicationBinding.Namespace // namespace = organization
	result[labels.LabelKeyProjectName] = webApplicationBinding.Spec.Owner.ProjectName
	result[labels.LabelKeyComponentName] = webApplicationBinding.Spec.Owner.ComponentName
	result[labels.LabelKeyEnvironmentName] = webApplicationBinding.Spec.Environment
	
	return result
}

func (r *Reconciler) resolveApiConnections(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	wls := webApplicationBinding.Spec.WorkloadSpec
	for connectionName, connection := range wls.Connections {
		if connection.Type != openchoreov1alpha1.ConnectionTypeAPI {
			continue // Skip non-API connections for now
		}

		// Extract parameters
		targetComponentName := connection.Params["componentName"]
		targetEndpointName := connection.Params["endpoint"]

		// Find target binding
		targetBinding, err := r.findTargetServiceBinding(ctx, webApplicationBinding.Namespace, targetComponentName, webApplicationBinding.Spec.Environment)
		if err != nil {
			return nil, fmt.Errorf("failed to find target binding for connection %s: %w", connectionName, err)
		}

		// Extract endpoint from binding status
		var endpointAccess *openchoreov1alpha1.EndpointAccess
		for _, ep := range targetBinding.Status.Endpoints {
			if ep.Name == targetEndpointName {
				endpointAccess = ep.Project // For POC, assume project-level access
				break
			}
		}

		if endpointAccess == nil {
			return nil, fmt.Errorf("endpoint %s not found in target binding %s", targetEndpointName, targetComponentName)
		}

		// Build result map with template variables
		results[connectionName] = endpointAccess
	}
	return results, nil
}

func (r *Reconciler) findTargetServiceBinding(ctx context.Context, namespace, componentName, environment string) (*openchoreov1alpha1.ServiceBinding, error) {
	// List all ServiceBindings in the namespace
	bindingList := &openchoreov1alpha1.ServiceBindingList{}
	if err := r.List(ctx, bindingList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	// Find binding that matches both component name and environment
	for _, binding := range bindingList.Items {
		if binding.Spec.Owner.ComponentName == componentName && binding.Spec.Environment == environment {
			return &binding, nil
		}
	}

	return nil, fmt.Errorf("no service binding found for component %s in environment %s", componentName, environment)
}
