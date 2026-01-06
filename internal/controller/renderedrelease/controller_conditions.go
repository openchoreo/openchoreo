// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionFinalizing represents whether the RenderedRelease is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Constants for condition reasons

const (
	// Reasons for Finalizing condition type

	// ReasonCleanupInProgress dataplane resources are being cleaned up
	ReasonCleanupInProgress controller.ConditionReason = "CleanupInProgress"
	// ReasonCleanupFailed cleanup of dataplane resources failed
	ReasonCleanupFailed controller.ConditionReason = "CleanupFailed"
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
