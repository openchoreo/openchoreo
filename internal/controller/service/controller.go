// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconciler reconciles a Service object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// The Service controller creates a ServiceBinding that combines the Service
// specification with the referenced Workload specification.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Service")

	// Fetch the Service instance
	service := &openchoreov1alpha1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Service")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if res, err := r.reconcileServiceBinding(ctx, service); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileServiceBinding reconciles the ServiceBinding with the given Service.
func (r *Reconciler) reconcileServiceBinding(ctx context.Context, service *openchoreov1alpha1.Service) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find the associated Workload
	workload := &openchoreov1alpha1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      service.Spec.WorkloadName,
		Namespace: service.Namespace,
	}, workload); err != nil {
		logger.Error(err, "Failed to get Workload",
			"workloadName", service.Spec.WorkloadName)
		return ctrl.Result{}, err
	}

	serviceBinding := &openchoreov1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceBinding, func() error {
		serviceBinding.Spec = r.makeServiceBinding(service, workload).Spec
		return controllerutil.SetControllerReference(service, serviceBinding, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile ServiceBinding", "Service", service.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled ServiceBinding", "Service", service.Name, "Operation", op)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeServiceBinding(service *openchoreov1alpha1.Service, workload *openchoreov1alpha1.Workload) *openchoreov1alpha1.ServiceBinding {
	sb := &openchoreov1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Spec: openchoreov1alpha1.ServiceBindingSpec{
			Owner: openchoreov1alpha1.ServiceOwner{
				ProjectName:   service.Spec.Owner.ProjectName,
				ComponentName: service.Spec.Owner.ComponentName,
			},
			Environment:  "development", // This should come from the actual environment when creating bindings
			ClassName:    service.Spec.ClassName,
			WorkloadSpec: workload.Spec.WorkloadTemplateSpec,
			APIs:         service.Spec.APIs,
		},
	}
	return sb
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for workload reference
	if err := r.setupWorkloadRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup workload reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Service{}).
		Owns(&openchoreov1alpha1.ServiceBinding{}).
		Watches(
			&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listServicesForWorkload),
		).
		Named("service").
		Complete(r)
}
