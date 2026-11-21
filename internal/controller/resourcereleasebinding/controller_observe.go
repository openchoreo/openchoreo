// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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
)

// reconcileObserve reads the objects the binding claims, in the namespace its
// project's ProjectReleaseBinding observes, through a RenderedRelease in
// Observe mode. It needs no pinned ResourceRelease, evaluates nothing from the
// ResourceType, and writes nothing to the data plane.
func (r *Reconciler) reconcileObserve(ctx context.Context, binding *openchoreov1alpha1.ResourceReleaseBinding) error {
	namespace, notObserving, why, err := projectreleasebinding.ObservedNamespace(ctx, r.Client, binding.Namespace,
		binding.Spec.Owner.ProjectName, binding.Spec.Environment)
	if err != nil {
		return err
	}
	if namespace == "" {
		markNotObserving(binding, notObserving, why)
		return r.stopObserving(ctx, binding)
	}

	var claims []openchoreov1alpha1.ObservedResourceClaim
	if binding.Spec.Observe != nil {
		claims = binding.Spec.Observe.Resources
	}
	if len(claims) == 0 {
		markNotObserving(binding, ReasonNothingObserved,
			"spec.observe.resources is empty; list the objects this resource runs as")
		return r.stopObserving(ctx, binding)
	}

	resource := &openchoreov1alpha1.Resource{}
	if err := r.Get(ctx, types.NamespacedName{Name: binding.Spec.Owner.ResourceName, Namespace: binding.Namespace},
		resource); err != nil {
		if apierrors.IsNotFound(err) {
			markNotObserving(binding, ReasonResourceNotFound,
				fmt.Sprintf("Resource %q not found", binding.Spec.Owner.ResourceName))
			return nil
		}
		return fmt.Errorf("failed to get Resource %q: %w", binding.Spec.Owner.ResourceName, err)
	}
	if resource.Spec.Owner.ProjectName != binding.Spec.Owner.ProjectName {
		markNotObserving(binding, ReasonResourceProjectMismatch,
			fmt.Sprintf("binding project %q does not match the project %q of Resource %q",
				binding.Spec.Owner.ProjectName, resource.Spec.Owner.ProjectName, resource.Name))
		return nil
	}

	observed := controller.ObservedResources(namespace, claims)
	rr, op, err := r.emitObservedRelease(ctx, binding, observed)
	if err != nil || rr == nil {
		return err
	}
	clearManagedStatus(binding)

	switch op {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseCreated,
			fmt.Sprintf("RenderedRelease %q %s with %d observed resource(s)", rr.Name, op, len(observed)))
	default:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseSynced,
			fmt.Sprintf("RenderedRelease %q is up to date", rr.Name))
	}

	if !renderedrelease.ObservedResourcesCurrent(rr) {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("RenderedRelease %q has not read the current observed resources yet", rr.Name))
		return nil
	}
	evaluateObservedResourcesReady(binding, rr)
	return nil
}

// emitObservedRelease creates or updates the binding's RenderedRelease in
// Observe mode, under the name Manage mode uses. It returns nil without an
// error when another owner holds the release.
func (r *Reconciler) emitObservedRelease(
	ctx context.Context,
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	observed []openchoreov1alpha1.ObservedResource,
) (*openchoreov1alpha1.RenderedRelease, controllerutil.OperationResult, error) {
	name := makeRenderedReleaseName(binding)
	rr, op, err := renderedrelease.EmitObserved(ctx, r.Client, r.Scheme, binding,
		types.NamespacedName{Name: name, Namespace: binding.Namespace},
		map[string]string{
			labels.LabelKeyNamespaceName:   binding.Namespace,
			labels.LabelKeyProjectName:     binding.Spec.Owner.ProjectName,
			labels.LabelKeyResourceName:    binding.Spec.Owner.ResourceName,
			labels.LabelKeyEnvironmentName: binding.Spec.Environment,
		},
		openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:  binding.Spec.Owner.ProjectName,
				ResourceName: binding.Spec.Owner.ResourceName,
			},
			EnvironmentName:   binding.Spec.Environment,
			TargetPlane:       openchoreov1alpha1.TargetPlaneDataPlane,
			ObservedResources: observed,
		})
	if err != nil {
		if errors.Is(err, renderedrelease.ErrNotOwned) {
			markNotObserving(binding, ReasonReleaseOwnershipConflict,
				fmt.Sprintf("RenderedRelease %q exists but is owned by another resource", name))
			return nil, op, nil
		}
		markNotObserving(binding, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", name, err))
		return nil, op, err
	}
	return rr, op, nil
}

