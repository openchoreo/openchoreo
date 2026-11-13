// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// ResourcesApplied and its reasons are untyped strings rather than
// controller.ConditionType and controller.ConditionReason: the binding
// controllers pass the condition type to meta.FindStatusCondition, which takes a
// string.
const (
	// ConditionResourcesApplied indicates whether resources were successfully applied to the target plane.
	// When False, it contains the error message from the failed apply operation.
	ConditionResourcesApplied = "ResourcesApplied"

	// ReasonApplySucceeded indicates all resources were applied successfully
	ReasonApplySucceeded = "ApplySucceeded"
	// ReasonApplyFailed indicates one or more resources failed to apply
	ReasonApplyFailed = "ApplyFailed"
	// ReasonObserveMode indicates the release is in Observe mode, so spec.resources
	// are not applied. ResourcesApplied is False with this reason.
	// TODO: the binding controllers read any current-generation False as an apply failure; check the reason first (#5362).
	ReasonObserveMode = "ObserveMode"
)

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
