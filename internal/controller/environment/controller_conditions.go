// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types
const (
	// ConditionReady represents whether the environment is ready
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons
const (
	// ReasonDeploymentReady the deployment is ready
	ReasonDeploymentReady controller.ConditionReason = "EnvironmentReady"
	// ReasonEnvironmentFinalizing the deployment is progressing
	ReasonEnvironmentFinalizing controller.ConditionReason = "EnvironmentFinalizing"
	// ReasonDeletionBlocked the environment deletion is blocked
	ReasonDeletionBlocked controller.ConditionReason = "DeletionBlocked"
	// ReasonReleaseBindingsPending the environment is waiting for release bindings to be removed
	ReasonReleaseBindingsPending controller.ConditionReason = "ReleaseBindingsPending"
)

func NewEnvironmentReadyCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionTrue,
		ReasonDeploymentReady,
		"Environment is ready",
		generation,
	)
}

func NewEnvironmentFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionFalse,
		ReasonEnvironmentFinalizing,
		"Environment is finalizing",
		generation,
	)
}

func NewReleaseBindingsPendingCondition(generation int64, message string) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionFalse,
		ReasonReleaseBindingsPending,
		message,
		generation,
	)
}

func NewDeletionBlockedCondition(generation int64, message string) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionFalse,
		ReasonDeletionBlocked,
		message,
		generation,
	)
}
