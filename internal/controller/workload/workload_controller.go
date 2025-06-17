// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a Workload object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointv2s,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workload object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Workload instance for this reconcile request
	workload := &choreov1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Workload")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Find the associated WorkloadClass
	//workloadClassName := workload.Spec.WorkloadTemplateSpec.ClassName
	//workloadClass := &choreov1.WorkloadClass{}
	//if err := r.Get(ctx, client.ObjectKey{
	//	Namespace: workload.Namespace,
	//	Name:      workloadClassName,
	//}, workloadClass); err != nil {
	//	logger.Error(err, "Failed to get WorkloadClass", "workloadClassName", workloadClassName)
	//	return ctrl.Result{}, err
	//}

	// TODO: Improve this to only list endpoints that are relevant to this workload
	// Find associated EndpointV2 resources for this workload
	//var endpoints []choreov1.EndpointV2
	//endpointList := &choreov1.EndpointV2List{}
	//if err := r.List(ctx, endpointList, client.InNamespace(workload.Namespace)); err != nil {
	//	logger.Error(err, "Failed to list EndpointV2 resources")
	//	return ctrl.Result{}, err
	//}

	// Filter endpoints that belong to this workload's component
	//for _, endpoint := range endpointList.Items {
	//	if endpoint.Spec.Owner.ProjectName == workload.Spec.Owner.ProjectName &&
	//		endpoint.Spec.Owner.ComponentName == workload.Spec.Owner.ComponentName &&
	//		endpoint.Spec.EnvironmentName == workload.Spec.EnvironmentName {
	//		endpoints = append(endpoints, endpoint)
	//	}
	//}

	if res, err := r.reconcileWorkloadBinding(ctx, workload); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileWorkloadRelease reconciles the WorkloadRelease associated with the Workload.
func (r *Reconciler) reconcileWorkloadBinding(ctx context.Context, workload *choreov1.Workload) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	workloadBinding := &choreov1.WorkloadBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, workloadBinding, func() error {
		workloadBinding.Spec = r.makeWorkloadBinding(workload).Spec
		return controllerutil.SetControllerReference(workload, workloadBinding, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile Workload", "Workload", workload.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled Workload", "Workload", workload.Name, "Operation", op)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeWorkloadBinding(workload *choreov1.Workload) *choreov1.WorkloadBinding {
	wb := &choreov1.WorkloadBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
		Spec: choreov1.WorkloadBindingSpec{
			EnvironmentName: "development",
			WorkloadSpec:    workload.Spec,
		},
	}
	return wb
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for workload class reference
	if err := r.setupWorkloadClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup workload class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Workload{}).
		Watches(
			&choreov1.WorkloadClass{},
			handler.EnqueueRequestsFromMapFunc(r.listWorkloadsForWorkloadClass),
		).
		Named("workload").
		Complete(r)
}