// stopObserving deletes the binding's RenderedRelease if it is in Observe
// mode. A release in Manage mode is left as it is: deleting it would remove
// what it applied.
func (r *Reconciler) stopObserving(ctx context.Context, binding *openchoreov1alpha1.ResourceReleaseBinding) error {
	rr, err := r.observingRelease(ctx, binding)
	if err != nil || rr == nil {
		return err
	}
	return renderedrelease.DeleteObserving(ctx, r.Client, rr)
}

// stopObservingUnlessAllowed runs in Manage mode. If the binding's release is
// still in Observe mode, from before the switch to Manage, it is deleted once
// the project's ProjectReleaseBinding no longer observes a namespace. While
// observing is allowed, it is left in place.
func (r *Reconciler) stopObservingUnlessAllowed(ctx context.Context, binding *openchoreov1alpha1.ResourceReleaseBinding) error {
	rr, err := r.observingRelease(ctx, binding)
	if err != nil || rr == nil {
		return err
	}
	prb, err := projectreleasebinding.Find(ctx, r.Client, binding.Namespace,
		binding.Spec.Owner.ProjectName, binding.Spec.Environment)
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
	binding *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
	return renderedrelease.GetObserving(ctx, r.Client, binding,
		types.NamespacedName{Name: makeRenderedReleaseName(binding), Namespace: binding.Namespace})
}

// evaluateObservedResourcesReady sets ResourcesReady from the health of the
// claimed objects. Healthy and Suspended pass. Otherwise a Degraded object wins
// over an unreadable one, and that over one still progressing, so the result
// doesn't depend on the order of the claims.
func evaluateObservedResourcesReady(binding *openchoreov1alpha1.ResourceReleaseBinding, rr *openchoreov1alpha1.RenderedRelease) {
	var degraded, unknown, progressing *openchoreov1alpha1.RenderedManifestStatus
	for i := range rr.Status.ObservedResources {
		st := &rr.Status.ObservedResources[i]
		switch st.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusSuspended:
		case openchoreov1alpha1.HealthStatusDegraded:
			if degraded == nil {
				degraded = st
			}
		case openchoreov1alpha1.HealthStatusUnknown:
			if unknown == nil {
				unknown = st
			}
		default:
			if progressing == nil {
				progressing = st
			}
		}
	}

	switch {
	case degraded != nil:
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesDegraded,
			fmt.Sprintf("%s %q is degraded", degraded.Kind, degraded.Name))
	case unknown != nil:
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesUnknown,
			fmt.Sprintf("%s %q could not be read; RenderedRelease %q condition %s says why",
				unknown.Kind, unknown.Name, rr.Name, renderedrelease.ConditionObservedResourcesRead))
	case progressing != nil:
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("%s %q is %s", progressing.Kind, progressing.Name, progressing.HealthStatus))
	case len(rr.Status.ObservedResources) < len(rr.Spec.ObservedResources):
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("RenderedRelease %q has not read every observed resource yet", rr.Name))
	default:
		controller.MarkTrueCondition(binding, ConditionResourcesReady, ReasonResourcesReady,
			fmt.Sprintf("All %d observed resource(s) ready", len(rr.Status.ObservedResources)))
	}
}

// markNotObserving reports why the binding observes nothing on Synced, and
// marks ResourcesReady Unknown until it observes again.
func markNotObserving(binding *openchoreov1alpha1.ResourceReleaseBinding, reason controller.ConditionReason, msg string) {
	controller.MarkFalseCondition(binding, ConditionSynced, reason, msg)
	controller.MarkUnknownCondition(binding, ConditionResourcesReady, ReasonSyncedNotReady,
		"ResourcesReady cannot be evaluated until Synced=True")
}

// clearManagedStatus drops what Manage mode reports and Observe mode does not
// compute: the outputs and their condition. It runs only once the release is
// in Observe mode, so consumers keep the outputs while a Manage-mode release
// still runs.
func clearManagedStatus(binding *openchoreov1alpha1.ResourceReleaseBinding) {
	binding.Status.Outputs = nil
	meta.RemoveStatusCondition(&binding.Status.Conditions, string(ConditionOutputsResolved))
}
