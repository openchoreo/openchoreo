// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	namespace, err := r.observedNamespace(ctx, releaseBinding)
	if err != nil {
		return err
	}
	if namespace == "" {
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
	// No revision is applied while the binding observes, so none may read
	// active. Their health and the binding's highestRevision are kept. A failed
	// write is returned once the status below is set, so the request is retried.
	var revErr error
	if err := r.revisionSnapshotter().Deactivate(ctx, releaseBinding); err != nil {
		revErr = fmt.Errorf("deactivate revisions: %w", err)
	}

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
		return revErr
	}
	setResourcesReadyFromStatuses(ctx, releaseBinding, rr.Status.ObservedResources, component)
	return revErr
}

// observedNamespace returns the namespace the project's ProjectReleaseBinding
// observes. When there is no such namespace, it says why on the binding and
// returns "".
func (r *Reconciler) observedNamespace(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) (string, error) {
	project := releaseBinding.Spec.Owner.ProjectName
	environment := releaseBinding.Spec.Environment

	prb, err := r.findProjectReleaseBinding(ctx, releaseBinding)
	switch {
	case errors.Is(err, errAmbiguousProjectReleaseBinding):
		markNotObserving(releaseBinding, ReasonProjectReleaseBindingConflict,
			fmt.Sprintf("Several ProjectReleaseBindings target project %q in environment %q", project, environment))
		return "", nil
	case err != nil:
		return "", err
	case prb == nil:
		markNotObserving(releaseBinding, ReasonProjectReleaseBindingNotFound,
			fmt.Sprintf("No ProjectReleaseBinding for project %q in environment %q; Observe mode needs one in Observe mode",
				project, environment))
		return "", nil
	case !prb.Spec.IsObserve():
		markNotObserving(releaseBinding, ReasonProjectNotObserved,
			fmt.Sprintf("ProjectReleaseBinding %q is in Manage mode; observing a component under it is not supported yet",
				prb.Name))
		return "", nil
	case prb.Status.Namespace == "":
		msg := fmt.Sprintf("ProjectReleaseBinding %q is not observing a namespace", prb.Name)
		if ready := meta.FindStatusCondition(prb.Status.Conditions, string(projectreleasebinding.ConditionReady)); ready != nil && ready.Message != "" {
			msg += ": " + ready.Message
		}
		markNotObserving(releaseBinding, ReasonNamespaceNotObserved, msg)
		return "", nil
	}
	return prb.Status.Namespace, nil
}

// errAmbiguousProjectReleaseBinding is returned when several
// ProjectReleaseBindings target the binding's project and environment.
var errAmbiguousProjectReleaseBinding = errors.New("several ProjectReleaseBindings target this project and environment")

// findProjectReleaseBinding returns the ProjectReleaseBinding of the binding's
// project and environment, nil when there is none, and
// errAmbiguousProjectReleaseBinding when there are several. One being deleted
// is skipped, so its replacement takes over while it finalizes.
func (r *Reconciler) findProjectReleaseBinding(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	list := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, list, client.InNamespace(releaseBinding.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list ProjectReleaseBindings: %w", err)
	}
	var found *openchoreov1alpha1.ProjectReleaseBinding
	for i := range list.Items {
		prb := &list.Items[i]
		if prb.Spec.Owner.ProjectName != releaseBinding.Spec.Owner.ProjectName ||
			prb.Spec.Environment != releaseBinding.Spec.Environment ||
			!prb.DeletionTimestamp.IsZero() {
			continue
		}
		if found != nil {
			return nil, errAmbiguousProjectReleaseBinding
		}
		found = prb
	}
	return found, nil
}

// emitObservedRelease creates or updates the binding's RenderedRelease in
// Observe mode. It returns nil without an error when another owner holds the
// release.
func (r *Reconciler) emitObservedRelease(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	observed []openchoreov1alpha1.ObservedResource,
) (*openchoreov1alpha1.RenderedRelease, controllerutil.OperationResult, error) {
	rr := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observedReleaseName(releaseBinding),
			Namespace: releaseBinding.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, rr, func() error {
		if rr.UID != "" && !metav1.IsControlledBy(rr, releaseBinding) {
			return fmt.Errorf("release exists but is not owned by this ReleaseBinding")
		}
		rr.Labels = map[string]string{
			labels.LabelKeyNamespaceName:   releaseBinding.Namespace,
			labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
			labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
			labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
		}
		if line := releaseBinding.Spec.Owner.ReleaseLine; line != "" {
			rr.Labels[labels.LabelKeyReleaseLine] = line
		}
		rr.Spec = openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   releaseBinding.Spec.Owner.ProjectName,
				ComponentName: releaseBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName:   releaseBinding.Spec.Environment,
			TargetPlane:       openchoreov1alpha1.TargetPlaneDataPlane,
			Mode:              openchoreov1alpha1.RenderedReleaseModeObserve,
			ObservedResources: observed,
		}
		return controllerutil.SetControllerReference(releaseBinding, rr, r.Scheme)
	})
	if err != nil {
		if strings.Contains(err.Error(), "not owned by") {
			markNotObserving(releaseBinding, ReasonReleaseOwnershipConflict,
				fmt.Sprintf("Release %q exists but is owned by another resource", rr.Name))
			return nil, op, nil
		}
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", rr.Name, err))
		return nil, op, err
	}
	return rr, op, nil
}

