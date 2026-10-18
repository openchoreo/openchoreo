// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
	endpointvalidation "github.com/openchoreo/openchoreo/internal/validation/endpoint"
)

// Reconciler reconciles an EndpointReleaseBinding object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline renders EndpointType templates. The instance holds CEL env and
	// program caches; reuse it across reconciles to keep them warm.
	Pipeline *endpointpipeline.Pipeline

	// CELCostLimit bounds the accumulated cost of a single CEL expression.
	// Zero selects the template engine's built-in default.
	CELCostLimit uint64

	// RenderTimeout bounds each render call. Zero disables the deadline. It is
	// handed to the pipeline at construction.
	RenderTimeout time.Duration
}

// reconcileInputs are the objects a binding renders against.
type reconcileInputs struct {
	release     *openchoreov1alpha1.EndpointRelease
	environment *openchoreov1alpha1.Environment
	dataPlane   *openchoreov1alpha1.DataPlane
	endpoint    *openchoreov1alpha1.Endpoint
	project     *openchoreov1alpha1.Project
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpointreleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	binding := &openchoreov1alpha1.EndpointReleaseBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get EndpointReleaseBinding")
		return ctrl.Result{}, err
	}

	old := binding.DeepCopy()

	// Under deletion: tear down the owned RenderedRelease (and so its
	// data-plane routes) before the binding is garbage collected.
	if !binding.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, old, binding)
	}

	if added, err := r.ensureFinalizer(ctx, binding); err != nil || added {
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, old, binding)
}

// reconcile loads the binding's inputs, resolves its backends in the
// environment, renders the type's resources, and emits a RenderedRelease.
func (r *Reconciler) reconcile(ctx context.Context, old, binding *openchoreov1alpha1.EndpointReleaseBinding) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Deferred status write: aggregate Ready so every exit path produces it,
	// skip the API call when nothing changed, aggregate errors.
	defer func() {
		setReadyCondition(binding)
		if apiequality.Semantic.DeepEqual(old.Status, binding.Status) {
			return
		}
		if err := r.Status().Update(ctx, binding); err != nil {
			logger.Error(err, "Failed to update EndpointReleaseBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	in, err := r.loadInputs(ctx, binding)
	if err != nil || in == nil {
		// in == nil: a Synced=False condition explains what is missing.
		return ctrl.Result{}, err
	}

	md := buildMetadataContext(binding, in)

	resolved, err := r.resolveBackends(ctx, binding, in.release, md.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	binding.Status.Backends = resolved.statuses

	if len(resolved.blocking) > 0 || len(resolved.backends) == 0 {
		// Leave the existing RenderedRelease alone: re-rendering without the
		// missing backend would silently shift its share of traffic to the
		// others, and rendering with it would send that share to an error. The
		// ReleaseBinding watch re-runs this once the backend becomes available.
		msg := fmt.Sprintf("Backend(s) not available in environment %q: %s",
			binding.Spec.Environment, strings.Join(resolved.blocking, "; "))
		if len(resolved.blocking) == 0 {
			msg = fmt.Sprintf("No backend is available in environment %q", binding.Spec.Environment)
		}
		controller.MarkFalseCondition(binding, ConditionBackendsResolved, ReasonBackendsUnresolved, msg)
		markSyncedFalse(binding, ReasonBackendsUnresolved, "RenderedRelease not updated until every backend resolves")
		return ctrl.Result{}, nil
	}
	controller.MarkTrueCondition(binding, ConditionBackendsResolved, ReasonBackendsResolved,
		fmt.Sprintf("%d backend(s) resolved", len(resolved.backends)))

	rr, err := r.renderAndEmit(ctx, binding, in, md, resolved.backends)
	if err != nil || rr == nil {
		return ctrl.Result{}, err
	}

	evaluateResourcesReady(binding, rr)
	return ctrl.Result{}, nil
}

// loadInputs fetches everything the binding renders against. It returns nil
// inputs (and no error) after recording Synced=False when an input is missing
// or inconsistent; errors are reserved for transient API failures.
func (r *Reconciler) loadInputs(ctx context.Context, binding *openchoreov1alpha1.EndpointReleaseBinding) (*reconcileInputs, error) {
	if binding.Spec.EndpointRelease == "" {
		markSyncedFalse(binding, ReasonEndpointReleaseNotSet,
			"spec.endpointRelease is unset; pin an EndpointRelease to deploy this binding")
		return nil, nil
	}

	in := &reconcileInputs{}
	ns := binding.Namespace

	in.release = &openchoreov1alpha1.EndpointRelease{}
	if found, err := r.getOrMark(ctx, binding, types.NamespacedName{Name: binding.Spec.EndpointRelease, Namespace: ns},
		in.release, ReasonEndpointReleaseNotFound, "EndpointRelease"); !found || err != nil {
		return nil, err
	}
	owner := in.release.Spec.Owner
	if owner.ProjectName != binding.Spec.Owner.ProjectName || owner.EndpointName != binding.Spec.Owner.EndpointName {
		markSyncedFalse(binding, ReasonInvalidReleaseConfiguration, fmt.Sprintf(
			"binding owner (project: %q, endpoint: %q) does not match EndpointRelease owner (project: %q, endpoint: %q)",
			binding.Spec.Owner.ProjectName, binding.Spec.Owner.EndpointName, owner.ProjectName, owner.EndpointName))
		return nil, nil
	}

	if err := endpointvalidation.ValidateValues(in.release.Spec.EndpointType.Spec.EnvironmentConfigs,
		binding.Spec.EndpointTypeEnvironmentConfigs); err != nil {
		markSyncedFalse(binding, ReasonInvalidEnvironmentConfigs, fmt.Sprintf(
			"spec.endpointTypeEnvironmentConfigs do not match %s %q: %v",
			in.release.Spec.EndpointType.Kind, in.release.Spec.EndpointType.Name, err))
		return nil, nil
	}

	in.environment = &openchoreov1alpha1.Environment{}
	if found, err := r.getOrMark(ctx, binding, types.NamespacedName{Name: binding.Spec.Environment, Namespace: ns},
		in.environment, ReasonEnvironmentNotFound, "Environment"); !found || err != nil {
		return nil, err
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, in.environment.Namespace, in.environment.Spec.DataPlaneRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonDataPlaneNotFound,
				fmt.Sprintf("DataPlane not found for environment %q", in.environment.Name))
			return nil, nil
		}
		return nil, fmt.Errorf("resolve DataPlane for environment %q: %w", in.environment.Name, err)
	}
	in.dataPlane = dataPlaneResult.ToDataPlane()

	in.endpoint = &openchoreov1alpha1.Endpoint{}
	if found, err := r.getOrMark(ctx, binding, types.NamespacedName{Name: binding.Spec.Owner.EndpointName, Namespace: ns},
		in.endpoint, ReasonEndpointNotFound, "Endpoint"); !found || err != nil {
		return nil, err
	}

	in.project = &openchoreov1alpha1.Project{}
	if found, err := r.getOrMark(ctx, binding, types.NamespacedName{Name: binding.Spec.Owner.ProjectName, Namespace: ns},
		in.project, ReasonProjectNotFound, "Project"); !found || err != nil {
		return nil, err
	}

	return in, nil
}

// getOrMark gets obj by key. On NotFound it records Synced=False with reason and
// returns found=false with no error.
func (r *Reconciler) getOrMark(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	key types.NamespacedName,
	obj client.Object,
	reason controller.ConditionReason,
	kind string,
) (bool, error) {
	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, reason, fmt.Sprintf("%s %q not found", kind, key.Name))
			return false, nil
		}
		return false, fmt.Errorf("get %s %q: %w", kind, key.Name, err)
	}
	return true, nil
}

