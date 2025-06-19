// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointv2

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
	"github.com/openchoreo/openchoreo/internal/controller/endpointv2/render"
)

// Reconciler reconciles a EndpointV2 object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointv2s,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointv2s/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointv2s/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpointreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the EndpointV2 instance for this reconcile request
	endpointv2 := &choreov1.EndpointV2{}
	if err := r.Get(ctx, req.NamespacedName, endpointv2); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get EndpointV2")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Find the associated EndpointClass
	endpointClassName := endpointv2.Spec.ClassName
	if endpointClassName == "" {
		logger.Info("EndpointV2 has no EndpointClass specified, skipping reconciliation", "endpointv2", endpointv2.Name)
		return ctrl.Result{}, nil
	}

	endpointClass := &choreov1.EndpointClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: endpointv2.Namespace,
		Name:      endpointClassName,
	}, endpointClass); err != nil {
		logger.Error(err, "Failed to get EndpointClass", "endpointClassName", endpointClassName)
		return ctrl.Result{}, err
	}

	// Create render context
	rCtx := &render.Context{
		EndpointV2:    endpointv2,
		EndpointClass: endpointClass,
	}

	// Reconcile the EndpointRelease
	if res, err := r.reconcileEndpointRelease(ctx, rCtx); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileEndpointRelease reconciles the EndpointRelease associated with the EndpointV2.
func (r *Reconciler) reconcileEndpointRelease(ctx context.Context, rCtx *render.Context) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	endpointRelease := &choreov1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.EndpointV2.Name,
			Namespace: rCtx.EndpointV2.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, endpointRelease, func() error {
		endpointRelease.Spec = r.makeEndpointRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(rCtx.EndpointV2, endpointRelease, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile EndpointRelease", "EndpointRelease", endpointRelease.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled EndpointRelease", "EndpointRelease", endpointRelease.Name, "Operation", op)
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeEndpointRelease(rCtx *render.Context) *choreov1.EndpointRelease {
	er := &choreov1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.EndpointV2.Name,
			Namespace: rCtx.EndpointV2.Namespace,
		},
		Spec: choreov1.EndpointReleaseSpec{
			Owner: choreov1.EndpointReleaseOwner{
				ProjectName:   rCtx.EndpointV2.Spec.Owner.ProjectName,
				ComponentName: rCtx.EndpointV2.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.EndpointV2.Spec.EnvironmentName,
			Type:            rCtx.EndpointV2.Spec.Type,
		},
	}

	var resources []choreov1.Resource

	// Add HTTPRoute resource for REST endpoints
	if rCtx.EndpointV2.Spec.Type == choreov1.EndpointTypeREST {
		routes := render.HTTPRoutes(rCtx)
		for _, route := range routes {
			if route != nil {
				resources = append(resources, *route)
			}
		}

		httpRouteFilters := render.HTTPRouteFilters(rCtx)
		for _, filter := range httpRouteFilters {
			if filter != nil {
				resources = append(resources, *filter)
			}
		}

		securityPolicies := render.SecurityPolicies(rCtx)
		for _, policy := range securityPolicies {
			if policy != nil {
				resources = append(resources, *policy)
			}
		}
	}

	er.Spec.Resources = resources
	return er
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for endpoint class reference
	if err := r.setupEndpointClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup endpoint class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.EndpointV2{}).
		Watches(
			&choreov1.EndpointClass{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointV2sForEndpointClass),
		).
		Named("endpointv2").
		Complete(r)
}