// stopObserving deletes the binding's RenderedRelease if it is in Observe
// mode. A release in Manage mode is left as it is: deleting it would remove
// what it deployed.
func (r *Reconciler) stopObserving(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) error {
	rr, err := r.observingRelease(ctx, releaseBinding)
	if err != nil {
		return err
	}
	if rr != nil {
		if err := r.deleteObservingRelease(ctx, rr); err != nil {
			return err
		}
	} else if running, err := r.hasRenderedRelease(ctx, releaseBinding); err != nil || running {
		// A release still in Manage mode runs the binding's active revision.
		return err
	}
	// The binding observes or runs nothing, so no revision may read active --
	// including when deactivating failed on an earlier reconcile, after the
	// switch to Observe.
	if err := r.revisionSnapshotter().Deactivate(ctx, releaseBinding); err != nil {
		return fmt.Errorf("deactivate revisions: %w", err)
	}
	return nil
}

// hasRenderedRelease reports whether the binding's RenderedRelease exists and
// is not being deleted.
func (r *Reconciler) hasRenderedRelease(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding) (bool, error) {
	rr := &openchoreov1alpha1.RenderedRelease{}
	err := r.Get(ctx, types.NamespacedName{Name: observedReleaseName(releaseBinding), Namespace: releaseBinding.Namespace}, rr)
	switch {
	case apierrors.IsNotFound(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("failed to get RenderedRelease %q: %w", observedReleaseName(releaseBinding), err)
	}
	return rr.DeletionTimestamp.IsZero(), nil
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
	prb, err := r.findProjectReleaseBinding(ctx, releaseBinding)
	if err != nil && !errors.Is(err, errAmbiguousProjectReleaseBinding) {
		return err
	}
	if prb != nil && prb.Spec.IsObserve() && prb.Status.Namespace != "" {
		return nil
	}
	return r.deleteObservingRelease(ctx, rr)
}

// observingRelease returns the binding's RenderedRelease if the binding
// controls it and it is in Observe mode, and nil otherwise.
func (r *Reconciler) observingRelease(ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
	name := observedReleaseName(releaseBinding)
	rr := &openchoreov1alpha1.RenderedRelease{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: releaseBinding.Namespace}, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get RenderedRelease %q: %w", name, err)
	}
	if !metav1.IsControlledBy(rr, releaseBinding) || !rr.DeletionTimestamp.IsZero() ||
		rr.Spec.Mode != openchoreov1alpha1.RenderedReleaseModeObserve {
		return nil, nil
	}
	return rr, nil
}

// deleteObservingRelease deletes a release read in Observe mode. The delete
// is conditioned on the release's UID and resourceVersion, so a release that
// has since been converted to Manage, and may have applied resources, is
// never deleted from a stale read.
func (r *Reconciler) deleteObservingRelease(ctx context.Context, rr *openchoreov1alpha1.RenderedRelease) error {
	err := r.Delete(ctx, rr, client.Preconditions{UID: &rr.UID, ResourceVersion: &rr.ResourceVersion})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete RenderedRelease %q: %w", rr.Name, err)
	}
	return nil
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
// delivery markers of the last rollout. It also clears currentRevision: an
// observed binding applies no revision, and no revision is recorded while it
// observes (reconcileObserve also marks the revisions inactive). highestRevision is kept so revision numbers are never reused once it
// returns to Manage mode. It runs only once the release is in Observe mode:
// while a Manage-mode release still runs, consumers keep resolving its
// endpoints and the binding still applies its current revision.
func clearManagedStatus(releaseBinding *openchoreov1alpha1.ReleaseBinding) {
	releaseBinding.Status.CurrentRevision = ""
	releaseBinding.Status.Endpoints = nil
	releaseBinding.Status.EndpointsObservedGeneration = 0
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
