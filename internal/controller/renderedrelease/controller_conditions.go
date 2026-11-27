// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ResourcesApplied and its reasons are untyped strings rather than
// controller.ConditionType and controller.ConditionReason: the binding
// controllers pass the condition type to meta.FindStatusCondition, which takes a
// string.
const (
	// ConditionResourcesApplied indicates whether resources were successfully applied to the target plane.
	// When False, it contains the error message from the failed apply operation, or what was refused.
	ConditionResourcesApplied = "ResourcesApplied"

	// ReasonApplySucceeded indicates all resources were applied successfully
	ReasonApplySucceeded = "ApplySucceeded"
	// ReasonApplyFailed indicates one or more resources failed to apply
	ReasonApplyFailed = "ApplyFailed"
	// ReasonApplyRefused indicates nothing was applied, because the release would have
	// written an object it must not touch
	ReasonApplyRefused = "ApplyRefused"
	// ReasonObserveMode indicates the release is in Observe mode, so spec.resources
	// are not applied. ResourcesApplied is False with this reason.
	ReasonObserveMode = "ObserveMode"
)

// ObservedResourcesCurrent reports whether status.observedResources reflects
// the release's current spec, meaning its ObservedResourcesRead condition is
// from the current generation. Until then it may still hold the reads of an
// earlier spec, without the objects added since.
func ObservedResourcesCurrent(release *openchoreov1alpha1.RenderedRelease) bool {
	cond := meta.FindStatusCondition(release.Status.Conditions, string(ConditionObservedResourcesRead))
	return cond != nil && cond.ObservedGeneration == release.Generation
}

// ApplyFailedCondition returns the release's ResourcesApplied condition when
// it reports a failed or refused apply of the current generation, and nil otherwise. A
// condition from an earlier generation describes a spec that has since been
// replaced, and False with ReasonObserveMode is not a failure.
func ApplyFailedCondition(release *openchoreov1alpha1.RenderedRelease) *metav1.Condition {
	cond := meta.FindStatusCondition(release.Status.Conditions, ConditionResourcesApplied)
	if cond == nil || cond.Status != metav1.ConditionFalse ||
		cond.ObservedGeneration != release.Generation || cond.Reason == ReasonObserveMode {
		return nil
	}
	return cond
}

// Constants for condition types

const (
	// ConditionFinalizing represents whether the RenderedRelease is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"

	// ConditionObservedResourcesRead reports whether every observed resource could be
	// read from the target plane. Present only while spec.observedResources is set.
	ConditionObservedResourcesRead controller.ConditionType = "ObservedResourcesRead"
)

// Constants for condition reasons

const (
	// Reasons for Finalizing condition type

	// ReasonCleanupInProgress dataplane resources are being cleaned up
	ReasonCleanupInProgress controller.ConditionReason = "CleanupInProgress"
	// ReasonCleanupFailed cleanup of dataplane resources failed
	ReasonCleanupFailed controller.ConditionReason = "CleanupFailed"

	// Reasons for ObservedResourcesRead condition type. When the resources that could
	// not be read failed for different reasons, the condition uses ReadFailed and the
	// message lists each failure and its cause.

	// ReasonObservedReadSucceeded every observed resource was read
	ReasonObservedReadSucceeded controller.ConditionReason = "ReadSucceeded"
	// ReasonObservedResourceNotFound an observed resource does not exist
	ReasonObservedResourceNotFound controller.ConditionReason = "ResourceNotFound"
	// ReasonObservedKindNotFound the plane does not serve an observed resource's API version and kind,
	// or not at the requested scope
	ReasonObservedKindNotFound controller.ConditionReason = "KindNotFound"
	// ReasonObservedReadForbidden the plane agent's RBAC does not allow reading an observed resource
	ReasonObservedReadForbidden controller.ConditionReason = "ReadForbidden"
	// ReasonObservedReadFailed reading an observed resource failed for another reason, or for more than one
	ReasonObservedReadFailed controller.ConditionReason = "ReadFailed"
)

func NewRenderedReleaseFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonCleanupInProgress,
		"Cleaning up dataplane resources",
		generation,
	)
}

func NewRenderedReleaseCleanupFailedCondition(generation int64, err error) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonCleanupFailed,
		err.Error(),
		generation,
	)
}
