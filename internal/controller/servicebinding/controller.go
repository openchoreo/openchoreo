// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package servicebinding

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
	"github.com/openchoreo/openchoreo/internal/controller/servicebinding/render"
)

// Reconciler reconciles a ServiceBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=servicebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=serviceclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ServiceBinding instance
	serviceBinding := &openchoreov1alpha1.ServiceBinding{}
	if err := r.Get(ctx, req.NamespacedName, serviceBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ServiceBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the associated ServiceClass
	serviceClass := &openchoreov1alpha1.ServiceClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: serviceBinding.Namespace,
		Name:      serviceBinding.Spec.ClassName,
	}, serviceClass); err != nil {
		logger.Error(err, "Failed to get ServiceClass", "serviceClassName", serviceBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	if res, err := r.reconcileRelease(ctx, serviceBinding, serviceClass); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileRelease reconciles the Release associated with the ServiceBinding.
func (r *Reconciler) reconcileRelease(ctx context.Context, serviceBinding *openchoreov1alpha1.ServiceBinding, serviceClass *openchoreov1alpha1.ServiceClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceBinding.Name,
			Namespace: serviceBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, release, func() error {
		rCtx := render.Context{
			ServiceBinding: serviceBinding,
			ServiceClass:   serviceClass,
		}
		release.Spec = r.makeRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(serviceBinding, release, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile Release", "Release", release.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled Release", "Release", release.Name, "Operation", op)
		// TODO: Update ServiceBinding status and requeue for further processing
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.ServiceBinding.Name,
			Namespace: rCtx.ServiceBinding.Namespace,
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   rCtx.ServiceBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.ServiceBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.ServiceBinding.Spec.Environment,
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

	// Add HTTPRoute resources for REST APIs
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
	// Set up the index for service class reference
	if err := r.setupServiceClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup service class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ServiceBinding{}).
		Watches(
			&openchoreov1alpha1.ServiceClass{},
			handler.EnqueueRequestsFromMapFunc(r.listServiceBindingsForServiceClass),
		).
		Named("servicebinding").
		Complete(r)
}
