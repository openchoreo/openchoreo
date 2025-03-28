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

package component

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
)

// Reconciler reconciles a Component object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Component object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling component")

	// Fetch the Component instance
	component := &choreov1.Component{}
	if err := r.Get(ctx, req.NamespacedName, component); err != nil {
		if apierrors.IsNotFound(err) {
			// The Component resource may have been deleted since it triggered the reconcile
			logger.Info("Component resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Component")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := component.DeepCopy()

	// Handle the deletion of the component
	if !component.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing component")
		return r.finalize(ctx, old, component)
	}

	// Ensure the finalizer is added to the component
	if finalizerAdded, err := r.ensureFinalizer(ctx, component); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Handle creation of the component
	// Check if a condition exists already to determine if this is a first-time creation
	existingCondition := meta.FindStatusCondition(old.Status.Conditions, controller.TypeCreated)
	isNewResource := existingCondition == nil

	component.Status.ObservedGeneration = component.Generation

	meta.SetStatusCondition(
		&component.Status.Conditions,
		NewComponentCreatedCondition(component.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, component); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.Recorder.Event(component, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+component.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("component-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Component{}).
		Named("component").
		// Watch for DeploymentTrack changes to reconcile the component
		Watches(
			&choreov1.DeploymentTrack{},
			handler.EnqueueRequestsFromMapFunc(r.listComponentsForDeploymentTrack),
		).
		Complete(r)
}
