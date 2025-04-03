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

package project

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

// Reconciler reconciles a Project object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Project object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Project instance
	project := &choreov1.Project{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			// The Project resource may have been deleted since it triggered the reconcile
			logger.Info("Project resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Project")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := project.DeepCopy()

	// Check if a condition exists already to determine if this is a first-time creation
	existingCondition := meta.FindStatusCondition(old.Status.Conditions, controller.TypeCreated)
	isNewResource := existingCondition == nil

	// Reconcile the Project resource
	r.reconcileProject(ctx, project)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, project); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.Recorder.Event(project, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+project.Name)
	}

	return ctrl.Result{}, nil
}

// reconcileProject handles the core reconciliation logic for the Project resource
func (r *Reconciler) reconcileProject(ctx context.Context, project *choreov1.Project) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)
	logger.Info("Reconciling project")

	// Set the observed generation
	project.Status.ObservedGeneration = project.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&project.Status.Conditions,
		NewProjectCreatedCondition(project.Generation),
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("project-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Project{}).
		Named("project").
		Complete(r)
}
