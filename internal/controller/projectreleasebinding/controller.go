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
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a ProjectReleaseBinding object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleases,verbs=get;list;watch

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

// reconcile validates that the pinned ProjectRelease exists, agrees on
// ownership, and that its inlined (Cluster)ProjectType satisfies the
// cell-namespace mandate. Rendering and resource readiness land in later
// Phase 4 commits.
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

	if reason, msg := validateCellNamespaceMandate(release.Spec.ProjectType.Spec); reason != "" {
		markSyncedFalse(binding, reason, msg)
		return ctrl.Result{}, nil
	}

	controller.MarkTrueCondition(binding, ConditionSynced, ReasonSyncedReady,
		"ProjectRelease resolved; cell-namespace mandate satisfied")
	return ctrl.Result{}, nil
}

// markSyncedFalse marks Synced=False with the given reason and message.
// Mirrors the resourcereleasebinding helper; broader sub-conditions
// (NamespaceReady, ResourcesReady) will be unknown'd here once they exist.
func markSyncedFalse(binding *openchoreov1alpha1.ProjectReleaseBinding,
	reason controller.ConditionReason, message string) {
	controller.MarkFalseCondition(binding, ConditionSynced, reason, message)
}

// setReadyCondition tracks Synced for now. Later phases will aggregate
// NamespaceReady and ResourcesReady alongside Synced.
func (r *Reconciler) setReadyCondition(binding *openchoreov1alpha1.ProjectReleaseBinding) {
	synced := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionSynced))
	if synced == nil {
		controller.MarkFalseCondition(binding, ConditionReady, ReasonSyncedNotReady,
			"Awaiting Synced evaluation")
		return
	}
	if synced.Status == metav1.ConditionTrue {
		controller.MarkTrueCondition(binding, ConditionReady, ReasonReady,
			"ProjectReleaseBinding is ready")
		return
	}
	controller.MarkFalseCondition(binding, ConditionReady,
		controller.ConditionReason(synced.Reason), synced.Message)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ProjectReleaseBinding{}).
		Named("projectreleasebinding").
		Complete(r)
}
