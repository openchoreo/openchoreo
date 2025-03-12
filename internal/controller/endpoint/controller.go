/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package endpoint

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes"
	k8sintegrations "github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/choreo-idp/choreo/internal/dataplane"
)

// Reconciler reconciles a Endpoint object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Endpoint object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get Endpoint CR
	ep := &choreov1.Endpoint{}
	if err := r.Get(ctx, req.NamespacedName, ep); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	old := ep.DeepCopy()

	if ep.Labels == nil {
		logger.Info("Endpoint labels not set.")
		return ctrl.Result{}, nil
	}

	resourceHandlers := r.makeExternalResourceHandlers()
	epCtx, err := r.makeEndpointContext(ctx, ep)
	if err != nil {
		logger.Error(err, "Failed to create endpoint context")
		r.recorder.Eventf(ep, corev1.EventTypeWarning, "ContextResolutionFailed",
			"Context resolution failed: %v", err)
		return ctrl.Result{}, controller.IgnoreHierarchyNotFoundError(err)
	}

	if !ep.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing endpoint")
		return r.finalize(ctx, old, ep)
	}

	// Ensure the finalizer is added to the deployment
	if err := r.ensureFinalizer(ctx, ep); err != nil {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	if err = r.reconcileExternalResources(ctx, resourceHandlers, epCtx); err != nil {
		base := client.MergeFrom(ep.DeepCopy())
		meta.SetStatusCondition(&ep.Status.Conditions, EndpointFailedExternalReconcileCondition(ep.Generation, err.Error()))
		logger.Error(err, "failed to reconcile external resources")
		r.recorder.Eventf(ep, corev1.EventTypeWarning, "ExternalResourceReconciliationFailed",
			"External resource reconciliation failed: %s", err)
		if err := r.Client.Patch(ctx, ep, base); err != nil {
			return ctrl.Result{}, fmt.Errorf("%w, failed to patch endpoint ready condition", err)
		}
		return ctrl.Result{}, err
	}
	meta.SetStatusCondition(&ep.Status.Conditions, EndpointReadyCondition(ep.Generation))
	ep.Status.Address = kubernetes.MakeAddress(epCtx, visibility.GatewayExternal)
	if ep.Status.Address != old.Status.Address ||
		controller.NeedConditionUpdate(old.Status.Conditions, ep.Status.Conditions) {
		if err := r.Status().Update(ctx, ep); err != nil {
			logger.Error(err, "Failed to update Endpoint status")
			return ctrl.Result{}, err
		}
	}

	oldReadyCondition := meta.IsStatusConditionTrue(old.Status.Conditions, ConditionReady.String())
	newReadyCondition := meta.IsStatusConditionTrue(ep.Status.Conditions, ConditionReady.String())

	// Emit an event if the endpoint is transitioning to ready
	if !oldReadyCondition && newReadyCondition {
		r.recorder.Eventf(ep, corev1.EventTypeNormal, "EndpointReady",
			"Endpoint is ready")
	}

	return ctrl.Result{}, nil
}

// makeEndpointContext creates a endpoint context for the given deployment by retrieving the
// parent objects that this deployment is associated with.
func (r *Reconciler) makeEndpointContext(ctx context.Context, ep *choreov1.Endpoint) (*dataplane.EndpointContext, error) {
	project, err := controller.GetProject(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the project: %w", err)
	}

	component, err := controller.GetComponent(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the component: %w", err)
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment track: %w", err)
	}

	environment, err := controller.GetEnvironment(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the environment: %w", err)
	}

	deployment, err := controller.GetDeployment(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment: %w", err)
	}
	dp, err := controller.GetDataplane(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the dataplane: %w", err)
	}
	return &dataplane.EndpointContext{
		DataPlane:       dp,
		Project:         project,
		Component:       component,
		DeploymentTrack: deploymentTrack,
		Deployment:      deployment,
		Environment:     environment,
		Endpoint:        ep,
	}, nil
}

func (r *Reconciler) makeExternalResourceHandlers() []dataplane.ResourceHandler[dataplane.EndpointContext] {
	// Define the resource handlers for the external resources
	resourceHandlers := []dataplane.ResourceHandler[dataplane.EndpointContext]{
		k8sintegrations.NewHTTPRouteHandler(r.Client, &visibility.PublicVisibilityStrategy{}),
		k8sintegrations.NewHTTPRouteHandler(r.Client, &visibility.OrganizationVisibilityStrategy{}),
		k8sintegrations.NewSecurityPolicyHandler(r.Client, &visibility.PublicVisibilityStrategy{}),
		k8sintegrations.NewSecurityPolicyHandler(r.Client, &visibility.OrganizationVisibilityStrategy{}),
	}

	return resourceHandlers
}

// reconcileExternalResources reconciles the provided external resources based on the deployment context.
func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	resourceHandlers []dataplane.ResourceHandler[dataplane.EndpointContext],
	epCtx *dataplane.EndpointContext) error {
	handlerNameLogKey := "resourceHandler"
	for _, resourceHandler := range resourceHandlers {
		logger := log.FromContext(ctx).WithValues(handlerNameLogKey, resourceHandler.Name())
		// Delete the external resource if it is not configured
		if !resourceHandler.IsRequired(epCtx) {
			if err := resourceHandler.Delete(ctx, epCtx); err != nil {
				logger.Error(err, "Error deleting external resource")
				return err
			}
			// No need to reconcile the external resource if it is not required
			logger.Info("Deleted external resource")
			continue
		}

		// Check if the external resource exists
		currentState, err := resourceHandler.GetCurrentState(ctx, epCtx)
		if err != nil {
			logger.Error(err, "Error retrieving current state of the external resource")
			return err
		}
		exists := currentState != nil
		if !exists {
			// Create the external resource if it does not exist
			if err := resourceHandler.Create(ctx, epCtx); err != nil {
				logger.Error(err, "Error creating external resource")
				return err
			}
		} else {
			// Update the external resource if it exists
			if err := resourceHandler.Update(ctx, epCtx, currentState); err != nil {
				logger.Error(err, "Error updating external resource")
				return err
			}
		}

		logger.Info("Reconciled external resource")
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.recorder == nil {
		r.recorder = mgr.GetEventRecorderFor("endpoint-controller")
	}

	if err := r.setupDataPlaneRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup dataPlane reference index: %w", err)
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Endpoint{}).
		Named("endpoint").
		WithEventFilter(predicate.GenerationChangedPredicate{}). // Only reconcile on spec changes
		Watches(
			&choreov1.DataPlane{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointsForDataplane),
		).
		Watches(
			&choreov1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointsForEnvironment),
		)

	return builder.Complete(r)
}
