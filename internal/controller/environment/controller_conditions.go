// Copyright OpenChoreo Authors 2025
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
