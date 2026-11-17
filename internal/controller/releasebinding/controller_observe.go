// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/projectreleasebinding"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

// reconcileObserve reads the objects the binding claims, in the namespace its
// project's ProjectReleaseBinding observes, through a RenderedRelease in
// Observe mode. It renders nothing and writes nothing to the data plane.
func (r *Reconciler) reconcileObserve(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) error {
	namespace, notObserving, why, err := projectreleasebinding.ObservedNamespace(ctx, r.Client, releaseBinding.Namespace,
		releaseBinding.Spec.Owner.ProjectName, releaseBinding.Spec.Environment)
	if err != nil {
		return err
	}
	if namespace == "" {
		markNotObserving(releaseBinding, notObserving, why)
		return r.stopObserving(ctx, releaseBinding)
	}

	var claims []openchoreov1alpha1.ObservedResourceClaim
	if releaseBinding.Spec.Observe != nil {
		claims = releaseBinding.Spec.Observe.Resources
	}
	if len(claims) == 0 {
		markNotObserving(releaseBinding, ReasonNothingObserved,
			"spec.observe.resources is empty; list the objects this component runs as")
		return r.stopObserving(ctx, releaseBinding)
	}

	if _, ok, err := r.validateReleaseLine(ctx, releaseBinding); err != nil || !ok {
		return err
	}

	component := &openchoreov1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      releaseBinding.Spec.Owner.ComponentName,
		Namespace: releaseBinding.Namespace,
	}, component); err != nil {
		if apierrors.IsNotFound(err) {
			markNotObserving(releaseBinding, ReasonComponentNotFound,
				fmt.Sprintf("Component %q not found", releaseBinding.Spec.Owner.ComponentName))
			return nil
		}
		return fmt.Errorf("failed to get Component %q: %w", releaseBinding.Spec.Owner.ComponentName, err)
	}

	observed := controller.ObservedResources(namespace, claims)
	rr, op, err := r.emitObservedRelease(ctx, releaseBinding, observed)
	if err != nil || rr == nil {
		return err
	}
	clearManagedStatus(releaseBinding)

	var msg string
	reason := ReasonReleaseSynced
	switch op {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		reason = ReasonReleaseCreated
		msg = fmt.Sprintf("RenderedRelease %q %s with %d observed resource(s)", rr.Name, op, len(observed))
	default:
		msg = fmt.Sprintf("RenderedRelease %q is up to date", rr.Name)
	}
	if releaseBinding.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		msg += "; spec.state Undeploy has no effect in Observe mode"
	}
	controller.MarkTrueCondition(releaseBinding, ConditionReleaseSynced, reason, msg)

	if !renderedrelease.ObservedResourcesCurrent(rr) {
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("RenderedRelease %q has not read the current observed resources yet", rr.Name))
		return nil
	}
	setResourcesReadyFromStatuses(ctx, releaseBinding, rr.Status.ObservedResources, component)
	return nil
}

// emitObservedRelease creates or updates the binding's RenderedRelease in
// Observe mode. It returns nil without an error when another owner holds the
// release.
func (r *Reconciler) emitObservedRelease(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	observed []openchoreov1alpha1.ObservedResource,
) (*openchoreov1alpha1.RenderedRelease, controllerutil.OperationResult, error) {
	name := observedReleaseName(releaseBinding)
	releaseLabels := map[string]string{
		labels.LabelKeyNamespaceName:   releaseBinding.Namespace,
		labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
		labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
		labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
	}
	if line := releaseBinding.Spec.Owner.ReleaseLine; line != "" {
		releaseLabels[labels.LabelKeyReleaseLine] = line
	}
	rr, op, err := renderedrelease.EmitObserved(ctx, r.Client, r.Scheme, releaseBinding,
		types.NamespacedName{Name: name, Namespace: releaseBinding.Namespace}, releaseLabels,
		openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   releaseBinding.Spec.Owner.ProjectName,
				ComponentName: releaseBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName:   releaseBinding.Spec.Environment,
			TargetPlane:       openchoreov1alpha1.TargetPlaneDataPlane,
			ObservedResources: observed,
		})
	if err != nil {
		if errors.Is(err, renderedrelease.ErrNotOwned) {
			markNotObserving(releaseBinding, ReasonReleaseOwnershipConflict,
				fmt.Sprintf("Release %q exists but is owned by another resource", name))
			return nil, op, nil
		}
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", name, err))
		return nil, op, err
	}
	return rr, op, nil
}

