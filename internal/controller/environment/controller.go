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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	k8sintegrations "github.com/openchoreo/openchoreo/internal/controller/environment/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// Reconciler reconciles a Environment object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=environments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=environments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=environments/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Environment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	logger := log.FromContext(ctx)

	// Fetch the Environment instance
	environment := &choreov1.Environment{}
	if err := r.Get(ctx, req.NamespacedName, environment); err != nil {
		if apierrors.IsNotFound(err) {
			// The Environment resource may have been deleted since it triggered the reconcile
			logger.Info("Environment resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Environment")
		return ctrl.Result{}, err
	}

	old := environment.DeepCopy()

	_, err := r.makeEnvironmentContext(ctx, environment)
	if err != nil {
		logger.Error(err, "Error creating environment context")
		r.Recorder.Eventf(environment, corev1.EventTypeWarning, "ContextResolutionFailed",
			"Context resolution failed: %s", err)
		if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, controller.IgnoreHierarchyNotFoundError(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion and handle finalization
	if !environment.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing environment")
		return r.finalize(ctx, old, environment)
	}

	// Ensure finalizer is added to the environment
	if finalizerAdded, err := r.ensureFinalizer(ctx, environment); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Mark the environment as ready. Reaching this point means the environment is successfully reconciled.
	meta.SetStatusCondition(&environment.Status.Conditions, NewEnvironmentReadyCondition(environment.Generation))

	if err := controller.UpdateStatusConditions(ctx, r.Client, old, environment); err != nil {
		return ctrl.Result{}, err
	}

	oldReadyCondition := meta.IsStatusConditionTrue(old.Status.Conditions, ConditionReady.String())
	newReadyCondition := meta.IsStatusConditionTrue(environment.Status.Conditions, ConditionReady.String())

	// Emit an event if the environment is transitioning to ready
	if !oldReadyCondition && newReadyCondition {
		r.Recorder.Event(environment, corev1.EventTypeNormal, "EnvironmentReady", "Environment is ready")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("environment-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Environment{}).
		Named("environment").
		Watches(
			&choreov1.Endpoint{},
			handler.EnqueueRequestsFromMapFunc(controller.HierarchyWatchHandler[*choreov1.Deployment, *choreov1.Environment](
				r.Client, controller.GetEnvironment)),
		).
		Complete(r)
}

func (r *Reconciler) makeExternalResourceHandlers() []dataplane.ResourceHandler[dataplane.EnvironmentContext] {
	// Environments only has k8s namespaces as external resources
	resourceHandlers := []dataplane.ResourceHandler[dataplane.EnvironmentContext]{
		k8sintegrations.NewNamespacesHandler(r.Client),
	}

	return resourceHandlers
}
