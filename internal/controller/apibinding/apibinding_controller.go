// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apibinding

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/apibinding/render"
)

// Reconciler reconciles a APIBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=apibindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=apibindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=apibindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=apiclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=apireleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the APIBinding instance for this reconcile request
	apiBinding := &choreov1.APIBinding{}
	if err := r.Get(ctx, req.NamespacedName, apiBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get APIBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// TODO: Implement logic to get the associated APIClass
	// For now, we'll skip this since the APIBinding structure is not fully defined
	// This would follow the pattern from WorkloadBinding where we get the class

	// Create render context
	rCtx := &render.Context{
		APIBinding: apiBinding,
		// APIClass will be added when the spec is properly defined
	}

	// Reconcile the APIRelease
	if res, err := r.reconcileAPIRelease(ctx, rCtx); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileAPIRelease reconciles the APIRelease associated with the APIBinding.
func (r *Reconciler) reconcileAPIRelease(ctx context.Context, rCtx *render.Context) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	apiRelease := &choreov1.APIRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.APIBinding.Name,
			Namespace: rCtx.APIBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, apiRelease, func() error {
		apiRelease.Spec = r.makeAPIRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(rCtx.APIBinding, apiRelease, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile APIRelease", "APIRelease", apiRelease.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled APIRelease", "APIRelease", apiRelease.Name, "Operation", op)
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeAPIRelease(rCtx *render.Context) *choreov1.APIRelease {
	ar := &choreov1.APIRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.APIBinding.Name,
			Namespace: rCtx.APIBinding.Namespace,
		},
		Spec: choreov1.APIReleaseSpec{
			// TODO: Implement proper spec mapping when APIBinding spec is defined
			// This would follow the pattern from WorkloadBinding -> WorkloadRelease
		},
	}

	var resources []choreov1.Resource

	// Add gateway resources (HTTPRoute, SecurityPolicy, etc.) following EndpointV2 pattern
	// This is where we would generate the Gateway API resources based on the API specification

	// TODO: Implement gateway artifact generation similar to endpointv2/render/httproute.go
	// For now, we'll leave this empty since the APIBinding spec is not fully defined

	ar.Spec.Resources = resources
	return ar
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// TODO: Set up index for API class reference when APIBinding spec is defined
	// if err := r.setupAPIClassRefIndex(context.Background(), mgr); err != nil {
	//     return fmt.Errorf("failed to setup API class reference index: %w", err)
	// }

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.APIBinding{}).
		// TODO: Add watches for APIClass when the spec is defined
		// Watches(
		//     &choreov1.APIClass{},
		//     handler.EnqueueRequestsFromMapFunc(r.listAPIBindingsForAPIClass),
		// ).
		Named("apibinding").
		Complete(r)
}
