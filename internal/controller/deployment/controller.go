// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	k8sintegrations "github.com/openchoreo/openchoreo/internal/controller/deployment/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpKubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Reconciler reconciles a Deployment object
type Reconciler struct {
	client.Client
	DpClientMgr *dpKubernetes.KubeClientManager
	Scheme      *runtime.Scheme
	recorder    record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Deployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Deployment instance for this reconcile request
	deployment := &openchoreov1alpha1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			// The Deployment resource may have been deleted since it triggered the reconcile
			logger.Info("Deployment resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	old := deployment.DeepCopy()

	// Handle the deletion of the deployment
	if !deployment.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing deployment")
		return r.finalize(ctx, old, deployment)
	}

	// Ensure the finalizer is added to the deployment
	if finalizerAdded, err := r.ensureFinalizer(ctx, deployment); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Mark the deployment as progressing so that any non-terminating paths will persist the progressing status
	meta.SetStatusCondition(&deployment.Status.Conditions, NewDeploymentProgressingCondition(deployment.Generation))

	// Create a new deployment context for the deployment with relevant hierarchy objects
	deploymentCtx, err := r.makeDeploymentContext(ctx, deployment)
	if err != nil {
		logger.Error(err, "Error creating deployment context")
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "ContextResolutionFailed",
			"Context resolution failed: %s", err)
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, deployment); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, controller.IgnoreHierarchyNotFoundError(err)
	}

	dpClient, err := r.getDPClient(ctx, deploymentCtx.Environment)
	if err != nil {
		logger.Error(err, "Error getting DP client")
		return ctrl.Result{}, err
	}

	// Find and reconcile all the external resources
	externalResourceHandlers := r.makeExternalResourceHandlers(dpClient)
	if err := r.reconcileExternalResources(ctx, externalResourceHandlers, deploymentCtx); err != nil {
		logger.Error(err, "Error reconciling external resources")
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "ExternalResourceReconciliationFailed",
			"External resource reconciliation failed: %s", err)
		return ctrl.Result{}, err
	}

	if err := r.reconcileChoreoEndpoints(ctx, deploymentCtx); err != nil {
		logger.Error(err, "Error reconciling endpoints")
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "EndpointReconciliationFailed",
			"Endpoint reconciliation failed: %s", err)
		return ctrl.Result{}, err
	}

	// TODO: Update the status of the deployment and emit events

	// Mark the deployment as ready. Reaching this point means the deployment is successfully reconciled.
	meta.SetStatusCondition(&deployment.Status.Conditions, NewDeploymentReadyCondition(deployment.Generation))

	if err := controller.UpdateStatusConditions(ctx, r.Client, old, deployment); err != nil {
		return ctrl.Result{}, err
	}

	oldReadyCondition := meta.IsStatusConditionTrue(old.Status.Conditions, ConditionReady.String())
	newReadyCondition := meta.IsStatusConditionTrue(deployment.Status.Conditions, ConditionReady.String())

	// Emit an event if the deployment is transitioning to ready
	if !oldReadyCondition && newReadyCondition {
		r.recorder.Event(deployment, corev1.EventTypeNormal, "DeploymentReady", "Deployment is ready")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.recorder == nil {
		r.recorder = mgr.GetEventRecorderFor("deployment-controller")
	}

	// Set up the index for the deployment artifact reference
	if err := r.setupDeploymentArtifactRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup deployment artifact reference index: %w", err)
	}

	// Set up the index for the configuration group reference via deployment artifacts
	if err := r.setupConfigurationGroupRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup deployment artifact reference index: %w", err)
	}

	// Set up the index for the endpoints that are owned by the deployment
	if err := r.setupEndpointsOwnerRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup endpoints owner reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Deployment{}).
		Named("deployment").
		// Watch for DeployableArtifact changes to reconcile the deployments
		Watches(
			&openchoreov1alpha1.DeployableArtifact{},
			handler.EnqueueRequestsFromMapFunc(r.listDeploymentsForDeployableArtifact),
		).
		// Watch for ConfigurationGroup changes to reconcile the deployments
		Watches(
			&openchoreov1alpha1.ConfigurationGroup{},
			handler.EnqueueRequestsFromMapFunc(r.listDeploymentsForConfigurationGroup),
		).
		// Watch Endpoints that are associated to the deployment by label
		Watches(
			&openchoreov1alpha1.Endpoint{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*openchoreov1alpha1.Endpoint, *openchoreov1alpha1.Deployment](
				r.Client, controller.GetDeployment)),
		).
		Complete(r)
}

// makeExternalResourceHandlers creates the chain of external resource handlers that are used to
// bring the external resources to the desired state.
func (r *Reconciler) makeExternalResourceHandlers(dpClient client.Client) []dataplane.ResourceHandler[dataplane.DeploymentContext] {
	var handlers []dataplane.ResourceHandler[dataplane.DeploymentContext]

	// IMPORTANT: The order of the handlers is important when reconciling the resources.
	// For example, the namespace handler should be reconciled before creating resources that depend on the namespace.
	handlers = append(handlers, k8sintegrations.NewNamespaceHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewCiliumNetworkPolicyHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewConfigMapHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewSecretProviderClassHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewCronJobHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewDeploymentHandler(dpClient))
	handlers = append(handlers, k8sintegrations.NewServiceHandler(dpClient))

	return handlers
}

func (r *Reconciler) getDPClient(ctx context.Context, env *openchoreov1alpha1.Environment) (client.Client, error) {
	dataplaneRes, err := controller.GetDataplaneOfEnv(ctx, r.Client, env)
	if err != nil {
		// Return an error if dataplane retrieval fails
		return nil, fmt.Errorf("failed to get dataplane for environment %s: %w", env.Name, err)
	}

	dpClient, err := dpKubernetes.GetDPClient(r.DpClientMgr, dataplaneRes)
	if err != nil {
		return nil, fmt.Errorf("failed to get DP client: %w", err)
	}

	return dpClient, nil
}

// reconcileExternalResources reconciles the provided external resources based on the deployment context.
func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	resourceHandlers []dataplane.ResourceHandler[dataplane.DeploymentContext],
	deploymentCtx *dataplane.DeploymentContext) error {
	handlerNameLogKey := "resourceHandler"
	for _, resourceHandler := range resourceHandlers {
		logger := log.FromContext(ctx).WithValues(handlerNameLogKey, resourceHandler.Name())
		// Delete the external resource if it is not configured
		if !resourceHandler.IsRequired(deploymentCtx) {
			if err := resourceHandler.Delete(ctx, deploymentCtx); err != nil {
				logger.Error(err, "Error deleting external resource")
				return err
			}
			// No need to reconcile the external resource if it is not required
			logger.Info("Deleted external resource")
			continue
		}

		// Check if the external resource exists
		currentState, err := resourceHandler.GetCurrentState(ctx, deploymentCtx)
		if err != nil {
			logger.Error(err, "Error retrieving current state of the external resource")
			return err
		}

		exists := currentState != nil
		if !exists {
			// Create the external resource if it does not exist
			if err := resourceHandler.Create(ctx, deploymentCtx); err != nil {
				logger.Error(err, "Error creating external resource")
				return err
			}
		} else {
			// Update the external resource if it exists
			if err := resourceHandler.Update(ctx, deploymentCtx, currentState); err != nil {
				logger.Error(err, "Error updating external resource")
				return err
			}
		}

		logger.Info("Reconciled external resource")
	}

	return nil
}
