// Copyright 2026 The OpenChoreo Authors
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleasebindings,verbs=get;list;watch

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

	old := res.DeepCopy()

	if !res.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, old, res)
	}

	if added, err := r.ensureFinalizer(ctx, res); err != nil || added {
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, old, res)
}

func (r *Reconciler) reconcile(ctx context.Context, old, res *openchoreov1alpha1.Resource) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

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

	releaseHash := computeReleaseHash(ReleaseSpec{
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
		// ClusterResourceTypeSpec is structurally identical to ResourceTypeSpec
		// today; if it ever diverges, this cast breaks at compile time and
		// ResourceReleaseResourceType.Spec needs a kind discriminator.
		return openchoreov1alpha1.ResourceReleaseResourceType{
			Kind: kind,
			Name: name,
			Spec: openchoreov1alpha1.ResourceTypeSpec(crt.Spec),
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
// doesn't already exist. On AlreadyExists, verify the existing release belongs
// to this Resource before claiming it; refuse to take over a release owned by
// a different Resource (defends against hash collisions and external creates
// landing on the same name).
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
	err := r.Create(ctx, rr)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create ResourceRelease %q: %w", name, err)
	}

	existing := &openchoreov1alpha1.ResourceRelease{}
	if getErr := r.Get(ctx, types.NamespacedName{Name: name, Namespace: res.Namespace}, existing); getErr != nil {
		return fmt.Errorf("verify existing ResourceRelease %q: %w", name, getErr)
	}
	if existing.Spec.Owner.ResourceName != res.Name {
		return fmt.Errorf("ResourceRelease %q already exists with owner %q, refusing to claim it for %q",
			name, existing.Spec.Owner.ResourceName, res.Name)
	}
	return nil
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
	if err := r.setupResourceTypeRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("setup resource type reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Resource{}).
		Watches(&openchoreov1alpha1.ResourceType{},
			handler.EnqueueRequestsFromMapFunc(r.listResourcesForResourceType)).
		Watches(&openchoreov1alpha1.ClusterResourceType{},
			handler.EnqueueRequestsFromMapFunc(r.listResourcesForClusterResourceType)).
		Named("resource").
		Complete(r)
}
