// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a Resource object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=resources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resources/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcetypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterresourcetypes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	res := &openchoreov1alpha1.Resource{}
	if err := r.Get(ctx, req.NamespacedName, res); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Resource")
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, res)
}

func (r *Reconciler) reconcile(ctx context.Context, res *openchoreov1alpha1.Resource) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	old := res.DeepCopy()

	// Deferred status write: skip when nothing changed, aggregate errors with
	// any returned by the body. Mirrors component/controller.go:88-109.
	defer func() {
		res.Status.ObservedGeneration = res.Generation
		if apiequality.Semantic.DeepEqual(old.Status, res.Status) {
			return
		}
		if err := r.Status().Update(ctx, res); err != nil {
			logger.Error(err, "Failed to update Resource status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	if err := r.resolveType(ctx, res); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s %q not found", resolvedKind(res.Spec.Type.Kind), res.Spec.Type.Name)
			controller.MarkFalseCondition(res, ConditionReady, ReasonResourceTypeNotFound, msg)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// resolveType fetches the (Cluster)ResourceType referenced by res.Spec.Type.
// Returns an apierrors.IsNotFound error when the referenced template is missing.
func (r *Reconciler) resolveType(ctx context.Context, res *openchoreov1alpha1.Resource) error {
	switch resolvedKind(res.Spec.Type.Kind) {
	case openchoreov1alpha1.ResourceTypeRefKindClusterResourceType:
		crt := &openchoreov1alpha1.ClusterResourceType{}
		return r.Get(ctx, types.NamespacedName{Name: res.Spec.Type.Name}, crt)
	default:
		rt := &openchoreov1alpha1.ResourceType{}
		return r.Get(ctx, types.NamespacedName{Name: res.Spec.Type.Name, Namespace: res.Namespace}, rt)
	}
}

// resolvedKind returns the Kind to use for type resolution, defaulting an empty
// Kind to ResourceType (namespaced) per the Resource CRD's stated default.
func resolvedKind(k openchoreov1alpha1.ResourceTypeRefKind) openchoreov1alpha1.ResourceTypeRefKind {
	if k == "" {
		return openchoreov1alpha1.ResourceTypeRefKindResourceType
	}
	return k
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Resource{}).
		Named("resource").
		Complete(r)
}
