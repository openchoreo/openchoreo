// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymenttrack

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionAvailable represents whether the deploymentTrack is created
	ConditionAvailable controller.ConditionType = "Available"
	// ConditionFinalizing represents whether the deploymentTrack is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonDeploymentTrackAvailable is the reason used when a deploymentTrack is available
	ReasonDeploymentTrackAvailable controller.ConditionReason = "DeploymentTrackAvailable"

	// ReasonDeploymentTrackFinalizing is the reason used when a deploymentTrack's dependents are being deleted'
	ReasonDeploymentTrackFinalizing controller.ConditionReason = "DeploymentTrackFinalizing"
)

// NewDeploymentTrackAvailableCondition creates a condition to indicate the deploymentTrack is available
func NewDeploymentTrackAvailableCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionAvailable,
		metav1.ConditionTrue,
		ReasonDeploymentTrackAvailable,
		"DeploymentTrack is available",
		generation,
	)
}

// NewDeploymentTrackFinalizingCondition creates a condition to indicate the deploymenttrack is being finalized
func NewDeploymentTrackFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeploymentTrackFinalizing,
		"DeploymentTrack is finalizing",
		generation,
	)
}
