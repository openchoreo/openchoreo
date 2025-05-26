// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package deployableartifact

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionAvailable represents whether the deployableArtifact is created
	ConditionAvailable controller.ConditionType = "Available"
	// ConditionFinalizing represents whether the deployableArtifact is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonDeployableArtifactAvailable is the reason used when a deployableArtifact is available
	ReasonDeployableArtifactAvailable controller.ConditionReason = "DeployableArtifactAvailable"

	// ReasonDeployableArtifactFinalizing is the reason used when a deployableArtifact's dependents are being deleted'
	ReasonDeployableArtifactFinalizing controller.ConditionReason = "DeployableArtifactFinalizing"
)

// NewDeployableArtifactAvailableCondition creates a condition to indicate the deployableArtifact is available
func NewDeployableArtifactAvailableCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionAvailable,
		metav1.ConditionTrue,
		ReasonDeployableArtifactAvailable,
		"DeployableArtifact is available",
		generation,
	)
}

// NewDeployableArtifactFinalizingCondition creates a condition to indicate the deployableArtifact is being finalized
func NewDeployableArtifactFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeployableArtifactFinalizing,
		"DeployableArtifact is finalizing",
		generation,
	)
}
