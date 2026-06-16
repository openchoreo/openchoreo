// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
)

// Reconciler reconciles a ProjectReleaseBinding object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline renders the inlined (Cluster)ProjectType resources for a
	// single ProjectReleaseBinding. The instance holds CEL env and program
	// caches; reuse it across reconciles to keep them warm.
	Pipeline *projectpipeline.Pipeline
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	binding := &openchoreov1alpha1.ProjectReleaseBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ProjectReleaseBinding")
		return ctrl.Result{}, err
	}

	// Finalizer / deletion handling lands in a later Phase 4 commit. For now
	// treat a deleting binding as a no-op.
	if !binding.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, binding)
}

// reconcile validates the pinned ProjectRelease (existence, owner agreement),
// resolves the binding's environment / dataplane / project, renders the
// inlined (Cluster)ProjectType resources via the project pipeline, validates
// the rendered output (mandatory project namespace present, no duplicate
// resources), and emits a RenderedRelease owned by this binding. Resource
// readiness aggregation and finalizer handling land in later Phase 4 commits.
func (r *Reconciler) reconcile(ctx context.Context, binding *openchoreov1alpha1.ProjectReleaseBinding) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	old := binding.DeepCopy()

	// Deferred status write: aggregate Ready (so every exit path produces
	// it), skip the API call when nothing changed, aggregate errors with any
	// returned by the body.
	defer func() {
		r.setReadyCondition(binding)
		if apiequality.Semantic.DeepEqual(old.Status, binding.Status) {
			return
		}
		if err := r.Status().Update(ctx, binding); err != nil {
			logger.Error(err, "Failed to update ProjectReleaseBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	if binding.Spec.ProjectRelease == "" {
		markSyncedFalse(binding, ReasonProjectReleaseNotSet,
			"spec.projectRelease is unset; pin a ProjectRelease to deploy this binding")
		return ctrl.Result{}, nil
	}

	release := &openchoreov1alpha1.ProjectRelease{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.ProjectRelease,
		Namespace: binding.Namespace,
	}, release); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonProjectReleaseNotFound,
				fmt.Sprintf("ProjectRelease %q not found", binding.Spec.ProjectRelease))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ProjectRelease", "projectRelease", binding.Spec.ProjectRelease)
		return ctrl.Result{}, err
	}

	if release.Spec.Owner.ProjectName != binding.Spec.Owner.ProjectName {
		markSyncedFalse(binding, ReasonInvalidReleaseConfiguration,
			fmt.Sprintf("binding owner (project: %q) does not match ProjectRelease owner (project: %q)",
				binding.Spec.Owner.ProjectName, release.Spec.Owner.ProjectName))
		return ctrl.Result{}, nil
	}

	environment, dataPlane, project, err := r.fetchSupportingResources(ctx, binding)
	if err != nil {
		return ctrl.Result{}, err
	}
	if environment == nil {
		// fetchSupportingResources already marked Synced=False; the deferred
		// aggregator handles Ready.
		return ctrl.Result{}, nil
	}

	rr, err := r.renderAndEmit(ctx, binding, release, environment, dataPlane, project)
	if err != nil {
		return ctrl.Result{}, err
	}
	if rr == nil {
		// renderAndEmit already marked Synced=False; the deferred aggregator
		// handles Ready.
		return ctrl.Result{}, nil
	}

	r.evaluateReadiness(binding, rr)
	return ctrl.Result{}, nil
}

// markSyncedFalse marks Synced=False and forces NamespaceReady and
// ResourcesReady to Unknown. Per-axis sub-conditions written by a previous
// successful reconcile would otherwise stay True after upstream validation
// breaks (snapshot deleted, environment removed, render now failing),
// giving a misleading status. Unknown signals "cannot evaluate" until
// Synced returns to True.
func markSyncedFalse(binding *openchoreov1alpha1.ProjectReleaseBinding,
	reason controller.ConditionReason, message string) {
	controller.MarkFalseCondition(binding, ConditionSynced, reason, message)
	controller.MarkUnknownCondition(binding, ConditionNamespaceReady, ReasonSyncedNotReady,
		"NamespaceReady cannot be evaluated until Synced=True")
	controller.MarkUnknownCondition(binding, ConditionResourcesReady, ReasonSyncedNotReady,
		"ResourcesReady cannot be evaluated until Synced=True")
}

// setReadyCondition aggregates Synced, NamespaceReady, and ResourcesReady
// into the top-level Ready. Ready=True only when all three sub-conditions
// are True; otherwise Ready=False inherits the failing sub-condition's
// Reason and Message in iteration order.
func (r *Reconciler) setReadyCondition(binding *openchoreov1alpha1.ProjectReleaseBinding) {
	synced := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionSynced))
	nsReady := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionNamespaceReady))
	resReady := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionResourcesReady))

	if isTrue(synced) && isTrue(nsReady) && isTrue(resReady) {
		controller.MarkTrueCondition(binding, ConditionReady, ReasonReady,
			"ProjectReleaseBinding is ready")
		return
	}

	for _, c := range []*metav1.Condition{synced, nsReady, resReady} {
		if c == nil || c.Status == metav1.ConditionTrue {
			continue
		}
		controller.MarkFalseCondition(binding, ConditionReady,
			controller.ConditionReason(c.Reason), c.Message)
		return
	}

	controller.MarkFalseCondition(binding, ConditionReady, ReasonSyncedNotReady,
		"Awaiting sub-condition evaluation")
}

func isTrue(c *metav1.Condition) bool {
	return c != nil && c.Status == metav1.ConditionTrue
}

// SetupWithManager sets up the controller with the Manager. The binding
// owns its emitted RenderedRelease so DP-side status updates re-enqueue the
// binding for readiness aggregation. Watches on DataPlane / ClusterDataPlane
// / Environment / Project recover the bootstrap-ordering case where a
// binding settles into a "*NotFound" Synced=False state because one of
// those upstream resources lands after the binding is created.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Pipeline == nil {
		r.Pipeline = projectpipeline.NewPipeline()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ProjectReleaseBinding{}).
		Owns(&openchoreov1alpha1.RenderedRelease{}).
		Watches(&openchoreov1alpha1.DataPlane{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForDataPlane)).
		Watches(&openchoreov1alpha1.ClusterDataPlane{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForClusterDataPlane)).
		Watches(&openchoreov1alpha1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForEnvironment)).
		Watches(&openchoreov1alpha1.Project{},
			handler.EnqueueRequestsFromMapFunc(r.listBindingsForProject)).
		Named("projectreleasebinding").
		Complete(r)
}
