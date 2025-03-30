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

package environment

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	k8s "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/labels"
)

const (
	// EnvCleanupFinalizer is the finalizer that is used to clean up the environment.
	EnvCleanupFinalizer = "core.choreo.dev/env-cleanup"
)

var (
	ErrDeploymentDeletionWait = errors.New("some deployments are still finalizing, retry later")
	ErrNamespaceDeletionWait  = errors.New("some namespaces are still finalizing, retry later")
)

// ensureFinalizer ensures that the finalizer is added to the environment.
// The first return value indicates whether the finalizer was added to the environment.
func (r *Reconciler) ensureFinalizer(ctx context.Context, environment *choreov1.Environment) (bool, error) {
	if !environment.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(environment, EnvCleanupFinalizer) {
		return true, r.Update(ctx, environment)
	}

	return false, nil
}

// finalize cleans up the data plane resources associated with the environment.
func (r *Reconciler) finalize(ctx context.Context, old, environment *choreov1.Environment) (ctrl.Result, error) {
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

	// Cleaning up the environment.
	// This assumes that, user already removed the environment from the deployment pipelines.

	// Delete all the deployments associated to the environment.
	if err := r.deleteDeployments(ctx, environment); err != nil {
		if errors.Is(err, ErrDeploymentDeletionWait) {
			// this means the deployment deletion is still in progress. So, we need to retry later.
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	// Delete all namespaces created for the environment
	if err := r.deleteNamespaces(ctx, environment); err != nil {
		if errors.Is(err, ErrNamespaceDeletionWait) {
			// this means the namespace deletion is still in progress. So, we need to retry later.
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	// Remove the finalizer after all the dependent resources are cleaned up
	if controllerutil.RemoveFinalizer(environment, EnvCleanupFinalizer) {
		if err := r.Update(ctx, environment); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// deleteDeployments deletes all the deployments associated with the environment.
func (r *Reconciler) deleteDeployments(ctx context.Context, environment *choreov1.Environment) error {
	logger := log.FromContext(ctx).WithValues("environment", environment.Name)
	logger.Info("Cleaning up the deployments associated with the environment")
	// List all deployments with the label `core.choreo.dev/environment=<environment.Name>`
	deploymentList := &choreov1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(environment.Namespace),
		client.MatchingLabels{
			labels.LabelKeyEnvironmentName: environment.Name,
		},
	}

	if err := r.List(ctx, deploymentList, listOpts...); err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info("No deployments associated with the environment")
			return nil
		}
		return fmt.Errorf("error listing deployments: %w", err)
	}

	blocked := false

	// Delete each deployment
	for _, deployment := range deploymentList.Items {
		if err := r.Delete(ctx, &deployment); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The deployment is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error deleting deployment %s: %w", deployment.Name, err)
		}

		// Get the resource back to check if the resource still exists
		if err := r.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The deployment is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error getting deployment %s: %w", deployment.Name, err)
		}

		// marking blocked as true as the deployment has still not deleted.
		blocked = true
	}

	// If at least one deployment is blocked, signal that we need to retry later
	if blocked {
		return ErrDeploymentDeletionWait
	}

	return nil
}

// deleteNamespaces deletes all the namespaces created for the environment.
func (r *Reconciler) deleteNamespaces(ctx context.Context, environment *choreov1.Environment) error {
	logger := log.FromContext(ctx).WithValues("environment", environment.Name)
	logger.Info("Cleaning up the namespaces created for the environment")
	// List all namespaces with the labels `environment-name=<environment.Name>` and `organization-name=<environment.Namespace>`
	namespaceList := &corev1.NamespaceList{}
	labelSelector := client.MatchingLabels{
		k8s.LabelKeyEnvironmentName:  environment.Name,
		k8s.LabelKeyOrganizationName: environment.Namespace,
	}

	if err := r.List(ctx, namespaceList, labelSelector); err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info("No namespaces created for the environment")
			return nil
		}
		return fmt.Errorf("error listing namespaces: %w", err)
	}

	blocked := false

	// Deleting each namespace
	for _, namespace := range namespaceList.Items {
		if err := r.Delete(ctx, &namespace); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The namespace is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error deleting namespace %s: %w", namespace.Name, err)
		}

		// Get the resource back to check if the resource still exists
		if err := r.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
			if k8sapierrors.IsNotFound(err) {
				// The namespace is already deleted, no need to retry
				continue
			}
			return fmt.Errorf("error getting namespace %s: %w", namespace.Name, err)
		}

		// marking blocked as true as the namespace has still not deleted.
		blocked = true
	}

	// If at least one namespace is blocked, return an error to trigger a retry
	if blocked {
		return ErrNamespaceDeletionWait
	}

	return nil
}
