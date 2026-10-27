// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
	endpointvalidation "github.com/openchoreo/openchoreo/internal/validation/endpoint"
)

// Reconciler reconciles an Endpoint object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpoints/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointtypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterendpointtypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=deploymentpipelines,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ep := &openchoreov1alpha1.Endpoint{}
	if err := r.Get(ctx, req.NamespacedName, ep); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Endpoint not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Endpoint")
		return ctrl.Result{}, err
	}

	old := ep.DeepCopy()

	if !ep.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, old, ep)
	}

	if added, err := r.ensureFinalizer(ctx, ep); err != nil || added {
		return ctrl.Result{}, err
	}

	if updated, err := r.ensureOwnerReference(ctx, ep); err != nil || updated {
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, old, ep)
}

func (r *Reconciler) reconcile(ctx context.Context, old, ep *openchoreov1alpha1.Endpoint) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Deferred status write: skip when nothing changed, aggregate errors with
	// any returned by the body. Mirrors resource/controller.go.
	defer func() {
		ep.Status.ObservedGeneration = ep.Generation
		if apiequality.Semantic.DeepEqual(old.Status, ep.Status) {
			return
		}
		if err := r.Status().Update(ctx, ep); err != nil {
			logger.Error(err, "Failed to update Endpoint status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	snapshot, err := r.resolveType(ctx, ep)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s %q not found", resolvedKind(ep.Spec.Type.Kind), ep.Spec.Type.Name)
			controller.MarkFalseCondition(ep, ConditionReady, ReasonEndpointTypeNotFound, msg)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := endpointvalidation.ValidateValues(snapshot.Spec.Parameters, ep.Spec.Parameters); err != nil {
		msg := fmt.Sprintf("spec.parameters do not match %s %q: %v", snapshot.Kind, snapshot.Name, err)
		controller.MarkFalseCondition(ep, ConditionReady, ReasonInvalidParameters, msg)
		return ctrl.Result{}, nil
	}

	problems, err := r.validateBackends(ctx, ep)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(problems) > 0 {
		messages := make([]string, len(problems))
		for i, p := range problems {
			messages[i] = p.message
		}
		controller.MarkFalseCondition(ep, ConditionReady, problems[0].reason, strings.Join(messages, "; "))
		return ctrl.Result{}, nil
	}

	releaseHash := computeReleaseHash(ReleaseSpec{
		EndpointType: snapshot,
		Visibility:   ep.Spec.Visibility,
		BackendRefs:  ep.Spec.BackendRefs,
		Schema:       ep.Spec.Schema,
		Parameters:   ep.Spec.Parameters,
	})

	if ep.Status.LatestRelease == nil || ep.Status.LatestRelease.Hash != releaseHash {
		releaseName := fmt.Sprintf("%s-%s", ep.Name, releaseHash)
		if err := r.ensureEndpointRelease(ctx, ep, snapshot, releaseName); err != nil {
			return ctrl.Result{}, err
		}
		ep.Status.LatestRelease = &openchoreov1alpha1.LatestEndpointRelease{
			Name: releaseName,
			Hash: releaseHash,
		}
	}

	if ep.Spec.AutoDeploy {
		env, problem, err := r.firstEnvironment(ctx, ep)
		if err != nil {
			return ctrl.Result{}, err
		}
		if problem != "" {
			controller.MarkFalseCondition(ep, ConditionReady, ReasonAutoDeployFailed, problem)
			return ctrl.Result{}, nil
		}
		if err := r.ensureAutoDeployBinding(ctx, ep, env, ep.Status.LatestRelease.Name); err != nil {
			return ctrl.Result{}, err
		}
	}

	controller.MarkTrueCondition(ep, ConditionReady, ReasonReconciled,
		fmt.Sprintf("EndpointRelease %s in place", ep.Status.LatestRelease.Name))

	return ctrl.Result{}, nil
}

// resolveType fetches the (Cluster)EndpointType referenced by ep.Spec.Type and
// returns the snapshot to embed in an EndpointRelease. Returns an
// apierrors.IsNotFound error when the referenced template is missing.
func (r *Reconciler) resolveType(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (
	openchoreov1alpha1.EndpointReleaseEndpointType, error,
) {
	kind := resolvedKind(ep.Spec.Type.Kind)
	name := ep.Spec.Type.Name

	switch kind {
	case openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType:
		cet := &openchoreov1alpha1.ClusterEndpointType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, cet); err != nil {
			return openchoreov1alpha1.EndpointReleaseEndpointType{}, err
		}
		// ClusterEndpointTypeSpec is structurally identical to EndpointTypeSpec
		// today; if it ever diverges, this cast breaks at compile time and
		// EndpointReleaseEndpointType.Spec needs a kind discriminator.
		return openchoreov1alpha1.EndpointReleaseEndpointType{
			Kind: kind,
			Name: name,
			Spec: openchoreov1alpha1.EndpointTypeSpec(cet.Spec),
		}, nil
	default:
		et := &openchoreov1alpha1.EndpointType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ep.Namespace}, et); err != nil {
			return openchoreov1alpha1.EndpointReleaseEndpointType{}, err
		}
		return openchoreov1alpha1.EndpointReleaseEndpointType{
			Kind: kind,
			Name: name,
			Spec: et.Spec,
		}, nil
	}
}

