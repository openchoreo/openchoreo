// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplication

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a WebApplication object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=webapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=webapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=webapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=webapplicationbindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// The WebApplication controller creates a WebApplicationBinding that combines the WebApplication
// specification with the referenced Workload specification.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling WebApplication")

	// Fetch the WebApplication instance
	webApplication := &choreov1.WebApplication{}
	if err := r.Get(ctx, req.NamespacedName, webApplication); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get WebApplication")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if res, err := r.reconcileWebApplicationBinding(ctx, webApplication); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileWebApplicationBinding reconciles the WebApplicationBinding with the given WebApplication.
func (r *Reconciler) reconcileWebApplicationBinding(ctx context.Context, webApplication *choreov1.WebApplication) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find the associated Workload
	workload := &choreov1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      webApplication.Spec.WorkloadName,
		Namespace: webApplication.Namespace,
	}, workload); err != nil {
		logger.Error(err, "Failed to get Workload",
			"workloadName", webApplication.Spec.WorkloadName)
		return ctrl.Result{}, err
	}

	webApplicationBinding := &choreov1.WebApplicationBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApplication.Name,
			Namespace: webApplication.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, webApplicationBinding, func() error {
		webApplicationBinding.Spec = r.makeWebApplicationBinding(webApplication, workload).Spec
		return controllerutil.SetControllerReference(webApplication, webApplicationBinding, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile WebApplicationBinding", "WebApplication", webApplication.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled WebApplicationBinding", "WebApplication", webApplication.Name, "Operation", op)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeWebApplicationBinding(webApplication *choreov1.WebApplication, workload *choreov1.Workload) *choreov1.WebApplicationBinding {
	wab := &choreov1.WebApplicationBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApplication.Name,
			Namespace: webApplication.Namespace,
		},
		Spec: choreov1.WebApplicationBindingSpec{
			Owner: choreov1.WebApplicationOwner{
				ProjectName:   webApplication.Spec.Owner.ProjectName,
				ComponentName: webApplication.Spec.Owner.ComponentName,
			},
			Environment:  "development", // This should come from the actual environment when creating bindings
			ClassName:    webApplication.Spec.ClassName,
			WorkloadSpec: workload.Spec.WorkloadTemplateSpec,
			Overrides:    webApplication.Spec.Overrides,
		},
	}
	return wab
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.WebApplication{}).
		Named("webapplication").
		Complete(r)
}
