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

package organization

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles a Organization object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=organizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=organizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=organizations/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Organization object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Organization instance
	organization := &choreov1.Organization{}
	if err := r.Get(ctx, req.NamespacedName, organization); err != nil {
		if apierrors.IsNotFound(err) {
			// The Organization resource may have been deleted since it triggered the reconcile
			logger.Info("Organization resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Organization")
		return ctrl.Result{}, err
	}

	old := organization.DeepCopy()

	// examine DeletionTimestamp to determine if object is under deletion and handle finalization
	if !organization.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing organization")
		return r.finalize(ctx, old, organization)
	}

	// Ensure finalizer is added to the organization
	if finalizerAdded, err := r.ensureFinalizer(ctx, organization); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	previousCondition := meta.FindStatusCondition(organization.Status.Conditions, controller.TypeReady)

	// TODO Shouldn't we add a prefix or suffix to the namespace name to avoid conflicts
	//  and identify that it is created by this controller?
	namespaceName := organization.Name

	// Check if the Namespace already exists, if not create a new one
	namespace := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace)
	if apierrors.IsNotFound(err) {
		newNamespace := makeOrganizationNamespace(organization)
		// Set Organization instance as the owner and controller
		if err := ctrl.SetControllerReference(organization, newNamespace, r.Scheme); err != nil {
			logger.Error(err, "Failed to set owner for Namespace")
			return ctrl.Result{}, err
		}
		logger.Info("Creating a new Namespace", "Namespace.Name", newNamespace.Name)
		if err := r.Create(ctx, newNamespace); err != nil {
			logger.Error(err, "Failed to create new Namespace", "Namespace.Name", newNamespace.Name)
			return ctrl.Result{}, err
		}
		logger.Info("Created a new Namespace", "Namespace.Name", newNamespace.Name)
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Namespace")
		return ctrl.Result{}, err
	}

	// Update the Namespace labels
	updated := false
	if namespace.Labels == nil {
		namespace.Labels = map[string]string{}
	}

	for key, value := range makeOrganizationNamespaceLabels(organization) {
		if namespace.Labels[key] != value {
			namespace.Labels[key] = value
			updated = true
		}
	}

	if updated {
		logger.Info("Updating Namespace", "Namespace.Name", namespace.Name)
		if err := r.Update(ctx, namespace); err != nil {
			logger.Error(err, "Failed to update Namespace", "Namespace.Name", namespace.Name)
			return ctrl.Result{}, err
		}
		logger.Info("Updated Namespace", "Namespace.Name", namespace.Name)
	}

	// Record the created Namespace in the Organization status
	organization.Status.Namespace = namespaceName
	organization.Status.ObservedGeneration = organization.Generation
	if err := controller.UpdateCondition(
		ctx,
		r.Status(),
		organization,
		&organization.Status.Conditions,
		controller.TypeReady,
		metav1.ConditionTrue,
		"NamespaceCreated",
		"Successfully created the Namespace",
	); err != nil {
		return ctrl.Result{}, err
	} else {
		if previousCondition == nil {
			r.Recorder.Event(organization, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+organization.Name)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("organization-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Organization{}).
		Owns(&corev1.Namespace{}). // Watch any changes to owned Namespaces
		Named("organization").
		Complete(r)
}

func makeOrganizationNamespace(organization *choreov1.Organization) *corev1.Namespace {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   organization.Name,
			Labels: makeOrganizationNamespaceLabels(organization),
		},
	}
	return namespace
}

func makeOrganizationNamespaceLabels(organization *choreov1.Organization) map[string]string {
	return map[string]string{
		labels.LabelKeyManagedBy:        labels.LabelValueManagedBy,
		labels.LabelKeyOrganizationName: organization.Name,
		labels.LabelKeyName:             organization.Name,
	}
}