// validateBackends checks the backends whose validity does not depend on the
// environment, and so can block the release rather than surface per binding.
// Returns one human-readable problem per offending backend.
//
// Only Component backends have such a check: the component must have a
// Workload declaring the named endpoint with internal or external visibility,
// and that Workload is the same in every environment. Backend kinds added later need a deliberate decision here rather
// than falling through: a Hostname or IP has nothing to look up, and a Service
// lives on the data plane, so its existence varies by environment and belongs
// with the binding's per-environment resolution, alongside an undeployed
// component.
func (r *Reconciler) validateBackends(ctx context.Context, ep *openchoreov1alpha1.Endpoint) ([]backendProblem, error) {
	// TODO: this lists every Workload in the namespace on each reconcile and
	// filters by owner in memory. It reads from the informer cache, so it is
	// cheap at today's namespace sizes, but it scales with the namespace rather
	// than the backend count. The Component controller already registers a
	// Workload owner index ("spec.owner", keyed "{project}/{component}") but
	// privately, and controller-runtime rejects registering the same index
	// twice. Moving that index into controller.SetupSharedIndexes would let this
	// become one indexed lookup per backend.
	var workloads openchoreov1alpha1.WorkloadList
	if err := r.List(ctx, &workloads, client.InNamespace(ep.Namespace)); err != nil {
		return nil, fmt.Errorf("list Workloads: %w", err)
	}

	var problems []backendProblem
	for _, ref := range ep.Spec.BackendRefs {
		if ref.Kind != openchoreov1alpha1.EndpointBackendKindComponent || ref.Component == nil {
			continue
		}
		workload := findWorkload(workloads.Items, ep.Spec.Owner.ProjectName, ref.Component.Name)
		if workload == nil {
			problems = append(problems, backendProblem{ReasonBackendNotFound, fmt.Sprintf(
				"backend %q: component %q in project %q has no Workload",
				ref.Name, ref.Component.Name, ep.Spec.Owner.ProjectName)})
			continue
		}
		// The workload endpoint only has to exist: what it is exposed to comes
		// from this Endpoint's visibility, which the component's network policy
		// follows, not from the workload.
		if _, ok := workload.Spec.Endpoints[ref.Component.Endpoint]; !ok {
			problems = append(problems, backendProblem{ReasonBackendNotFound, fmt.Sprintf(
				"backend %q: component %q has no workload endpoint %q (available: %s)",
				ref.Name, ref.Component.Name, ref.Component.Endpoint, endpointNames(workload))})
		}
	}
	return problems, nil
}

// backendProblem is one reason a backend cannot be released.
type backendProblem struct {
	reason  controller.ConditionReason
	message string
}

// findWorkload returns the Workload owned by the given component, or nil.
func findWorkload(items []openchoreov1alpha1.Workload, projectName, componentName string) *openchoreov1alpha1.Workload {
	for i := range items {
		owner := items[i].Spec.Owner
		if owner.ProjectName == projectName && owner.ComponentName == componentName {
			return &items[i]
		}
	}
	return nil
}

// endpointNames lists a Workload's endpoint names in sorted order for messages.
func endpointNames(w *openchoreov1alpha1.Workload) string {
	if len(w.Spec.Endpoints) == 0 {
		return "none"
	}
	names := make([]string, 0, len(w.Spec.Endpoints))
	for name := range w.Spec.Endpoints {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// ensureEndpointRelease creates an EndpointRelease with the given name if it
// doesn't already exist. On AlreadyExists, verify the existing release belongs
// to this Endpoint before claiming it; refuse to take over a release owned by a
// different Endpoint (defends against hash collisions and external creates
// landing on the same name).
func (r *Reconciler) ensureEndpointRelease(
	ctx context.Context,
	ep *openchoreov1alpha1.Endpoint,
	snapshot openchoreov1alpha1.EndpointReleaseEndpointType,
	name string,
) error {
	release := &openchoreov1alpha1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ep.Namespace,
		},
		Spec: openchoreov1alpha1.EndpointReleaseSpec{
			Owner: openchoreov1alpha1.EndpointReleaseOwner{
				ProjectName:  ep.Spec.Owner.ProjectName,
				EndpointName: ep.Name,
			},
			EndpointType: snapshot,
			Visibility:   ep.Spec.Visibility,
			BackendRefs:  ep.Spec.BackendRefs,
			Schema:       ep.Spec.Schema,
			Parameters:   ep.Spec.Parameters,
		},
	}
	err := r.Create(ctx, release)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create EndpointRelease %q: %w", name, err)
	}

	existing := &openchoreov1alpha1.EndpointRelease{}
	if getErr := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ep.Namespace}, existing); getErr != nil {
		return fmt.Errorf("verify existing EndpointRelease %q: %w", name, getErr)
	}
	if existing.Spec.Owner.EndpointName != ep.Name {
		return fmt.Errorf("EndpointRelease %q already exists with owner %q, refusing to claim it for %q",
			name, existing.Spec.Owner.EndpointName, ep.Name)
	}
	return nil
}

// resolvedKind returns the Kind to use for type resolution, defaulting an empty
// Kind to EndpointType (namespaced) per the Endpoint CRD's stated default.
func resolvedKind(k openchoreov1alpha1.EndpointTypeRefKind) openchoreov1alpha1.EndpointTypeRefKind {
	if k == "" {
		return openchoreov1alpha1.EndpointTypeRefKindEndpointType
	}
	return k
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	if err := r.setupEndpointTypeRefIndex(ctx, mgr); err != nil {
		return fmt.Errorf("setup endpoint type reference index: %w", err)
	}
	if err := r.setupBackendComponentIndex(ctx, mgr); err != nil {
		return fmt.Errorf("setup backend component index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Endpoint{}).
		Watches(&openchoreov1alpha1.EndpointType{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointsForEndpointType)).
		Watches(&openchoreov1alpha1.ClusterEndpointType{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointsForClusterEndpointType)).
		Watches(&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listEndpointsForWorkload)).
		Named("endpoint").
		Complete(r)
}
