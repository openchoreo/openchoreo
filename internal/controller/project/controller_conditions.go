// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package project

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the project is created
	ConditionCreated controller.ConditionType = "Created"
	// ConditionFinalizing represents whether the project is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonProjectCreated is the reason used when a project is created/ready
	ReasonProjectCreated controller.ConditionReason = "ProjectCreated"

	// ReasonProjectFinalizing is the reason used when a projects's dependents are being deleted'
	ReasonProjectFinalizing controller.ConditionReason = "ProjectFinalizing"
)

// NewProjectCreatedCondition creates a condition to indicate the project is created/ready
func NewProjectCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonProjectCreated,
		"Project is created",
		generation,
	)
}

func NewProjectFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonProjectFinalizing,
		"Project is finalizing",
		generation,
	)
}
