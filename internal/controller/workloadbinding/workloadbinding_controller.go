// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workloadbinding

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
	"github.com/openchoreo/openchoreo/internal/controller/workloadbinding/render"
)

// Reconciler reconciles a WorkloadBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointv2s,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloadreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WorkloadBinding instance for this reconcile request
	workloadBinding := &choreov1.WorkloadBinding{}
	if err := r.Get(ctx, req.NamespacedName, workloadBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get WorkloadBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Find the associated WorkloadClass from embedded workloadSpec
	workloadClassName := workloadBinding.Spec.WorkloadSpec.WorkloadTemplateSpec.ClassName
	workloadClass := &choreov1.WorkloadClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: workloadBinding.Namespace,
		Name:      workloadClassName,
	}, workloadClass); err != nil {
		logger.Error(err, "Failed to get WorkloadClass", "workloadClassName", workloadClassName)
		return ctrl.Result{}, err
	}

	// TODO: Improve this to only list endpoints that are relevant to this workload
	// Find associated EndpointV2 resources for this workload
	var endpoints []choreov1.EndpointV2
	endpointList := &choreov1.EndpointV2List{}
	if err := r.List(ctx, endpointList, client.InNamespace(workloadBinding.Namespace)); err != nil {
		logger.Error(err, "Failed to list EndpointV2 resources")
		return ctrl.Result{}, err
	}

	// Filter endpoints that belong to this workload's component and environment
	for _, endpoint := range endpointList.Items {
		if endpoint.Spec.Owner.ProjectName == workloadBinding.Spec.WorkloadSpec.Owner.ProjectName &&
			endpoint.Spec.Owner.ComponentName == workloadBinding.Spec.WorkloadSpec.Owner.ComponentName &&
			endpoint.Spec.EnvironmentName == workloadBinding.Spec.EnvironmentName {
			endpoints = append(endpoints, endpoint)
		}
	}

	rCtx := &render.Context{
		WorkloadBinding: workloadBinding,
		WorkloadClass:   workloadClass,
		Endpoints:       endpoints,
	}

	if res, err := r.reconcileWorkloadRelease(ctx, rCtx); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileWorkloadRelease reconciles the WorkloadRelease associated with the WorkloadBinding.
//
//nolint:unparam
func (r *Reconciler) reconcileWorkloadRelease(ctx context.Context, rCtx *render.Context) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	workloadRelease := &choreov1.WorkloadRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.WorkloadBinding.Name,
			Namespace: rCtx.WorkloadBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, workloadRelease, func() error {
		workloadRelease.Spec = r.makeWorkloadRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(rCtx.WorkloadBinding, workloadRelease, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile WorkloadRelease", "WorkloadRelease", workloadRelease.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled WorkloadRelease", "WorkloadRelease", workloadRelease.Name, "Operation", op)
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeWorkloadRelease(rCtx *render.Context) *choreov1.WorkloadRelease {
	wr := &choreov1.WorkloadRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.WorkloadBinding.Name,
			Namespace: rCtx.WorkloadBinding.Namespace,
		},
		Spec: choreov1.WorkloadReleaseSpec{
			Owner: choreov1.WorkloadReleaseOwner{
				ProjectName:   rCtx.WorkloadBinding.Spec.WorkloadSpec.Owner.ProjectName,
				ComponentName: rCtx.WorkloadBinding.Name,
			},
			EnvironmentName: rCtx.WorkloadBinding.Spec.EnvironmentName,
			Type:            rCtx.WorkloadBinding.Spec.WorkloadSpec.Type,
		},
	}

	var resources []choreov1.Resource

	// Add Deployment resource
	if res := render.Deployment(rCtx); res != nil {
		resources = append(resources, *res)
	}

	// Add Service resource
	if res := render.Service(rCtx); res != nil {
		resources = append(resources, *res)
	}

	wr.Spec.Resources = resources
	return wr
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for workload class reference
	if err := r.setupWorkloadClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup workload class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.WorkloadBinding{}).
		Watches(
			&choreov1.WorkloadClass{},
			handler.EnqueueRequestsFromMapFunc(r.listWorkloadBindingsForWorkloadClass),
		).
		Named("workloadbinding").
		Complete(r)
}
