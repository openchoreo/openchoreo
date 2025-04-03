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

package deployableartifact

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a DeployableArtifact object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DeployableArtifact object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling DeployableArtifact")

	// Fetch the DeployableArtifact instance
	deployableartifact := &choreov1.DeployableArtifact{}
	if err := r.Get(ctx, req.NamespacedName, deployableartifact); err != nil {
		if apierrors.IsNotFound(err) {
			// The DeployableArtifact resource may have been deleted since it triggered the reconcile
			logger.Info("DeployableArtifact resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get DeployableArtifact")
		return ctrl.Result{}, err
	}

	// Ignore reconcile if the DeployableArtifact is already available since this is a one-time createß
	if r.shouldIgnoreReconcile(deployableartifact) {
		return ctrl.Result{}, nil
	}

	// Keep a copy of the original object for comparison
	old := deployableartifact.DeepCopy()

	// Reconcile the DeployableArtifact resource
	r.reconcileDeployableArtifact(ctx, deployableartifact)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, deployableartifact); err != nil {
		return ctrl.Result{}, err
	}

	// Emit an event to indicate the reconcile create is complete
	r.Recorder.Event(deployableartifact, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+deployableartifact.Name)

	return ctrl.Result{}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(deployableArtifact *choreov1.DeployableArtifact) bool {
	return meta.FindStatusCondition(deployableArtifact.Status.Conditions, string(controller.TypeAvailable)) != nil
}

// reconcileDeployableArtifact handles the core reconciliation logic for the DeployableArtifact resource
func (r *Reconciler) reconcileDeployableArtifact(ctx context.Context, deployableartifact *choreov1.DeployableArtifact) {
	logger := log.FromContext(ctx).WithValues("deployableArtifact", deployableartifact.Name)
	logger.Info("Reconciling deployableArtifact")

	// Set the observed generation
	deployableartifact.Status.ObservedGeneration = deployableartifact.Generation

	// Update the status condition to indicate the deployableArtifact is created/ready
	meta.SetStatusCondition(
		&deployableartifact.Status.Conditions,
		NewDeployableArtifactAvailableCondition(deployableartifact.Generation),
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("deployableartifact-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.DeployableArtifact{}).
		Named("deployableartifact").
		Complete(r)
}