// stopObserving deletes the binding's RenderedRelease if it is in Observe
// mode. A release in Manage mode is left as it is: deleting it would remove
// what it deployed.
func (r *Reconciler) stopObserving(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) error {
	rr, err := r.observingRelease(ctx, releaseBinding)
	if err != nil || rr == nil {
		return err
	}
	return renderedrelease.DeleteObserving(ctx, r.Client, rr)
}

// stopObservingUnlessAllowed runs in Manage mode. If the binding's release is
// still in Observe mode, from before the switch to Manage, it is deleted once
// the project's ProjectReleaseBinding no longer observes a namespace. While
// observing is allowed, it is left for the Manage path to convert in place.
func (r *Reconciler) stopObservingUnlessAllowed(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) error {
	rr, err := r.observingRelease(ctx, releaseBinding)
	if err != nil || rr == nil {
		return err
	}
	prb, err := projectreleasebinding.Find(ctx, r.Client, releaseBinding.Namespace,
		releaseBinding.Spec.Owner.ProjectName, releaseBinding.Spec.Environment)
	if err != nil && !errors.Is(err, projectreleasebinding.ErrAmbiguous) {
		return err
	}
	if prb != nil && prb.Spec.IsObserve() && prb.Status.Namespace != "" {
		return nil
	}
	return renderedrelease.DeleteObserving(ctx, r.Client, rr)
}

// observingRelease returns the binding's RenderedRelease if the binding
// controls it and it is in Observe mode, and nil otherwise.
func (r *Reconciler) observingRelease(ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
	return renderedrelease.GetObserving(ctx, r.Client, releaseBinding,
		types.NamespacedName{Name: observedReleaseName(releaseBinding), Namespace: releaseBinding.Namespace})
}

// observedReleaseName returns the binding's data-plane RenderedRelease name. It
// is the name Manage mode uses, computed from the binding since an observed
// binding may have no ComponentRelease.
func observedReleaseName(releaseBinding *openchoreov1alpha1.ReleaseBinding) string {
	owner := releaseBinding.Spec.Owner
	return releaseline.RenderedReleaseName(owner.ComponentName, owner.ReleaseLine, releaseBinding.Spec.Environment)
}

// markNotObserving reports why the binding observes nothing on both
// ReleaseSynced and ResourcesReady.
func markNotObserving(releaseBinding *openchoreov1alpha1.ReleaseBinding, reason controller.ConditionReason, msg string) {
	controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced, reason, msg)
	controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady, reason, msg)
}

// clearManagedStatus drops what Manage mode reports and Observe mode does not
// compute: endpoint URLs, connections and resource dependencies, which also
// feed the field indexes that find consumers and SecretReference users, and the
// delivery markers of the last rollout. It runs
// only once the release is in Observe mode: while a Manage-mode release still
// runs, consumers keep resolving its endpoints.
func clearManagedStatus(releaseBinding *openchoreov1alpha1.ReleaseBinding) {
	releaseBinding.Status.Endpoints = nil
	releaseBinding.Status.ConnectionTargets = nil
	releaseBinding.Status.ResolvedConnections = nil
	releaseBinding.Status.PendingConnections = nil
	releaseBinding.Status.ResourceDependencyTargets = nil
	releaseBinding.Status.PendingResourceDependencies = nil
	releaseBinding.Status.SecretReferenceNames = nil
	releaseBinding.Status.Delivery = nil
	meta.RemoveStatusCondition(&releaseBinding.Status.Conditions, string(ConditionConnectionsResolved))
	meta.RemoveStatusCondition(&releaseBinding.Status.Conditions, string(ConditionResourceDependenciesReady))
}
