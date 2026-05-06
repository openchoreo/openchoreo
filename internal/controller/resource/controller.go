// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleases,verbs=get;list;watch;create;update;patch;delete

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

	rtSnapshot, err := r.resolveType(ctx, res)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s %q not found", resolvedKind(res.Spec.Type.Kind), res.Spec.Type.Name)
			controller.MarkFalseCondition(res, ConditionReady, ReasonResourceTypeNotFound, msg)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	releaseHash := ComputeReleaseHash(&ReleaseSpec{
		ResourceType: rtSnapshot,
		Parameters:   res.Spec.Parameters,
	}, nil)

	if res.Status.LatestRelease == nil || res.Status.LatestRelease.Hash != releaseHash {
		rrName := fmt.Sprintf("%s-%s", res.Name, releaseHash)
		if err := r.ensureResourceRelease(ctx, res, rtSnapshot, rrName); err != nil {
			return ctrl.Result{}, err
		}
		res.Status.LatestRelease = &openchoreov1alpha1.LatestResourceRelease{
			Name: rrName,
			Hash: releaseHash,
		}
	}

	controller.MarkTrueCondition(res, ConditionReady, ReasonReconciled,
		fmt.Sprintf("ResourceRelease %s in place", res.Status.LatestRelease.Name))

	return ctrl.Result{}, nil
}

// resolveType fetches the (Cluster)ResourceType referenced by res.Spec.Type and
// returns the snapshot to embed in a ResourceRelease. Returns an
// apierrors.IsNotFound error when the referenced template is missing.
func (r *Reconciler) resolveType(ctx context.Context, res *openchoreov1alpha1.Resource) (
	openchoreov1alpha1.ResourceReleaseResourceType, error,
) {
	kind := resolvedKind(res.Spec.Type.Kind)
	name := res.Spec.Type.Name

	switch kind {
	case openchoreov1alpha1.ResourceTypeRefKindClusterResourceType:
		crt := &openchoreov1alpha1.ClusterResourceType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, crt); err != nil {
			return openchoreov1alpha1.ResourceReleaseResourceType{}, err
		}
		return openchoreov1alpha1.ResourceReleaseResourceType{
			Kind: kind,
			Name: name,
			Spec: clusterResourceTypeSpecToResourceTypeSpec(crt.Spec),
		}, nil
	default:
		rt := &openchoreov1alpha1.ResourceType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: res.Namespace}, rt); err != nil {
			return openchoreov1alpha1.ResourceReleaseResourceType{}, err
		}
		return openchoreov1alpha1.ResourceReleaseResourceType{
			Kind: kind,
			Name: name,
			Spec: rt.Spec,
		}, nil
	}
}

// ensureResourceRelease creates a ResourceRelease with the given name if it
// doesn't already exist. AlreadyExists is treated as success so a parallel
// reconcile that won the race is benign.
func (r *Reconciler) ensureResourceRelease(
	ctx context.Context,
	res *openchoreov1alpha1.Resource,
	rt openchoreov1alpha1.ResourceReleaseResourceType,
	name string,
) error {
	rr := &openchoreov1alpha1.ResourceRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: res.Namespace,
		},
		Spec: openchoreov1alpha1.ResourceReleaseSpec{
			Owner: openchoreov1alpha1.ResourceReleaseOwner{
				ProjectName:  res.Spec.Owner.ProjectName,
				ResourceName: res.Name,
			},
			ResourceType: rt,
			Parameters:   res.Spec.Parameters,
		},
	}
	if err := r.Create(ctx, rr); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create ResourceRelease %q: %w", name, err)
	}
	return nil
}

// clusterResourceTypeSpecToResourceTypeSpec converts the structurally-identical
// ClusterResourceTypeSpec to ResourceTypeSpec so both kinds share a single
// snapshot type on ResourceRelease (mirrors the ComponentReleaseComponentType
// precedent). If the cluster-scoped spec ever diverges, this conversion needs
// revisiting along with ResourceReleaseResourceType.Spec's type.
func clusterResourceTypeSpecToResourceTypeSpec(s openchoreov1alpha1.ClusterResourceTypeSpec) openchoreov1alpha1.ResourceTypeSpec {
	return openchoreov1alpha1.ResourceTypeSpec(s)
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