// SetupWithManager sets up the controller with the Manager. The watch surface:
//   - For: EndpointReleaseBinding (primary).
//   - Owns: RenderedRelease, so data-plane status flows back into readiness.
//   - Watches: ReleaseBinding, so a backend component being deployed, moved,
//     or undeployed in an environment re-resolves the bindings routing to it.
//   - Watches: EndpointRelease, Endpoint, Environment, DataPlane,
//     ClusterDataPlane, Project, so a binding that settled into a "*NotFound"
//     state because an input landed after it recovers on its own.
//
// (Cluster)EndpointType is intentionally not watched: rendering reads the
// immutable EndpointRelease snapshot.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Pipeline == nil {
		r.Pipeline = endpointpipeline.NewPipeline(
			endpointpipeline.WithCostLimit(r.CELCostLimit),
			endpointpipeline.WithRenderTimeout(r.RenderTimeout),
		)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.EndpointReleaseBinding{}).
		Owns(&openchoreov1alpha1.RenderedRelease{}).
		Watches(&openchoreov1alpha1.ReleaseBinding{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForReleaseBinding)).
		Watches(&openchoreov1alpha1.EndpointRelease{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForEndpointRelease)).
		Watches(&openchoreov1alpha1.Endpoint{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForEndpoint)).
		Watches(&openchoreov1alpha1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForEnvironment)).
		Watches(&openchoreov1alpha1.DataPlane{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForDataPlane)).
		Watches(&openchoreov1alpha1.ClusterDataPlane{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForClusterDataPlane)).
		Watches(&openchoreov1alpha1.Project{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForProject)).
		Named("endpointreleasebinding").
		Complete(r)
}
