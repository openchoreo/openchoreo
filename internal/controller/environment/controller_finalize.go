// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"
	"time"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// EnvCleanupFinalizer is the finalizer that is used to clean up the environment.
	EnvCleanupFinalizer = "openchoreo.dev/environment-cleanup"
)

// ensureFinalizer ensures that the finalizer is added to the environment.
// The first return value indicates whether the finalizer was added to the environment.
func (r *Reconciler) ensureFinalizer(ctx context.Context, environment *openchoreov1alpha1.Environment) (bool, error) {
	if !environment.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(environment, EnvCleanupFinalizer) {
		return true, r.Update(ctx, environment)
	}

	return false, nil
}

// finalize cleans up the data plane resources associated with the environment.
func (r *Reconciler) finalize(ctx context.Context, old, environment *openchoreov1alpha1.Environment) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("environment", environment.Name)
	if !controllerutil.ContainsFinalizer(environment, EnvCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Mark the environment condition as finalizing and return so that the environment will indicate that it is being finalized.
	// The actual finalization will be done in the next reconcile loop triggered by the status update.
	if meta.SetStatusCondition(&environment.Status.Conditions, NewEnvironmentFinalizingCondition(environment.Generation)) {
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Get the deployment context and delete the data plane resources
	envCtx, err := r.makeEnvironmentContext(ctx, environment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to make environment context for finalization: %w", err)
	}

	// Cleaning up the environment.
	// This assumes that, user already removed the environment from the deployment pipelines.

	// Delete all the deployments associated to the environment.
	isPending, err := r.cleanupDeployments(ctx, environment)

	if err != nil {
		return ctrl.Result{}, err
	}

	if isPending {
		// the next reconcile will be triggered after the pending deployment(s) deleted
		return ctrl.Result{}, nil
	}

	dpClient, err := r.getDPClient(ctx, envCtx.Environment)
	if err != nil {
		logger.Error(err, "Error getting DP client")
		return ctrl.Result{}, err
	}

	resourceHandlers := r.makeExternalResourceHandlers(dpClient)
	pendingDeletion := false

	for _, resourceHandler := range resourceHandlers {
		// Check if the resource is still being deleted
		exists, err := resourceHandler.GetCurrentState(ctx, envCtx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check existence of external resource %s: %w", resourceHandler.Name(), err)
		}

		if exists == nil {
			continue
		}

		pendingDeletion = true
		// Trigger deletion of the resource as it is still exists
		if err := resourceHandler.Delete(ctx, envCtx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete external resource %s: %w", resourceHandler.Name(), err)
		}
	}

	// Requeue the reconcile loop if there are still resources pending deletion
	if pendingDeletion {
		logger.Info("environment deletion is still pending as the dependent resource deletion pending.. retrying..")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Remove the finalizer after all the dependent resources are cleaned up
	if controllerutil.RemoveFinalizer(environment, EnvCleanupFinalizer) {
		if err := r.Update(ctx, environment); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// cleanupDeployments deletes all the deployments associated with the environment.
func (r *Reconciler) cleanupDeployments(ctx context.Context, environment *openchoreov1alpha1.Environment) (bool, error) {
	logger := log.FromContext(ctx).WithValues("environment", environment.Name)
	logger.Info("Cleaning up the deployments associated with the environment")

	// List all deployments with the label `openchoreo.dev/environment=<environment.Name>`
	deploymentList := &openchoreov1alpha1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(environment.Namespace),
		client.MatchingLabels{
			labels.LabelKeyEnvironmentName:  environment.Name,
			labels.LabelKeyOrganizationName: environment.Labels[labels.LabelKeyOrganizationName],
		},
	}

	if err := r.List(ctx, deploymentList, listOpts...); err != nil {
		return false, fmt.Errorf("error listing deployments: %w", err)
	}

	if len(deploymentList.Items) == 0 {
		logger.Info("No deployments associated with the environment")
		return false, nil
	}

	// Delete each deployment
	for _, deployment := range deploymentList.Items {
		// Check if the deployment is being already deleting
		if !deployment.DeletionTimestamp.IsZero() {
			continue
		}

		if err := r.Delete(ctx, &deployment); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The deployment is already deleted, no need to retry
				continue
			}
			return false, fmt.Errorf("error deleting deployment %s: %w", deployment.Name, err)
		}
	}

	// Reaching this point means the deployment deletion is either still in progress or has just been initiated.
	// If this is the first deletion attempt, marking the pending deletion as true.
	return true, nil
}
