// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplicationbinding

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
	"github.com/openchoreo/openchoreo/internal/controller/webapplicationbinding/render"
)

// Reconciler reconciles a WebApplicationBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=webapplicationclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WebApplicationBinding instance
	webApplicationBinding := &openchoreov1alpha1.WebApplicationBinding{}
	if err := r.Get(ctx, req.NamespacedName, webApplicationBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get WebApplicationBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the associated WebApplicationClass
	webApplicationClass := &openchoreov1alpha1.WebApplicationClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: webApplicationBinding.Namespace,
		Name:      webApplicationBinding.Spec.ClassName,
	}, webApplicationClass); err != nil {
		logger.Error(err, "Failed to get WebApplicationClass", "webApplicationClassName", webApplicationBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	if res, err := r.reconcileRelease(ctx, webApplicationBinding, webApplicationClass); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileRelease reconciles the Release associated with the WebApplicationBinding.
func (r *Reconciler) reconcileRelease(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding, webApplicationClass *openchoreov1alpha1.WebApplicationClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApplicationBinding.Name,
			Namespace: webApplicationBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, release, func() error {
		rCtx := render.Context{
			WebApplicationBinding: webApplicationBinding,
			WebApplicationClass:   webApplicationClass,
		}
		release.Spec = r.makeRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(webApplicationBinding, release, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile Release", "Release", release.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled Release", "Release", release.Name, "Operation", op)
		// TODO: Update WebApplicationBinding status and requeue for further processing
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.WebApplicationBinding.Name,
			Namespace: rCtx.WebApplicationBinding.Namespace,
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   rCtx.WebApplicationBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.WebApplicationBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.WebApplicationBinding.Spec.Environment,
		},
	}

	var resources []openchoreov1alpha1.Resource

	// Add Deployment resource
	if res := render.Deployment(rCtx); res != nil {
		resources = append(resources, *res)
	}

	// Add Service resource
	if res := render.Service(rCtx); res != nil {
		resources = append(resources, *res)
	}

	// Add HTTPRoute resources (to act as ingress)
	if res := render.HTTPRoutes(rCtx); res != nil {
		for _, httpRoute := range res {
			resources = append(resources, *httpRoute)
		}
	}

	release.Spec.Resources = resources
	return release
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for web application class reference
	if err := r.setupWebApplicationClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup web application class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.WebApplicationBinding{}).
		Watches(
			&openchoreov1alpha1.WebApplicationClass{},
			handler.EnqueueRequestsFromMapFunc(r.listWebApplicationBindingsForWebApplicationClass),
		).
		Named("webapplicationbinding").
		Complete(r)
}
